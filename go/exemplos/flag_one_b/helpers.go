package main

import (
	"fmt"
	"libphysical"
)

// printFrame exibe o quadro IEEE 802.3 + o pacote IP + o segmento TCP dentro dele.
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

	tcpPayload := pkt[ihl:]
	if len(tcpPayload) >= 20 && protocol == 6 {
		printTCPHeader(tcpPayload)
	} else if len(tcpPayload) > 0 {
		fmt.Printf("    IP payload: %q\n", string(tcpPayload))
	}
}

// printTCPHeader exibe o conteúdo do cabeçalho TCP.
func printTCPHeader(seg []byte) {
	if len(seg) < 20 {
		fmt.Printf("    TCP: segmento muito curto (%d bytes)\n", len(seg))
		return
	}

	// Extrai campos do cabeçalho
	srcPort := uint16(seg[0])<<8 | uint16(seg[1])
	dstPort := uint16(seg[2])<<8 | uint16(seg[3])
	seq := uint32(seg[4])<<24 | uint32(seg[5])<<16 | uint32(seg[6])<<8 | uint32(seg[7])
	ack := uint32(seg[8])<<24 | uint32(seg[9])<<16 | uint32(seg[10])<<8 | uint32(seg[11])
	dataOffset := (seg[12] >> 4) * 4
	flags := seg[13]
	window := uint16(seg[14])<<8 | uint16(seg[15])

	// Formata flags como string
	flagStrs := ""
	if flags&0x01 != 0 {
		flagStrs += "FIN "
	}
	if flags&0x02 != 0 {
		flagStrs += "SYN "
	}
	if flags&0x04 != 0 {
		flagStrs += "RST "
	}
	if flags&0x08 != 0 {
		flagStrs += "PSH "
	}
	if flags&0x10 != 0 {
		flagStrs += "ACK "
	}

	fmt.Printf("    TCP: %d → %d  seq=%d  ack=%d  flags=[%s] window=%d dataOffset=%d\n",
		srcPort, dstPort, seq, ack, flagStrs, window, dataOffset)

	payload := seg[dataOffset:]
	if len(payload) > 0 {
		fmt.Printf("    TCP payload: %q\n", string(payload))
	}
}

// printTCPSegment exibe um segmento TCP com título.
func printTCPSegment(label string, seg []byte) {
	fmt.Printf("--- %s ---\n", label)
	printTCPHeader(seg)
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
