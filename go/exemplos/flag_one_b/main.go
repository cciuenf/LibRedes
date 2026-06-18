// flag_one_b — captura a Flag 1B do flag_server via physical_medium.
//
// Empilha TRÊS camadas: IEEE 802.3 → IP → TCP. O flag_server exige um
// handshake TCP completo (3-way handshake: SYN → SYN-ACK → ACK) antes de
// entregar a flag. Este arquivo descreve a estrutura — o aluno implementa.
//
//  1. HELLO          → registra o grupo (MAC + IP virtual) // FEITO
//  2. Constrói segmento TCP SYN (flags SYN=1, ACK=0)
//  3. Encapsula em IP (proto=6) e quadro IEEE 802.3
//  4. Envia SYN para 10.0.0.1 → recebe SYN-ACK
//  5. Constrói ACK final + requisição HTTP no payload TCP
//  6. Envia → recebe a flag na resposta TCP
//
// Para rodar:
//
//	physical_medium -flags-dir ../flag_server/flags
//	flag_server -addr :8080 -flags-dir ./flags
//
// Diferença crucial da Flag 1A:
//   Flag 1A → IP puro, o túnel extrai o path diretamente do payload IP
//   Flag 1B → IP + TCP, o túnel espera um handshake TCP antes de aceitar dados
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"log"

	"libphysical"
)

func main() {}

// =========================================================================
// Pilha de protocolos: IEEE 802.3 → IP → TCP
// =========================================================================
//
// Um quadro IEEE 802.3 transportando um pacote IP que transporta um
// segmento TCP com uma requisição HTTP:
//
//	Offset | Camada      | Campo                       | Valor
//	-------|-------------|-----------------------------|------------------------
//	  0    | IEEE 802.3  | Preâmbulo (7 bytes)         | 0xAA × 7
//	  7    | IEEE 802.3  | SFD (1 byte)                | 0xAB
//	  8    | IEEE 802.3  | MAC destino (6 bytes)       | FF:FF:FF:FF:FF:FF
//	 14    | IEEE 802.3  | MAC origem (6 bytes)        | srcMAC (do HELLO_ACK)
//	 20    | IEEE 802.3  | Tamanho (2 bytes, big-end.) | len(ipPkt)
//	 22    | IP          | Cabeçalho IPv4 (20 bytes)   | proto=6 (TCP)
//	 42    | TCP         | Cabeçalho TCP (20 bytes)    | ver buildTCPSegment
//	 62    | TCP         | Payload (N bytes)           | ex: "GET /flag1b ..."
//	62+N   | IEEE 802.3  | FCS — CRC-32 (4 bytes)      | sobre offsets 0..61+N
//
// IP:  (Internet Protocol)         endereçamento e roteamento (RFC 791)
// TCP: (Transmission Control)      confiabilidade, ordenação, handshake (RFC 793)
// FCS: (Frame Check Sequence)      4 bytes de verificação no final do quadro
// CRC: (Cyclic Redundancy Check)   algoritmo que gera o FCS (polinômio IEEE 802.3)

// =========================================================================
// TCP 3-Way Handshake (RFC 793, seção 3.4)
// =========================================================================
//
// Antes de enviar dados, TCP exige um handshake de 3 passos:
//
//   CLIENTE                                   SERVIDOR
//   ───────                                   ────────
//   ESTADO: CLOSED                            ESTADO: LISTEN
//
//   PASSO 1 — SYN
//   constrói segmento:
//     srcPort=12345  dstPort=80
//     seq=clientISN (ex: 32768)
//     ack=0
//     flags=SYN (0x02)
//     payload=vazio
//   encapsula em IP (proto=6) → 802.3
//   PhySend(10.0.0.1) ─────────────────────→  recebe, responde
//   ESTADO: SYN_SENT                           ESTADO: SYN_RECEIVED
//                                              envia SYN-ACK:
//                                                srcPort=80  dstPort=12345
//                                                seq=serverSeq (ex: 1000)
//                                                ack=clientISN+1
//                                                flags=SYN|ACK (0x12)
//
//   PASSO 2 — recebe SYN-ACK
//   PhyRecv() ←──────────────────────────────  (túnel respondeu)
//   valida: flags=SYN|ACK ✓, ack=clientISN+1 ✓
//   extrai serverSeq
//
//   PASSO 3 — ACK + dados
//   constrói segmento:
//     srcPort=12345  dstPort=80
//     seq=clientISN+1
//     ack=serverSeq+1
//     flags=ACK|PSH (0x18)
//     payload="GET /flag1b HTTP/1.0\r\nHost: flag_server\r\n\r\n"
//   encapsula em IP → 802.3
//   PhySend(10.0.0.1) ─────────────────────→  handshake completo!
//   ESTADO: ESTABLISHED                       ESTADO: ESTABLISHED
//                                              processa HTTP, responde flag
//
// SYN: (Synchronize)        flag que abre conexão (bit 1 do byte 13)
// ACK: (Acknowledgment)     flag que confirma recebimento (bit 4)
// PSH: (Push)               flag que solicita entrega imediata (bit 3)
// ISN: (Initial Sequence)   número de sequência inicial (32 bits, aleatório)
// seq: (Sequence Number)    offset do primeiro byte de dados neste segmento
// ack: (Acknowledgment Num) próximo byte esperado do lado oposto

// ------------------------------------------------------------------------ //

// =========================================================================
// Constantes TCP
// =========================================================================
//
// Portas: usamos 80 (HTTP padrão) para destino, 12345 (efêmera) para origem.
// Flags: cada flag é um bit no byte 13 do cabeçalho TCP.
// Combinam-se com | (OR bit-a-bit): SYN|ACK = 0x02 | 0x10 = 0x12.
// Portas: destino 80 (HTTP), origem 12345 (efêmera).
// Flags no byte 13: FIN=0x01, SYN=0x02, RST=0x04, PSH=0x08, ACK=0x10.
// Cabeçalho mínimo: 20 bytes (Data Offset = 5 palavras × 4 bytes).
// ISN do cliente: valor arbitrário de 32 bits (ex: 32768).

// ------------------------------------------------------------------------ //

// =========================================================================
// buildTCPSegment — constrói um segmento TCP (RFC 793)
// =========================================================================
//
// O cabeçalho TCP tem 20 bytes mínimos (Data Offset = 5 palavras):
//
//	Offset | Bytes | Campo               | Exemplo (SYN)  | Exemplo (ACK+PSH)
//	-------|-------|---------------------|----------------|-------------------
//	  0    |   2   | Source Port         | 12345          | 12345
//	  2    |   2   | Destination Port    | 80             | 80
//	  4    |   4   | Sequence Number     | 32768          | 32769
//	  8    |   4   | Acknowledgment Num  | 0              | 1001
//	 12    |   1   | Data Offset (4 bits)| 5 (0x50)       | 5 (0x50)
//	 13    |   1   | Flags (8 bits)      | 0x02 (SYN)     | 0x18 (ACK|PSH)
//	 14    |   2   | Window Size         | 65535          | 65535
//	 16    |   2   | Checksum            | (calculado)    | (calculado)
//	 18    |   2   | Urgent Pointer      | 0              | 0
//	 20    |   N   | Payload             | —              | "GET /flag1b..."
//
// Data Offset: nibble superior do byte 12.
//   Ex: 5 palavras × 4 bytes = 20 bytes → byte 12 = 5 << 4 = 0x50.
//
// ⚠️ O checksum TCP (bytes 16-17) DEVE ficar zerado aqui.
//    Ele será calculado depois por buildIPPacket, porque depende
//    dos IPs de origem e destino (pseudo-header).
//
// Assinatura esperada:
//
//	func buildTCPSegment(srcPort, dstPort uint16, seq, ack uint32,
//	                     flags uint8, payload []byte) []byte

// ------------------------------------------------------------------------ //

// =========================================================================
// buildIPPacketWithTCP — cabeçalho IPv4 (20 bytes) + segmento TCP
// =========================================================================
//
// Igual ao flag_one_a, mas protocol=6 (TCP) em vez de 0 (raw).
//
// Campos do cabeçalho IP (RFC 791):
//
//	Offset | Bytes | Campo             | Valor
//	-------|-------|-------------------|-------------------------------
//	  0    |   1   | Version + IHL     | 0x45 (IPv4, IHL=5)
//	  2    |   2   | Total Length      | 20 + len(tcpSegment)
//	  6    |   2   | Flags + Fragment  | 0x4000 (DF — Don't Fragment)
//	  8    |   1   | TTL               | 64
//	  9    |   1   | Protocol          | 6 = TCP (antes era 0 = raw)
//	 10    |   2   | Header Checksum   | calculado por ipChecksum()
//	 12    |   4   | Source IP         | srcIP, em big-endian
//	 16    |   4   | Destination IP    | dstIP, em big-endian
//
// ⚠️ Além do checksum IP, esta função também calcula o checksum TCP.
//    O checksum TCP cobre: pseudo-header (12 bytes) + segmento TCP.
//
// Pseudo-header TCP (12 bytes, NÃO transmitido — só para checksum):
//
//	Offset | Bytes | Campo          | Valor
//	-------|-------|----------------|-------------------------
//	  0    |   4   | Source IP      | srcIP (big-endian)
//	  4    |   4   | Destination IP | dstIP (big-endian)
//	  8    |   1   | Zero           | 0x00
//	  9    |   1   | Protocol       | 0x06 (TCP)
//	 10    |   2   | TCP Length     | len(tcpSegment)
//
// Assinatura esperada:
//
//	func buildIPPacketWithTCP(srcIP, dstIP uint32, tcpSeg []byte) []byte

// ------------------------------------------------------------------------ //

// =========================================================================
// tcpChecksum — checksum TCP (RFC 793)
// =========================================================================
//
// Algoritmo idêntico ao ipChecksum (complemento de 1 da soma de palavras
// de 16 bits), mas calculado sobre pseudo-header + segmento TCP.
//
// Passos:
//  1. Concatena pseudo-header (12 bytes) + segmento TCP
//  2. Soma palavras de 16 bits em acumulador de 32 bits
//  3. Se tamanho ímpar, último byte tratado como byte alto
//  4. Dobra o carry: (sum >> 16) + (sum & 0xFFFF), repete
//  5. Complemento de 1: ^uint16(sum)
//
// PRÉ-CONDIÇÃO: bytes 16-17 do cabeçalho TCP devem estar zerados.
//
// Assinatura esperada:
//
//	func tcpChecksum(pseudo, tcpSeg []byte) uint16

// ------------------------------------------------------------------------ //

// =========================================================================
// buildTCPPseudoHeader — pseudo-header de 12 bytes (RFC 793)
// =========================================================================
//
// Constrói o pseudo-header usado no cálculo do checksum TCP.
// Ele NÃO é transmitido — existe apenas para o checksum detectar
// erros de roteamento (entrega ao IP errado).
//
// Assinatura esperada:
//
//	func buildTCPPseudoHeader(srcIP, dstIP uint32, tcpSeg []byte) []byte

// ------------------------------------------------------------------------ //

// =========================================================================
// ipChecksum — checksum de cabeçalho IP (RFC 791)
// =========================================================================
//
// Mesmo algoritmo do flag_one_a.
// O cabeçalho DEVE ter bytes 10-11 zerados antes da chamada.
//
// Assinatura esperada:
//
//	func ipChecksum(hdr []byte) uint16

// ------------------------------------------------------------------------ //

// =========================================================================
// extractFramePayload — extrai payload de dentro do quadro IEEE 802.3
// =========================================================================
//
// Remove preâmbulo(7)+SFD(1)+MACs(12)+tamanho(2) do início e CRC(4) do
// final, retornando apenas o pacote IP contido no quadro.
//
// Um quadro válido tem no mínimo 26 bytes (22 fixos + 4 CRC).
//
// Assinatura esperada:
//
//	func extractFramePayload(frame []byte) []byte

// ------------------------------------------------------------------------ //

// =========================================================================
// extractIPPayload — extrai payload de dentro do pacote IPv4
// =========================================================================
//
// O tamanho do cabeçalho IP está no nibble inferior do byte 0 (IHL).
// Ex: pkt[0] = 0x45 → IHL = 5 × 4 = 20 bytes → payload começa no byte 20.
//
// Assinatura esperada:
//
//	func extractIPPayload(pkt []byte) []byte

// ------------------------------------------------------------------------ //

// =========================================================================
// extractTCPPayload — extrai payload de dentro do segmento TCP
// =========================================================================
//
// O tamanho do cabeçalho TCP está no nibble superior do byte 12 (Data Offset).
// Ex: seg[12] >> 4 = 5 → 5 × 4 = 20 bytes → payload começa no byte 20.
//
// Assinatura esperada:
//
//	func extractTCPPayload(seg []byte) []byte

// ------------------------------------------------------------------------ //

// =========================================================================
// Helpers de parsing TCP
// =========================================================================

// hasFlag — verifica se uma flag está ligada no byte 13 do cabeçalho TCP.
//   func hasFlag(seg []byte, flag uint8) bool

// getTCPSeq — extrai Sequence Number dos bytes 4-7 do cabeçalho TCP.
//   func getTCPSeq(seg []byte) uint32

// getTCPAck — extrai Acknowledgment Number dos bytes 8-11 do cabeçalho TCP.
//   func getTCPAck(seg []byte) uint32
