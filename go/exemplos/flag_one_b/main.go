// =============================================================================
// flag_one_b — captura a Flag 1B do flag_server implementando TCP do zero.
// =============================================================================
//
// PROGRESSÃO DAS FLAGS (o que cada uma exige):
//
//	Flag 0:  IEEE 802.3 puro.
//	         Só o quadro Ethernet. O setup mode do physical_medium valida
//	         preâmbulo, SFD, MAC origem e CRC-32. Se passar, entrega a flag.
//	         setup_mode.go → ValidateFrame()
//
//	Flag 1A: IEEE 802.3 → IP (raw, protocol=0).
//	         Empilha IP sobre Ethernet. O payload do IP é uma string de path
//	         (ex: "/flag1a"). O túnel no physical_medium lê esse path,
//	         faz HTTP GET ao flag_server e devolve a resposta.
//	         tunnel.go → handleRaw()
//
//	Flag 1B: IEEE 802.3 → IP → TCP (protocol=6).
//	         Empilha TCP sobre IP sobre Ethernet. O túnel agora espera um
//	         handshake TCP de 3 vias (SYN → SYN-ACK → ACK) antes de aceitar
//	         qualquer dado. Só depois do handshake completo é que os dados
//	         (a requisição HTTP) são processados.
//	         tunnel.go → handleTCP()
//
// PILHA DE PROTOCOLOS — como os bytes são empacotados:
//
//	┌──────────────────────────────────────────────────┐
//	│ Quadro IEEE 802.3                                │
//	│ ┌──────────────────────────────────────────────┐ │
//	│ │ Preâmbulo (7)│ SFD │ MAC dst │ MAC src │ Len │ │
//	│ │  0xAA × 7    │0xAB │ 6 bytes │ 6 bytes │ 2B  │ │
//	│ ├──────────────────────────────────────────────┤ │
//	│ │ Pacote IPv4                                  │ │
//	│ │ ┌──────────────────────────────────────────┐ │ │
//	│ │ │ IP Header (20 bytes)                     │ │ │
//	│ │ │ ver=4, IHL=5, proto=6(TCP), TTL=64       │ │ │
//	│ │ ├──────────────────────────────────────────┤ │ │
//	│ │ │ Segmento TCP                             │ │ │
//	│ │ │ ┌──────────────────────────────────────┐ │ │ │
//	│ │ │ │ TCP Header (20 bytes)                │ │ │ │
//	│ │ │ │ srcPort, dstPort, seq, ack, flags    │ │ │ │
//	│ │ │ ├──────────────────────────────────────┤ │ │ │
//	│ │ │ │ TCP Payload                          │ │ │ │
//	│ │ │ │ "GET /flag1b HTTP/1.0\r\n..."        │ │ │ │
//	│ │ │ └──────────────────────────────────────┘ │ │ │
//	│ │ └──────────────────────────────────────────┘ │ │
//	│ └──────────────────────────────────────────────┘ │
//	│ FCS / CRC-32 (4 bytes)                           │
//	└──────────────────────────────────────────────────┘
//
// TCP 3-WAY HANDSHAKE (RFC 793, seção 3.4):
//
//	CLIENTE                                   SERVIDOR (túnel)
//	───────                                   ────────────────
//	ESTADO: CLOSED                            ESTADO: LISTEN
//
//	PASSO 1 — SYN
//	constrói segmento:
//	  srcPort=12345  dstPort=80
//	  seq=42 (ISN)   ack=0
//	  flags=SYN
//	  payload=vazio
//	encapsula em IP (proto=6)
//	encapsula em 802.3
//	PhySend(10.0.0.1) ─────────────────────→  recebe, cria conexão
//	ESTADO: SYN_SENT                          ESTADO: SYN_RECEIVED
//	                                          responde SYN-ACK:
//	                                          srcPort=80  dstPort=12345
//	                                          seq=1000     ack=43
//	                                          flags=SYN|ACK
//
//	PASSO 2 — recebe SYN-ACK
//	PhyRecv() ←────────────────────────────── (enviado pelo túnel)
//	valida: flags=SYN|ACK, ack=43 ✓
//	extrai serverSeq=1000
//	ESTADO: ESTABLISHED (cliente)
//
//	PASSO 3 — ACK + dados
//	constrói segmento:
//	  srcPort=12345  dstPort=80
//	  seq=43         ack=1001
//	  flags=ACK|PSH
//	  payload="GET /flag1b HTTP/1.0\r\n..."
//	encapsula em IP → 802.3
//	PhySend(10.0.0.1) ─────────────────────→  handshake completo!
//	ESTADO: ESTABLISHED                       ESTADO: ESTABLISHED
//	                                          processa HTTP GET /flag1b
//	                                          responde com a flag:
//	                                          flags=ACK|PSH
//	                                          payload="FLAG1B{...}"
//
//	PASSO 4 — recebe resposta
//	PhyRecv() ←────────────────────────────── (flag dentro do TCP payload)
//	extrai TCP payload → imprime a flag
//
// CONCEITOS ESSENCIAIS DO TCP:
//
//	ISN  (Initial Sequence Number): número inicial aleatório.
//	      Cliente e servidor escolhem cada um o seu.
//	      Aqui: cliente=42, servidor=1000.
//
//	seq  (Sequence Number): offset do primeiro byte de dados neste segmento
//	      dentro do fluxo. O SYN consome 1 número de sequência.
//
//	ack  (Acknowledgment Number): próximo byte que o remetente espera RECEBER.
//	      Ex: se recebi bytes até seq=42, meu ack será 43.
//
//	Flags: bits no byte 13 do cabeçalho TCP.
//	       SYN=0x02  (synchronize — abre conexão)
//	       ACK=0x10  (acknowledgment — confirma recebimento)
//	       PSH=0x08  (push — entrega imediatamente à aplicação)
//	       RST=0x04  (reset — aborta conexão)
//	       FIN=0x01  (finish — encerra conexão)
//
//	Window: espaço de buffer disponível para receber dados (aqui: 65535).
//
// PARA RODAR ESTE EXEMPLO:
//
//	Terminal 1 — nó central:
//	  cd go/physical_medium
//	  go run . -udp :9000 -ctrl :9090 -profiles profiles/clean.json
//
//	Terminal 2 — servidor de flags:
//	  cd go/flag_server
//	  go run . -addr :8080 -flags-dir ./flags
//
//	Terminal 3 — este exemplo:
//	  cd go/Exemplos/flag_one_b
//	  go run .
//
// REFERÊNCIAS:
//
//	RFC 793  — Transmission Control Protocol
//	RFC 791  — Internet Protocol
//	IEEE 802.3 — Ethernet Frame Format
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"

	"libphysical"
)

// =============================================================================
// CONSTANTES DO PROTOCOLO TCP
// =============================================================================
//
// Cada constante representa um campo ou valor fixo usado na construção
// dos segmentos TCP. Os valores seguem o RFC 793.

const (
	// --- Portas TCP ---
	// O servidor HTTP do túnel escuta na porta 80 (padrão HTTP).
	// O cliente escolhe uma porta efêmera qualquer (aqui: 12345).
	tcpPortHTTP   = 80    // porta de destino = porta do servidor HTTP
	tcpPortClient = 12345 // porta de origem = porta efêmera do cliente

	// --- Flags TCP (byte 13 do cabeçalho) ---
	// Cada flag ocupa 1 bit. Múltiplas flags podem ser combinadas com | (OR).
	// Ex: SYN|ACK = 0x02 | 0x10 = 0x12
	tcpFlagFIN = 0x01 // bit 0: FIN — encerra conexão (Finish)
	tcpFlagSYN = 0x02 // bit 1: SYN — abre conexão (Synchronize)
	tcpFlagRST = 0x04 // bit 2: RST — aborta conexão (Reset)
	tcpFlagPSH = 0x08 // bit 3: PSH — push, entrega imediata (Push)
	tcpFlagACK = 0x10 // bit 4: ACK — confirma recebimento (Acknowledgment)
	// bits 5-7: CWR, ECE, URG — não usados neste exemplo

	// --- Tamanho do cabeçalho TCP ---
	// Data Offset mínimo = 5 palavras de 32 bits = 5 × 4 = 20 bytes.
	// Sem opções TCP, o cabeçalho tem exatamente 20 bytes.
	tcpHeaderLen = 20

	// --- ISN do cliente ---
	// Número de sequência inicial (Initial Sequence Number).
	// No TCP real seria aleatório. Aqui usamos 42 para depuração.
	clientISN = 32768 // ISN do cliente (arbitrário, mas fácil de identificar)
)

func main() {}

// =============================================================================
// buildTCPSegment — constrói um segmento TCP bruto (RFC 793)
// =============================================================================
//
// Esta função aloca um []byte com o cabeçalho TCP (20 bytes) + payload
// e preenche cada campo conforme os parâmetros.
//
// ESTRUTURA DO CABEÇALHO TCP (20 bytes mínimos, sem opções):
//
//   Offset | Bytes | Campo               | Exemplo (SYN)  | Exemplo (ACK+data)
//   ───────|───────|─────────────────────|────────────────|───────────────────
//    0     |   2   | Source Port         | 12345          | 12345
//    2     |   2   | Destination Port    | 80             | 80
//    4     |   4   | Sequence Number     | 32768          | 32769
//    8     |   4   | Acknowledgment Num  | 0              | 1001
//   12     |   1   | Data Offset (4 bits)| 5              | 5
//          |       | + Reserved (3 bits) |                |
//          |       | + NS (1 bit)        |                |
//   13     |   1   | Flags (8 bits)      | 0x02 (SYN)     | 0x18 (ACK|PSH)
//          |       |  CWR(0x80)          |                |
//          |       |  ECE(0x40)          |                |
//          |       |  URG(0x20)          |                |
//          |       |  ACK(0x10)          |                |
//          |       |  PSH(0x08)          |                |
//          |       |  RST(0x04)          |                |
//          |       |  SYN(0x02)          |                |
//          |       |  FIN(0x01)          |                |
//   14     |   2   | Window Size         | 65535          | 65535
//   16     |   2   | Checksum            | (calculado)    | (calculado)
//   18     |   2   | Urgent Pointer      | 0              | 0
//   20     |   N   | Payload (opcional)  | (vazio)        | "GET /flag1b..."
//
// O CAMPO DATA OFFSET (byte 12, nibble superior):
//   Indica o tamanho do cabeçalho TCP em palavras de 32 bits (4 bytes).
//   Sem opções: 5 palavras × 4 = 20 bytes.
//   Com opções (ex: Timestamp, MSS): pode ser 6, 7, 8...
//   O nibble superior do byte 12 armazena esse valor (5 << 4 = 0x50).
//
// O CAMPO WINDOW SIZE (bytes 14-15):
//   Indica quanto espaço de buffer o remetente tem para receber dados.
//   65535 é o máximo sem window scale option.
//   Para este exemplo didático, window size não é crítico.
//
// CHECKSUM TCP (bytes 16-17):
//   ⚠️ Este campo NÃO é preenchido aqui! Fica com zero.
//   Motivo: o checksum TCP depende dos IPs de origem e destino
//   (pseudo-header), que só são conhecidos em buildIPPacket().
//   buildIPPacket() chama tcpChecksum() e preenche os bytes 16-17.

// =============================================================================
// buildIPPacket — constrói um pacote IPv4 com checksums IP e TCP corretos
// =============================================================================
//
// DIFERENÇA CRUCIAL DO FLAG 1A:
//   Na Flag 1A, o campo Protocol do IP era 0 (raw) e o payload do IP
//   era uma string de path (ex: "/flag1a").
//   Na Flag 1B, o campo Protocol é 6 (TCP) e o payload do IP é um
//   segmento TCP completo (cabeçalho + payload).
//
// ESTRUTURA DO CABEÇALHO IP (20 bytes, sem opções):
//
//   Offset | Bytes | Campo            | Valor p/ TCP
//   ───────|───────|──────────────────|────────────────────────
//    0     |   1   | Version + IHL    | 0x45 (IPv4, IHL=5)
//    1     |   1   | DSCP + ECN      | 0x00
//    2     |   2   | Total Length     | 20 + len(tcpSegment)
//    4     |   2   | Identification   | 0x0000
//    6     |   2   | Flags + Fragment | 0x4000 (DF=1, Fragment=0)
//    8     |   1   | TTL              | 64
//    9     |   1   | Protocol         | 6 (= TCP)
//   10     |   2   | Header Checksum  | (calculado)
//   12     |   4   | Source IP        | IP virtual do grupo
//   16     |   4   | Destination IP   | 10.0.0.1
//
// O CHECKSUM IP (bytes 10-11):
//   Cobre apenas o cabeçalho IP (20 bytes).
//   O campo de checksum (bytes 10-11) é zerado antes do cálculo.
//   Algoritmo: complemento de 1 da soma de palavras de 16 bits.
//
// O CHECKSUM TCP (bytes 16-17 do segmento TCP):
//   Esta função também calcula o checksum TCP se protocol=6.
//   O checksum TCP cobre: pseudo-header + cabeçalho TCP + payload TCP.
//   O pseudo-header inclui os IPs para detectar erros de roteamento.
//
// PARÂMETROS:
//   srcIP, dstIP: em HOST byte order (uint32)
//   protocol:     6 = TCP, 0 = raw (Flag 1A), 1 = ICMP, 17 = UDP
//   payload:      segmento TCP completo (cabeçalho + dados)

// =============================================================================
// buildTCPPseudoHeader — pseudo-header de 12 bytes para o checksum TCP
// =============================================================================
//
// O TCP usa um "pseudo-header" no cálculo do checksum para detectar
// erros de roteamento: se o pacote IP for entregue ao endereço errado,
// o checksum TCP vai falhar porque os IPs no pseudo-header não batem.
//
// ESTRUTURA DO PSEUDO-HEADER (12 bytes, NÃO transmitido na rede):
//
//   Offset | Bytes | Campo          | Origem do valor
//   ───────|───────|────────────────|─────────────────────────────
//    0     |   4   | Source IP      | srcIP (parâmetro)
//    4     |   4   | Destination IP | dstIP (parâmetro)
//    8     |   1   | Zero           | sempre 0x00 (reservado)
//    9     |   1   | Protocol       | sempre 0x06 (TCP)
//   10     |   2   | TCP Length     | len(tcpSegment) = header+payload
//
// O pseudo-header é prefixado ao segmento TCP apenas para o cálculo
// do checksum. Ele NÃO faz parte do pacote transmitido.

// =============================================================================
// tcpChecksum — calcula o checksum TCP (RFC 793)
// =============================================================================
//
// O algoritmo é EXATAMENTE o mesmo do checksum IP (complemento de 1
// da soma de palavras de 16 bits), mas aplicado sobre:
//   pseudo-header (12 bytes) + segmento TCP (header + payload)
//
// ALGORITMO PASSO A PASSO:
//
//   1. Concatena pseudo-header + segmento TCP em um buffer
//   2. Percorre o buffer de 2 em 2 bytes
//   3. Cada par de bytes é interpretado como um uint16 big-endian
//   4. Soma todos os uint16 em um acumulador de 32 bits (para capturar overflow)
//   5. Se o buffer tiver tamanho ímpar, o último byte é tratado como
//      byte alto de uma palavra com byte baixo = 0x00
//   6. "Dobra o carry": soma os 16 bits superiores do acumulador
//      nos 16 bits inferiores
//   7. Repete o passo 6 (pode gerar carry novamente)
//   8. Inverte todos os 16 bits (complemento de 1)
//
// EXEMPLO MANUAL (dados: 0x00 0x06 0x00 0x28):
//   palavra 1: 0x0006 = 6
//   palavra 2: 0x0028 = 40
//   soma: 6 + 40 = 46 = 0x002E
//   carry: 0 (46 cabe em 16 bits)
//   complemento: ~0x002E = 0xFFD1
//
// PRÉ-CONDIÇÃO:
//   O campo checksum dentro do cabeçalho TCP (bytes 16-17 do segmento)
//   DEVE estar zerado. buildTCPSegment() já garante isso.

// =============================================================================
// buildIEEE8023Frame — constrói um quadro Ethernet (IEEE 802.3)
// =============================================================================
//
// ESTRUTURA DO QUADRO (MESMA DA FLAG 0 E FLAG 1A):
//
//   Offset | Bytes | Campo         | Valor
//   ───────|───────|───────────────|────────────────────────────────
//     0    |   7   | Preâmbulo     | 0xAA × 7
//     7    |   1   | SFD           | 0xAB
//     8    |   6   | MAC Destino   | FF:FF:FF:FF:FF:FF (broadcast)
//    14    |   6   | MAC Origem    | MAC virtual do grupo
//    20    |   2   | Tamanho       | len(payload) em big-endian
//    22    |   N   | Payload       | o pacote IP completo
//  22+N    |   4   | FCS (CRC-32)  | sobre bytes 0 a 21+N
//
// PREÂMBULO (7 × 0xAA):
//   Padrão de bits alternados (10101010...) usado pelo hardware
//   receptor para sincronizar o relógio com o sinal recebido.
//   Em binário: 10101010 10101010 10101010 10101010 10101010 10101010 10101010
//
// SFD (Start Frame Delimiter, 0xAB):
//   Último byte do preâmbulo quebrado (10101011) para sinalizar
//   que os próximos bytes são o cabeçalho MAC.
//
// FCS (Frame Check Sequence):
//   CRC-32 sobre todos os bytes do quadro (exceto o próprio FCS).
//   Polinômio IEEE 802.3: x³² + x²⁶ + x²³ + x²² + x¹⁶ + x¹² + x¹¹ +
//   x¹⁰ + x⁸ + x⁷ + x⁵ + x⁴ + x² + x + 1
//   Representação reversa: 0xEDB88320

// =============================================================================
// ipChecksum — checksum de cabeçalho IP (RFC 791)
// =============================================================================
//
// Usa o mesmo algoritmo de complemento de 1 do TCP (tcpChecksum),
// mas aplicado apenas sobre o cabeçalho IP (20 bytes).
//
// O campo checksum (bytes 10-11) DEVE estar zerado antes da chamada.
// buildIPPacket já garante isso (faz o checksum por último).
//
// FUNCIONAMENTO DETALHADO:
//   O cabeçalho IP é tratado como um array de palavras de 16 bits.
//   A soma dessas palavras (com overflow dobrado de volta) é invertida.
//   Para verificar: soma todas as palavras incluindo o checksum → deve dar 0.

// =============================================================================
// FUNÇÕES AUXILIARES DE PARSING
// =============================================================================
// Estas funções extraem campos específicos dos cabeçalhos recebidos.
// São usadas para validar as respostas do servidor.

// hasFlag — verifica se uma flag TCP está ligada no segmento.
//
// Uso: hasFlag(segmento, tcpFlagSYN) → true se SYN=1
//
// O byte 13 do cabeçalho TCP contém as 8 flags.
// Fazemos AND bit-a-bit: se o bit da flag estiver ligado, retorna true.

// getTCPSeq — extrai o Sequence Number do cabeçalho TCP.
//
// Sequence Number está nos bytes 4-7 do cabeçalho (uint32 big-endian).

// getTCPAck — extrai o Acknowledgment Number do cabeçalho TCP.
//
// Acknowledgment Number está nos bytes 8-11 do cabeçalho (uint32 big-endian).

// =============================================================================
// FUNÇÕES DE DESENCAPSULAMENTO
// =============================================================================
// A resposta do servidor chega como: [quadro 802.3][pacote IP][segmento TCP]
// Cada função remove uma camada.

// extractFramePayload — remove o envelope IEEE 802.3 e retorna o pacote IP.
//
// Um quadro válido tem no mínimo 26 bytes:
//
//	7 (preâmbulo) + 1 (SFD) + 6 (MAC dst) + 6 (MAC src) + 2 (tamanho) + 4 (CRC)
//
// Retorna os bytes entre o cabeçalho 802.3 e o CRC.

// extractIPPayload — remove o cabeçalho IP e retorna o segmento TCP.
//
// O tamanho do cabeçalho IP está no nibble inferior do byte 0:
//
//	IHL (Internet Header Length): pkt[0] & 0x0F → número de palavras de 32 bits
//	Tamanho em bytes = IHL × 4
//
// Exemplo: se pkt[0] = 0x45 → IHL = 5 → cabeçalho = 20 bytes
//
//	payload começa no byte 20

// extractTCPPayload — remove o cabeçalho TCP e retorna só os dados.
//
// O tamanho do cabeçalho TCP está no nibble superior do byte 12:
//
//	Data Offset: seg[12] >> 4 → número de palavras de 32 bits
//	Tamanho em bytes = Data Offset × 4
//
// Exemplo: se seg[12] = 0x50 → Data Offset = 5 → cabeçalho = 20 bytes
//
//	payload começa no byte 20

// =============================================================================
// CRC-32 (IEEE 802.3)
// =============================================================================
//
// Implementação manual para evitar dependência de hash/crc32.
// A função nativa seria: hash/crc32.ChecksumIEEE(data)
//
// POLINÔMIO: 0xEDB88320 (representação reversa do polinômio IEEE 802.3)
//   Polinômio original: x³² + x²⁶ + x²³ + x²² + x¹⁶ + x¹² + x¹¹ +
//                        x¹⁰ + x⁸ + x⁷ + x⁵ + x⁴ + x² + x + 1
//   Representação reversa (usada no algoritmo): 0xEDB88320
//
// ALGORITMO (método da tabela, O(n)):
//   1. Inicializa CRC com 0xFFFFFFFF
//   2. Para cada byte de dados:
//      a. XOR do byte com o byte baixo do CRC atual
//      b. Usa o resultado como índice na tabela
//      c. CRC = tabela[índice] ^ (CRC >> 8)
//   3. Inverte o CRC final: CRC ^ 0xFFFFFFFF

// makeCRC32Table — pré-calcula a tabela de 256 entradas para CRC-32.
//
// Cada entrada table[i] contém o resultado de aplicar o polinômio
// 0xEDB88320 ao valor i por 8 iterações.
//
// O cálculo simula divisão polinomial: para cada bit de i (do menos
// significativo), se o bit for 1, faz XOR com o polinômio após shift.
