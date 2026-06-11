package main

import (
	"fmt"
	"libphysical"
)

// printFrame exibe o quadro formatado.
func printFrame(frame []byte) {
	fmt.Println("=== Quadro IEEE 802.3 construído ===")
	offsets := []struct {
		name   string
		start  int
		length int
	}{
		{"Preâmbulo", 0, 7},
		{"SFD", 7, 1},
		{"MAC destino", 8, 6},
		{"MAC origem", 14, 6},
		{"Tamanho", 20, 2},
		{"Payload", 22, len(frame) - 26},
		{"FCS (CRC-32)", len(frame) - 4, 4},
	}

	for _, f := range offsets {
		fmt.Printf("  %-14s [%2d:%2d] ", f.name+":", f.start, f.start+f.length-1)
		if f.length <= 8 {
			fmt.Printf("% X\n", frame[f.start:f.start+f.length])
		} else {
			// Payload grande: mostra início + "..." + fim
			mid := frame[f.start : f.start+f.length]
			fmt.Printf("% X ... % X  (%d bytes)\n",
				mid[:4], mid[len(mid)-4:], f.length)
		}
	}
}

func printConnection(phy *libphysical.PhysicalHandler) {
	fmt.Println("=== Conectado ao nó central ===")
	fmt.Printf("  MAC virtual: ")
	mac := phy.PhyMacAddr()
	libphysical.PrintMac(&mac)
	fmt.Printf("  IP virtual:  ")
	libphysical.PrintIP(phy.PhyVirtualIP())
	fmt.Println()
}
