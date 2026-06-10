// Package libphysical implementa a API do meio físico simulado.
//
// Protocolo UDP (big-endian):
//
//	HELLO     [0x01][group_id: 16 bytes, null-padded]
//	HELLO_ACK [0x02][Mac: 6 bytes][virtual_ip: 4 bytes, network byte order]
//	DATA      [0x03][dst_ip: 4 bytes, network byte order][payload: N bytes]
//	DELIVER   [0x04][src_ip: 4 bytes, network byte order][payload: N bytes]
//	MEDIUM    [0x05]
//	MEDIUM_OK [0x06][free: 1 byte]
//	PING      [0x07]
//	PONG      [0x08][rtt_hint: 2 bytes, microssegundos]
//	BYE       [0x09]
//	ERROR     [0xFF][msg: N bytes UTF-8]
//
// Convenção de byte order:
//   - Tudo que entra e sai pela API pública está em HOST byte order.
//   - A conversão para/de network byte order acontece aqui dentro.
//   - PhyServerIP e PhyVirtualIP retornam host byte order.
package libphysical

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"time"
)

// ---------------------------------------------------------------------------
// Constantes do protocolo
// ---------------------------------------------------------------------------

const (
	msgHello = iota + 1
	msgHelloAck
	msgData
	msgDeliver
	msgMedium
	msgMediumOk
	msgPing
	msgPong
	msgBye
	msgError = 0xFF
)

const (
	maxGroupID  = 16
	recvBufSize = 2048 // MTU simulado + cabeçalhos
)

// PhyServerIP é o IP virtual do servidor de flags na rede simulada:
// 10.0.0.1 (host byte order).
const PhyServerIP uint32 = 0x0A000001

// ErrTimeout é retornado quando uma operação excede o prazo.
var ErrTimeout = errors.New("timeout")

// ---------------------------------------------------------------------------
// PhysicalHandler
// ---------------------------------------------------------------------------

// PhysicalHandler representa uma conexão ativa com o nó central.
// Os campos são privados, acesse via métodos.
type PhysicalHandler struct {
	conn       *net.UDPConn
	serverAddr *net.UDPAddr
	mac        [6]byte
	virtualIP  uint32 // HOST byte order
	groupID    string
}

// ---------------------------------------------------------------------------
// API pública
// ---------------------------------------------------------------------------

// PhyConnect conecta ao nó central e registra o grupo.
//
// groupID: identificador único do grupo (ex: "grupo_a"), máx 16 chars
// host:    IP ou hostname do nó central
// port:    porta UDP do nó central (normalmente 9000)
func PhyConnect(groupID, host string, port uint16) (*PhysicalHandler, error) {
	if groupID == "" {
		return nil, errors.New("group_id não pode ser vazio")
	}
	if len(groupID) > maxGroupID {
		return nil, fmt.Errorf("group_id muito longo (max %d)", maxGroupID)
	}

	// Resolve hostname
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, fmt.Errorf("erro resolvendo %s: %w", host, err)
	}

	// Cria socket UDP
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, fmt.Errorf("erro criando socket: %w", err)
	}

	h := &PhysicalHandler{
		conn:       conn,
		serverAddr: addr,
		groupID:    groupID,
	}

	if err = h.doHello(); err != nil {
		cErr := conn.Close()
		if cErr != nil {
			log.Printf("erro fechando conexão: %v", cErr)
		}
		return nil, err
	}

	return h, nil
}

// MustPhyConnect faz o que PhyConnect faz, mas dá pânico caso ocorra um erro
func MustPhyConnect(groupID, host string, port uint16) *PhysicalHandler {
	h, err := PhyConnect(groupID, host, port)
	if err != nil {
		panic(err)
	}
	return h
}

// PhyMacAddr retorna o MAC virtual atribuído ao grupo (6 bytes).
func (h *PhysicalHandler) PhyMacAddr() [6]byte {
	return h.mac
}

// PhyVirtualIP retorna o IP virtual do grupo em HOST byte order.
func (h *PhysicalHandler) PhyVirtualIP() uint32 {
	return h.virtualIP
}

// PhyMediumFree consulta se o meio está livre (CSMA/CA).
// Retorna true se livre, false se ocupado.
func (h *PhysicalHandler) PhyMediumFree() (bool, error) {
	pkt := []byte{msgMedium}
	if _, err := h.conn.WriteToUDP(pkt, h.serverAddr); err != nil {
		return false, fmt.Errorf("erro enviando MEDIUM: %w", err)
	}

	buf := make([]byte, 4)
	_ = h.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, _, err := h.conn.ReadFromUDP(buf)
	_ = h.conn.SetReadDeadline(time.Time{})

	if err != nil {
		return false, fmt.Errorf("erro lendo MEDIUM_OK: %w", err)
	}
	if n < 2 || buf[0] != msgMediumOk {
		return false, errors.New("resposta inválida")
	}
	return buf[1] == 0x01, nil
}

// PhySend envia bytes brutos para um IP virtual de destino.
//
// dstIP: IP do destino em HOST byte order
// data:  dados a enviar
func (h *PhysicalHandler) PhySend(dstIP uint32, data []byte) error {
	dstNet := make([]byte, 4)
	binary.BigEndian.PutUint32(dstNet, dstIP) // host → network byte order

	pkt := make([]byte, 1+4+len(data))
	pkt[0] = msgData
	copy(pkt[1:5], dstNet)
	copy(pkt[5:], data)

	if _, err := h.conn.WriteToUDP(pkt, h.serverAddr); err != nil {
		return fmt.Errorf("erro enviando DATA: %w", err)
	}
	return nil
}

// PhyRecv recebe bytes brutos do nó central.
//
// Bloqueia até chegar um pacote de dados ou o timeout esgotar.
// Pacotes de controle (MEDIUM_OK, PONG, etc.) são descartados automaticamente.
//
// buf:       buffer do chamador para os dados recebidos
// timeoutMs: timeout em milissegundos; 0 = bloqueio indefinido
//
// Retorna o número de bytes copiados para buf, o IP virtual de origem
// em HOST byte order, e um erro. Em timeout, retorna 0, 0 e ErrTimeout.
func (h *PhysicalHandler) PhyRecv(buf []byte, timeoutMs int) (n int, srcIP uint32, err error) {
	tmp := make([]byte, recvBufSize)

	var deadline time.Time
	if timeoutMs > 0 {
		deadline = time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	}

	for {
		if timeoutMs > 0 {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return 0, 0, ErrTimeout
			}
			_ = h.conn.SetReadDeadline(time.Now().Add(remaining))
		} else {
			_ = h.conn.SetReadDeadline(time.Time{})
		}

		n, _, err := h.conn.ReadFromUDP(tmp)

		if err != nil {
			_ = h.conn.SetReadDeadline(time.Time{})
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return 0, 0, ErrTimeout
			}
			return 0, 0, fmt.Errorf("lendo pacote: %w", err)
		}

		if n < 1 || tmp[0] != msgDeliver || n < 5 {
			continue // descarta controle e pacotes inválidos
		}

		ipNet := binary.BigEndian.Uint32(tmp[1:5])

		payloadLen := n - 5
		if payloadLen > len(buf) {
			payloadLen = len(buf)
		}
		copy(buf, tmp[5:5+payloadLen])

		_ = h.conn.SetReadDeadline(time.Time{})
		return payloadLen, ipNet, nil
	}
}

// PhyPing mede o RTT até o nó central.
// Retorna a duração em sucesso, ou erro em falha/timeout.
func (h *PhysicalHandler) PhyPing() (time.Duration, error) {
	pkt := []byte{msgPing}

	t0 := time.Now()

	if _, err := h.conn.WriteToUDP(pkt, h.serverAddr); err != nil {
		return 0, fmt.Errorf("erro enviando PING: %w", err)
	}

	buf := make([]byte, 8)
	_ = h.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := h.conn.ReadFromUDP(buf)
	_ = h.conn.SetReadDeadline(time.Time{})

	if err != nil {
		return 0, fmt.Errorf("erro lendo PONG: %w", err)
	}
	if n < 1 || buf[0] != msgPong {
		return 0, errors.New("resposta PONG inválida")
	}

	return time.Since(t0), nil
}

// PhyDisconnect encerra a conexão com o nó central e libera o handler.
func (h *PhysicalHandler) PhyDisconnect() {
	bye := []byte{msgBye}
	_, _ = h.conn.WriteToUDP(bye, h.serverAddr) // best-effort
	_ = h.conn.Close()
}

// ---------------------------------------------------------------------------
// Formatação (conveniência para depuração)
// ---------------------------------------------------------------------------

// FormatMAC retorna o MAC no formato xx:xx:xx:xx:xx:xx.
func FormatMAC(mac [6]byte) string {
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
}

// FormatIP retorna o IP virtual (host byte order) no formato dotted-quad.
func FormatIP(ip uint32) string {
	ipBytes := make(net.IP, 4)
	binary.BigEndian.PutUint32(ipBytes, ip)
	return fmt.Sprintf("%d.%d.%d.%d", ipBytes[0], ipBytes[1], ipBytes[2], ipBytes[3])
}

// PrintMac imprime o MAC com newline (atalho para depuração).
func PrintMac(mac *[6]byte) {
	fmt.Println(FormatMAC(*mac))
}

// PrintIP imprime o IP com newline (atalho para depuração).
func PrintIP(ip uint32) {
	fmt.Println(FormatIP(ip))
}

// ---------------------------------------------------------------------------
// Handshake HELLO (privado)
// ---------------------------------------------------------------------------

func (h *PhysicalHandler) doHello() error {
	pkt := make([]byte, 1+maxGroupID)
	pkt[0] = msgHello
	copy(pkt[1:], h.groupID)

	buf := make([]byte, 64)
	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := h.conn.WriteToUDP(pkt, h.serverAddr); err != nil {
			return fmt.Errorf("erro enviando HELLO: %w", err)
		}

		_ = h.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _, err := h.conn.ReadFromUDP(buf)
		_ = h.conn.SetReadDeadline(time.Time{})

		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Printf("Timeout aguardando HELLO_ACK (tentativa %d)\n", attempt)
				continue
			}
			return fmt.Errorf("erro lendo HELLO_ACK: %w", err)
		}

		if n < 11 || buf[0] != msgHelloAck {
			return fmt.Errorf("HELLO_ACK inesperado (type=0x%02X, len=%d)", buf[0], n)
		}

		copy(h.mac[:], buf[1:7])
		h.virtualIP = binary.BigEndian.Uint32(buf[7:11]) // network → host
		return nil
	}

	return errors.New("nó central não respondeu ao HELLO após 3 tentativas")
}
