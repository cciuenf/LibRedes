// whoami — descobre a identidade do grupo na rede simulada.
//
// O physical_medium atribui um MAC e um IP virtual a cada grupo
// que se registra. Este programa faz o handshake HELLO e imprime
// os dados recebidos.
//
// Fluxo:
//  1. PhyConnect → envia HELLO, espera HELLO_ACK
//  2. O HELLO_ACK contém o MAC (6 bytes) e o IP virtual (4 bytes)
//  3. Imprime ambos no terminal
//
// Para rodar:
//
//	physical_medium -flags-dir ../flag_server/flags
package main

import (
	"fmt"

	"libphysical"
)

// =========================================================================
// MAC: (Media Access Control) endereço físico de 6 bytes (ex: 02:00:00:00:00:02)
// IP:  (Internet Protocol)    endereço lógico de 4 bytes (ex: 10.0.0.2)
// =========================================================================
//
// PhyConnect registra o grupo e devolve um *PhysicalHandler.
// Use defer com PhyDisconnect para liberar ao final.
//
// PhyMacAddr() retorna o MAC virtual como [6]byte.
// PhyVirtualIP() retorna o IP virtual como uint32 (host byte order).
//
// Para imprimir:
//   PrintMac  recebe *[6]byte e imprime no formato xx:xx:xx:xx:xx:xx
//   PrintIP   recebe uint32 e imprime no formato dotted-quad (x.x.x.x)

func main() {
	h := libphysical.MustPhyConnect("grupo_b", "136.248.93.84", 8000)
	defer h.PhyDisconnect()

	fmt.Println("=== Conectado ao nó central ===")

	fmt.Print("  MAC virtual: ")
	mac := h.PhyMacAddr()
	libphysical.PrintMac(&mac)

	fmt.Print("  IP virtual:  ")
	libphysical.PrintIP(h.PhyVirtualIP())
}
