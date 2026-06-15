package main

import (
	"fmt"
	"libphysical"
)

// printFrame exibe o quadro IEEE 802.3 + o pacote IP dentro dele.
func printFrame(frame []byte) {
	fmt.Println("=== Quadro IEEE 802.3 ===")

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
	}
	for _, f := range offsets {
		fmt.Printf("  %-14s [%2d:%2d] % X\n", f.name+":", f.start, f.start+f.length-1,
			frame[f.start:f.start+f.length])
	}

	payload := frame[22 : len(frame)-4]
	fmt.Printf("  %-14s [%2d:%2d] %d bytes (pacote IP)\n", "Payload:", 22, 22+len(payload)-1, len(payload))

	if len(payload) >= 20 {
		printIPPacket(payload)
	}

	fcsStart := len(frame) - 4
	fmt.Printf("  %-14s [%2d:%2d] % X\n", "FCS (CRC-32):", fcsStart, fcsStart+3,
		frame[fcsStart:fcsStart+4])
}

func printIPPacket(pkt []byte) {
	ihl := (pkt[0] & 0x0F) * 4
	totalLen := int(pkt[2])<<8 | int(pkt[3])
	ttl := pkt[8]
	protocol := pkt[9]
	src := fmt.Sprintf("%d.%d.%d.%d", pkt[12], pkt[13], pkt[14], pkt[15])
	dst := fmt.Sprintf("%d.%d.%d.%d", pkt[16], pkt[17], pkt[18], pkt[19])

	fmt.Printf("    IP: ver=%d IHL=%d len=%d proto=%d TTL=%d\n",
		pkt[0]>>4, ihl, totalLen, protocol, ttl)
	fmt.Printf("    IP: %s → %s\n", src, dst)

	ipPayload := pkt[ihl:]
	if len(ipPayload) > 0 {
		fmt.Printf("    IP payload: %q\n", string(ipPayload))
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
