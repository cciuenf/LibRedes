// =============================================================================
// helpers.go — funções de depuração e visualização dos protocolos
// =============================================================================
//
// Este arquivo contém funções que imprimem na tela o conteúdo de cada
// camada de protocolo (IEEE 802.3, IP, TCP) em formato legível.
//
// Elas NÃO são necessárias para capturar a flag — são puramente educativas,
// para que o aluno possa VER os bytes que estão sendo enviados e recebidos,
// entender a estrutura de cada cabeçalho e depurar problemas.
//
// As funções seguem o fluxo de encapsulamento (de fora para dentro):
//   printFrame       → mostra quadro 802.3, depois chama printIPPacket
//   printIPPacket    → mostra cabeçalho IP, depois chama printTCPHeader
//   printTCPHeader   → mostra cabeçalho TCP + payload
//   printTCPSegment  → atalho para mostrar só o TCP (sem frame/IP)
//   printConnection  → mostra MAC e IP virtual obtidos do HELLO_ACK

package main

import (
	"fmt"

	"libphysical"
)

// =============================================================================
// printFrame — exibe um quadro IEEE 802.3 completo com todas as camadas
// =============================================================================
//
// A saída é formatada em seções hierárquicas:
//
//	=== Título ===
//	  Preâmbulo:     [ 0: 6] AA AA AA AA AA AA AA
//	  SFD:           [ 7: 7] AB
//	  MAC destino:   [ 8:13] FF FF FF FF FF FF
//	  MAC origem:    [14:19] 02 00 00 00 00 02
//	  Tamanho:       [20:21] 00 3C
//	  Payload:       [22:81] 60 bytes (pacote IP)
//	    IP: ver=4 IHL=5 len=60 proto=6 TTL=64
//	    IP: 10.0.0.2 → 10.0.0.1
//	    TCP: 12345 → 80  seq=32768  ack=0  flags=[SYN ] window=65535 dataOffset=20
//	  FCS (CRC-32):   [82:85] A1 B2 C3 D4
//
// PARÂMETROS:
//
//	frame: bytes brutos do quadro 802.3 completo
//	title: título descritivo (ex: "Quadro IEEE 802.3 enviado")
func printFrame(frame []byte, title string) {
	fmt.Printf("=== %s ===\n", title)

	// ────────────────────────────────────────────────────────────────
	// Seção 1: Cabeçalho do quadro IEEE 802.3 (22 bytes fixos)
	// ────────────────────────────────────────────────────────────────
	// Mostra cada campo do envelope Ethernet com seu offset [início:fim]
	offsets := []struct {
		name   string // nome do campo
		start  int    // offset inicial (0-indexed)
		length int    // tamanho em bytes
	}{
		{"Preâmbulo", 0, 7},   // bytes 0-6: 7 × 0xAA
		{"SFD", 7, 1},         // byte  7:   0xAB
		{"MAC destino", 8, 6}, // bytes 8-13: FF:FF:FF:FF:FF:FF
		{"MAC origem", 14, 6}, // bytes 14-19: MAC virtual do grupo
		{"Tamanho", 20, 2},    // bytes 20-21: tamanho do payload em big-endian
	}
	for _, f := range offsets {
		fmt.Printf("  %-14s [%2d:%2d] % X\n",
			f.name+":",
			f.start,
			f.start+f.length-1,
			frame[f.start:f.start+f.length])
	}

	// ────────────────────────────────────────────────────────────────
	// Seção 2: Payload (pacote IP) — entre o header 802.3 e o CRC
	// ────────────────────────────────────────────────────────────────
	// O payload começa no byte 22 e termina 4 bytes antes do final (CRC).
	payload := frame[22 : len(frame)-4]
	fmt.Printf("  %-14s [%2d:%2d] %d bytes (pacote IP)\n",
		"Payload:", 22, 22+len(payload)-1, len(payload))

	// Se o payload tiver pelo menos 20 bytes, tenta interpretar como IP
	if len(payload) >= 20 {
		printIPPacket(payload)
	}

	// ────────────────────────────────────────────────────────────────
	// Seção 3: FCS (CRC-32) — últimos 4 bytes do quadro
	// ────────────────────────────────────────────────────────────────
	fcsStart := len(frame) - 4
	fmt.Printf("  %-14s [%2d:%2d] % X\n",
		"FCS (CRC-32):",
		fcsStart, fcsStart+3,
		frame[fcsStart:fcsStart+4])
}

// =============================================================================
// printIPPacket — exibe o conteúdo de um pacote IPv4
// =============================================================================
//
// Extrai e formata cada campo do cabeçalho IP (RFC 791).
// Se o protocolo for TCP (campo Protocol=6), também mostra o segmento TCP.
//
// CAMPOS EXIBIDOS:
//   ver      — versão do IP (4 = IPv4)
//   IHL      — tamanho do cabeçalho em palavras de 32 bits (5 = 20 bytes)
//   len      — tamanho total do pacote (cabeçalho + payload)
//   proto    — protocolo da camada superior (6 = TCP, 17 = UDP, 0 = raw)
//   TTL      — time to live (máximo de saltos, decrementado por roteadores)
//   src → dst — IPs de origem e destino em notação dotted-quad
//
// EXEMPLO DE SAÍDA:
//     IP: ver=4 IHL=5 len=60 proto=6 TTL=64
//     IP: 10.0.0.2 → 10.0.0.1
//       TCP: 12345 → 80  seq=32768  ack=0  flags=[SYN ] window=65535

func printIPPacket(pkt []byte) {
	// ── Extrai campos do cabeçalho IP ──

	// Byte 0: Version (nibble superior) + IHL (nibble inferior)
	//   Ex: 0x45 → ver=4, IHL=5
	ver := pkt[0] >> 4         // nibble superior = versão
	ihl := (pkt[0] & 0x0F) * 4 // nibble inferior × 4 = tamanho em bytes

	// Bytes 2-3: Total Length (big-endian)
	totalLen := int(pkt[2])<<8 | int(pkt[3])

	// Byte 8: TTL
	ttl := pkt[8]

	// Byte 9: Protocol (6 = TCP, 17 = UDP, 1 = ICMP)
	protocol := pkt[9]

	// Bytes 12-15: Source IP (32 bits)
	src := fmt.Sprintf("%d.%d.%d.%d", pkt[12], pkt[13], pkt[14], pkt[15])

	// Bytes 16-19: Destination IP (32 bits)
	dst := fmt.Sprintf("%d.%d.%d.%d", pkt[16], pkt[17], pkt[18], pkt[19])

	fmt.Printf("    IP: ver=%d IHL=%d len=%d proto=%d TTL=%d\n",
		ver, ihl, totalLen, protocol, ttl)
	fmt.Printf("    IP: %s → %s\n", src, dst)

	// ── Mostra o payload do IP (segmento TCP, se protocol=6) ──
	ipPayload := pkt[ihl:] // payload começa depois do cabeçalho IP
	if len(ipPayload) >= 20 && protocol == 6 {
		printTCPHeader(ipPayload)
	} else if len(ipPayload) > 0 {
		// Modo raw (Flag 1A) ou protocolo não-TCP
		fmt.Printf("    Payload: %q\n", string(ipPayload))
	}
}

// =============================================================================
// printTCPHeader — exibe o conteúdo de um segmento TCP
// =============================================================================
//
// Extrai e formata cada campo do cabeçalho TCP (RFC 793).
// Se houver payload, mostra os primeiros bytes como string.
//
// CAMPOS EXIBIDOS:
//   srcPort → dstPort — portas de origem e destino
//   seq                — número de sequência
//   ack                — número de acknowledgement
//   flags              — quais flags estão ligadas (SYN, ACK, PSH, RST, FIN)
//   window             — tamanho da janela de recepção
//   dataOffset         — tamanho do cabeçalho TCP em bytes
//
// EXEMPLO DE SAÍDA:
//     TCP: 12345 → 80  seq=32768  ack=0  flags=[SYN ] window=65535 dataOffset=20
//     TCP payload (43 bytes): "GET /flag2 HTTP/1.0\r\nHost: flag_server\r\n\r\n"

func printTCPHeader(seg []byte) {
	if len(seg) < 20 {
		fmt.Printf("    TCP: segmento muito curto (%d bytes)\n", len(seg))
		return
	}

	// ── Extrai campos do cabeçalho TCP ──

	// Bytes 0-1: Source Port (uint16 big-endian)
	srcPort := uint16(seg[0])<<8 | uint16(seg[1])

	// Bytes 2-3: Destination Port (uint16 big-endian)
	dstPort := uint16(seg[2])<<8 | uint16(seg[3])

	// Bytes 4-7: Sequence Number (uint32 big-endian)
	seq := uint32(seg[4])<<24 | uint32(seg[5])<<16 | uint32(seg[6])<<8 | uint32(seg[7])

	// Bytes 8-11: Acknowledgment Number (uint32 big-endian)
	ack := uint32(seg[8])<<24 | uint32(seg[9])<<16 | uint32(seg[10])<<8 | uint32(seg[11])

	// Byte 12, nibble superior: Data Offset (palavras de 32 bits → bytes)
	dataOffset := (seg[12] >> 4) * 4

	// Byte 13: Flags
	flags := seg[13]

	// Bytes 14-15: Window Size (uint16 big-endian)
	window := uint16(seg[14])<<8 | uint16(seg[15])

	// ── Formata flags como string legível ──
	// Cada flag é um bit. Verificamos um por um e montamos uma string.
	flagStrs := ""
	if flags&0x01 != 0 { // bit 0: FIN
		flagStrs += "FIN "
	}
	if flags&0x02 != 0 { // bit 1: SYN
		flagStrs += "SYN "
	}
	if flags&0x04 != 0 { // bit 2: RST
		flagStrs += "RST "
	}
	if flags&0x08 != 0 { // bit 3: PSH
		flagStrs += "PSH "
	}
	if flags&0x10 != 0 { // bit 4: ACK
		flagStrs += "ACK "
	}
	if flagStrs == "" {
		flagStrs = "(nenhuma)"
	}

	// ── Imprime campos do TCP ──
	fmt.Printf("    TCP: %d → %d  seq=%d  ack=%d  flags=[%s] window=%d dataOffset=%d\n",
		srcPort, dstPort, seq, ack, flagStrs, window, dataOffset)

	// ── Mostra payload TCP (se houver) ──
	payload := seg[dataOffset:]
	if len(payload) > 0 {
		// Se for texto puro, mostra como string.
		// Se tiver bytes não-imprimíveis, mostra em hexadecimal também.
		if isPrintable(payload) {
			fmt.Printf("    TCP payload (%d bytes): %q\n", len(payload), string(payload))
		} else {
			// Mostra primeiros 32 bytes em hex + tenta mostrar como string
			showLen := len(payload)
			if showLen > 32 {
				showLen = 32
			}
			fmt.Printf("    TCP payload (%d bytes): % X ...\n", len(payload), payload[:showLen])
		}
	}
}

// =============================================================================
// printTCPSegment — atalho para mostrar um segmento TCP isolado
// =============================================================================
//
// Usado durante as fases do handshake para mostrar cada segmento TCP
// enviado/recebido sem repetir a hierarquia completa do quadro.
//
// EXEMPLO DE SAÍDA:
//
//	--- TCP SYN enviado ---
//	  TCP: 12345 → 80  seq=32768  ack=0  flags=[SYN ] window=65535 dataOffset=20
func printTCPSegment(label string, seg []byte) {
	fmt.Printf("  ─── %s ───\n", label)
	printTCPHeader(seg)
}

// =============================================================================
// printConnection — mostra os dados de identidade obtidos no HELLO_ACK
// =============================================================================
//
// O physical_medium atribui um MAC virtual (6 bytes) e um IP virtual (4 bytes)
// a cada grupo que se registra. Esta função imprime ambos.
//
// EXEMPLO DE SAÍDA:
//
//	=== Conectado ao nó central ===
//	  MAC virtual: 02:00:00:00:00:02
//	  IP virtual:  10.0.0.2
//
// Formato do MAC:   02:00:00:00:00:02 (6 pares hex separados por :)
// Formato do IP:    10.0.0.2           (notação dotted-quad)
func printConnection(phy *libphysical.PhysicalHandler) {
	fmt.Println("=== Conectado ao nó central ===")

	fmt.Print("  MAC virtual: ")
	mac := phy.PhyMacAddr()
	// PrintMac imprime no formato xx:xx:xx:xx:xx:xx
	libphysical.PrintMac(&mac)

	fmt.Print("  IP virtual:  ")
	// PrintIP converte uint32 (host byte order) para dotted-quad
	libphysical.PrintIP(phy.PhyVirtualIP())
	fmt.Println()
}

// =============================================================================
// isPrintable — verifica se um slice de bytes é texto imprimível
// =============================================================================
//
// Considera imprimível se todos os bytes estão na faixa 0x20-0x7E (ASCII
// visível) ou são caracteres de controle comuns (\r, \n, \t).
//
// Usado para decidir se mostramos o payload TCP como string ou como hex dump.
func isPrintable(data []byte) bool {
	for _, b := range data {
		if b < 0x20 && b != '\r' && b != '\n' && b != '\t' {
			return false
		}
		if b > 0x7E {
			return false
		}
	}
	return true
}
