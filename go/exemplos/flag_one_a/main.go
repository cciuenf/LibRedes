// flag_one_a — captura a Flag 1A do flag_server via physical_medium.
//
// Empilha duas camadas: IEEE 802.3 → IP. O payload do IP é o path
// desejado (ex: "/flag1a"). O tunnel no physical_medium faz a ponte
// HTTP internamente — o aluno só implementa enlace + IP.
//
//  1. HELLO          → registra o grupo (MAC + IP virtual) // FEITO
//  2. Constrói quadro IEEE 802.3 com pacote IP dentro 
//  3. Envia via PhySend para 10.0.0.1 (servidor)
//  4. Recebe um pacote IP com a flag no payload
//
// Para rodar:
//
//	physical_medium -flags-dir ../flag_server/flags
//	flag_server -addr :8080 -flags-dir ./flags
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
// Pilha de protocolos: IEEE 802.3 → IP
// =========================================================================
//
// Um quadro IEEE 802.3 transportando um pacote IP como payload:
//
//	Offset | Camada      | Campo                       | Valor
//	-------|-------------|-----------------------------|------------------------
//	  0    | IEEE 802.3  | Preâmbulo (7 bytes)         | 0xAA × 7
//	  7    | IEEE 802.3  | SFD (1 byte)                | 0xAB
//	  8    | IEEE 802.3  | MAC destino (6 bytes)       | FF:FF:FF:FF:FF:FF
//	 14    | IEEE 802.3  | MAC origem (6 bytes)        | srcMAC (do HELLO_ACK)
//	 20    | IEEE 802.3  | Tamanho (2 bytes, big-end.) | len(ipPkt)
//	 22    | IP          | Cabeçalho IPv4 (20 bytes)   | ver buildIPPacket
//	 42    | IP          | Payload (N bytes)           | ex: "/flag1a"
//	42+N   | IEEE 802.3  | FCS — CRC-32 (4 bytes)      | sobre offsets 0..41+N
//
// IP:  (Internet Protocol)         endereçamento e roteamento (RFC 791)
// FCS: (Frame Check Sequence)      4 bytes de verificação no final do quadro
// CRC: (Cyclic Redundancy Check)   algoritmo que gera o FCS (polinômio IEEE 802.3)

// ------------------------------------------------------------------------ //

// =========================================================================
// buildIPPacket — cabeçalho IPv4 mínimo (20 bytes, sem opções)
// =========================================================================
//
// O cabeçalho IP (RFC 791) tem 20 bytes obrigatórios. Cada campo é
// preenchido conforme a tabela abaixo. O checksum (offset 10) é o
// último campo — ele cobre o cabeçalho inteiro, então só pode ser
// calculado depois que todos os outros campos estão prontos.
//
// Campos do cabeçalho IP:
//
//	Offset | Bytes | Campo             | Valor / Explicação
//	-------|-------|-------------------|--------------------------------------
//	  0    |   1   | Version + IHL     | 0x45 = versão 4, IHL 5 (5×4=20 bytes)
//	  1    |   1   | DSCP + ECN        | 0x00 — sem prioridade (não usado aqui)
//	  2    |   2   | Total Length      | 20 + len(payload), em big-endian
//	  4    |   2   | Identification    | 0x0000 — sem fragmentação
//	  6    |   2   | Flags + Fragment  | 0x4000 — DF (Don't Fragment)
//	  8    |   1   | TTL               | 64 — número máximo de saltos
//	  9    |   1   | Protocol          | 0 — raw (o tunnel entende)
//	 10    |   2   | Header Checksum   | calculado por ipChecksum()
//	 12    |   4   | Source IP         | srcIP, em big-endian (ex: 10.0.0.2)
//	 16    |   4   | Destination IP    | dstIP, em big-endian (ex: 10.0.0.1)
//
// IP:   (Internet Protocol)                  protocolo de rede que endereça e roteia pacotes
// IHL:  (Internet Header Length)             tamanho do cabeçalho em palavras de 4 bytes
// DSCP: (Differentiated Services Code Point) prioridade do pacote (0 = normal)
// ECN:  (Explicit Congestion Notification)   notificação de congestionamento
//
// srcIP e dstIP estão em host byte order (uint32). A conversão para
// network byte order (big-endian) é feita por binary.BigEndian.PutUint32.
//
// O campo Header Checksum (bytes 10-11) começa zerado. O ipChecksum
// lê o cabeçalho com esses bytes em zero, calcula a soma, e o resultado
// é escrito DE VOLTA nos bytes 10-11.

// ------------------------------------------------------------------------ //

// =========================================================================
// ipChecksum — checksum de cabeçalho IP (RFC 791)
// =========================================================================
//
// Algoritmo:
//  1. Percorre o cabeçalho de 2 em 2 bytes, tratando cada par como
//     um número de 16 bits em big-endian.
//  2. Soma todos esses números em um acumulador de 32 bits (para
//     capturar o overflow).
//  3. "Dobra" o carry: os 16 bits superiores do acumulador são
//     somados de volta aos 16 bits inferiores.
//  4. Repete o passo 3 (pode gerar carry de novo).
//  5. Inverte todos os 16 bits (complemento de 1).
//
// O cabeçalho DEVE ter bytes 10-11 zerados antes de chamar esta função.
// Isso faz parte da especificação: o checksum é calculado como se o
// próprio campo de checksum fosse zero.
//
// Se o cabeçalho tiver tamanho ímpar (o que não acontece com IPv4
// sem opções), o último byte é tratado como byte alto de uma palavra
// de 16 bits com byte baixo zero.

// ------------------------------------------------------------------------ //

// =========================================================================
// extractIPPayload — extrai o payload de um pacote IPv4 recebido
// =========================================================================
//
// A resposta do servidor vem encapsulada em um pacote IP. Esta função
// descarta o cabeçalho e retorna só os dados (a flag).
//
// O tamanho do cabeçalho está no nibble inferior do primeiro byte:
//
//	pkt[0] = 0x45  →  versão = 4, IHL = 5
//	IHL = 5 × 4 = 20 bytes de cabeçalho
//
// Se o pacote for muito curto ou o IHL for inválido, retorna o pacote
// inteiro (fallback seguro).
