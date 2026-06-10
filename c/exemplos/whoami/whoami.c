/*
 * whoami.c
 *
 * Primeiro programa usando a LibPhysical.
 *
 * Objetivos:
 *   1. Conectar ao meio físico simulado.
 *   2. Descobrir o MAC virtual atribuído ao grupo.
 *   3. Descobrir o IP virtual atribuído ao grupo.
 *   4. Encerrar a conexão corretamente.
 */

#include <stdio.h>
#include <stdint.h>
#include <arpa/inet.h>

#include "libphysical.h"

int main(void) {
    PhysicalHandle *h = phy_connect("grupo_a","127.0.0.1",9000);
    if (!h) {
        printf("Falha ao conectar ao meio físico.\n");
        return 1;
    }
    uint8_t mac[6];
    phy_mac_addr(h, mac);
    uint32_t ip = phy_virtual_ip(h);
    struct in_addr addr;
    addr.s_addr = htonl(ip);
    printf("IP : %s\n", inet_ntoa(addr));
    phy_disconnect(h);
    return 0;
}

