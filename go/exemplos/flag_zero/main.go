// flag_zero — captura a Flag 0 do physical_medium usando libphysical.
//
// Demonstra o protocolo completo:
//  1. HELLO          → registra o grupo no nó central
//  2. HELLO_ACK      → recebe MAC e IP virtuais
//  3. DATA (quadro)  → envia um quadro IEEE 802.3 válido
//  4. DELIVER        → recebe a flag
//
// A flag só é entregue se o quadro passar na validação:
//   - Preâmbulo: 7 bytes 0xAA
//   - SFD:       1 byte  0xAB
//   - MAC origem: igual ao MAC virtual do grupo
//   - CRC-32:    correto sobre todo o quadro
package main

func main() {}

// =============================================================================
// Construção do quadro IEEE 802.3
// =============================================================================
//
// Um quadro Ethernet / IEEE 802.3 tem a seguinte estrutura:
//
//   Offset | Tamanho | Campo       | Valor
//   -------|---------|-------------|-----------------------------------
//    0     |  7      | Preâmbulo   | 0xAA × 7 (sincronização do receptor)
//    7     |  1      | SFD         | 0xAB (Start Frame Delimiter)
//    8     |  6      | MAC destino | FF:FF:FF:FF:FF:FF (broadcast)
//   14     |  6      | MAC origem  | MAC virtual do grupo (do HELLO_ACK)
//   20     |  2      | Tamanho     | tamanho do payload, big-endian
//   22     |  N      | Payload     | dados (ex: "FLAG0")
//   22+N   |  4      | FCS (CRC)   | CRC-32 sobre offsets 0..21+N
//
// SFD: (Start Frame Delimiter)   byte 0xAB que marca o fim do preâmbulo
// MAC: (Media Access Control)    endereço físico de 6 bytes da interface
// FCS: (Frame Check Sequence)    4 bytes de verificação no final do quadro
// CRC: (Cyclic Redundancy Check) algoritmo que gera o FCS (polinômio IEEE 802.3)
