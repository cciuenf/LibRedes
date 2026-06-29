// =============================================================================
// flag_two — captura a Flag 2 do flag_server implementando retransmissão TCP.
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
//	Flag 2:  IEEE 802.3 → IP → TCP COM RETRANSMISSÃO.
//	         Igual à Flag 1B, mas o perfil noisy.json está ativo (5% drop).
//	         Cada envio agora tem um loop de retransmissão: timeout → reenvia.
//	         Sem retransmissão, ~5% das execuções falham.

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"log"
	"time"

	"libphysical"
)

// =============================================================================
// CONSTANTES DE RETRANSMISSÃO (NOVO NA FLAG 2)
// =============================================================================
//
// Com o perfil noisy (5% drop), ~1 a cada 20 pacotes se perde.
// Cada fase do protocolo agora tem um loop que reenvia em caso de timeout.
//
// timeoutRetransmit: quanto esperar por resposta antes de reenviar.
// maxRetransmits:    quantas tentativas no total antes de desistir.
const (
	timeoutRetransmit = 500 * time.Millisecond
	maxRetransmits    = 10
)

// =============================================================================
// main — fluxo principal da captura da Flag 2 (COM RETRANSMISSÃO)
// =============================================================================
//
// A Flag 2 parte do código da Flag 1B. A pilha de protocolos (802.3, IP,
// TCP) e as funções auxiliares são as mesmas. A única diferença é que o
// physical_medium agora está rodando com o perfil noisy.json, que introduz
// perda aleatória de pacotes (~5% de drop). Sua tarefa é proteger o código
// contra essas perdas adicionando retransmissão.
//
// O QUE FAZER (em ordem):
//
//   1. Adicionar duas constantes: um timeout de retransmissão (ex: 500ms)
//      e um número máximo de tentativas (ex: 10).
//
//   2. Trocar o path HTTP de "/flag1b" para "/flag2".
//
//   3. Envolver o envio do SYN e a recepção do SYN-ACK em um loop.
//      Se o PhyRecv der timeout, o loop reenvia o SYN e tenta de novo.
//      O loop termina quando receber um SYN-ACK válido ou esgotar as
//      tentativas.
//
//   4. Fazer o mesmo para o envio do ACK+Dados e a recepção da resposta:
//      envolver em um loop com timeout e reenvio.
//
//   5. A cada reenvio, reconstruir o quadro IEEE 802.3 (não reutilizar
//      o []byte anterior). Pense: por que o CRC-32 precisa ser recalculado?
//
//
// DICA: teste com run100.sh. Sem retransmissão você vai ver ~10-15% de
// falhas. Com retransmissão, deve chegar a 100%.

