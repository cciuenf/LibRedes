/* Necessário para getaddrinfo/freeaddrinfo/struct addrinfo com -std=c11 */
#define _POSIX_C_SOURCE 200112L

/*
 * libphysical.c — implementação da API do meio físico simulado
 *
 * Protocolo UDP (big-endian):
 *   HELLO     [0x01][group_id: 16 bytes, null-padded]
 *   HELLO_ACK [0x02][mac: 6 bytes][virtual_ip: 4 bytes, network byte order]
 *   DATA      [0x03][dst_ip: 4 bytes, network byte order][payload: N bytes]
 *   DELIVER   [0x04][src_ip: 4 bytes, network byte order][payload: N bytes]
 *   MEDIUM    [0x05]
 *   MEDIUM_OK [0x06][free: 1 byte]
 *   PING      [0x07]
 *   PONG      [0x08][rtt_hint: 2 bytes, microssegundos]
 *   BYE       [0x09]
 *   ERROR     [0xFF][msg: N bytes UTF-8]
 *
 * Convenção de byte order nesta lib:
 *   - Tudo que entra e sai pela API pública está em HOST byte order.
 *   - A conversão para/de network byte order acontece aqui dentro.
 *   - PHY_SERVER_IP e phy_virtual_ip() retornam host byte order.
 */

#include "libphysical.h"

#include <arpa/inet.h>
#include <errno.h>
#include <netdb.h>
#include <netinet/in.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <time.h>
#include <unistd.h>

/* ---- tipos de mensagem ---- */
#define MSG_HELLO     0x01
#define MSG_HELLO_ACK 0x02
#define MSG_DATA      0x03
#define MSG_DELIVER   0x04
#define MSG_MEDIUM    0x05
#define MSG_MEDIUM_OK 0x06
#define MSG_PING      0x07
#define MSG_PONG      0x08
#define MSG_BYE       0x09
#define MSG_ERROR     0xFF

#define MAX_GROUP_ID  16
#define RECV_BUF_SIZE 2048   /* MTU simulado + cabeçalhos; evita 64KB no stack */

struct PhysicalHandle {
    int                  sock;
    struct sockaddr_in   server_addr;
    uint8_t              mac[6];
    uint32_t             virtual_ip;   /* HOST byte order */
    char                 group_id[MAX_GROUP_ID + 1];
};

/* ---- helpers internos ---- */

static int set_recv_timeout(int sock, int ms) {
    struct timeval tv;
    tv.tv_sec  = ms / 1000;
    tv.tv_usec = (ms % 1000) * 1000;
    return setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
}

static int clear_recv_timeout(int sock) {
    struct timeval tv = {0, 0};
    return setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
}

static ssize_t udp_send(struct PhysicalHandle *h, const uint8_t *buf, size_t len) {
    return sendto(h->sock, buf, len, 0,
                  (struct sockaddr *)&h->server_addr, sizeof(h->server_addr));
}

static ssize_t udp_recv(struct PhysicalHandle *h, uint8_t *buf, size_t len) {
    return recv(h->sock, buf, len, 0);
}

/* Retorna tempo monotônico em milissegundos. */
static long now_ms(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (long)(ts.tv_sec * 1000L + ts.tv_nsec / 1000000L);
}

/* ---- handshake HELLO ---- */

static int do_hello(struct PhysicalHandle *h) {
    uint8_t pkt[1 + MAX_GROUP_ID];
    memset(pkt, 0, sizeof(pkt));
    pkt[0] = MSG_HELLO;
    strncpy((char *)pkt + 1, h->group_id, MAX_GROUP_ID);

    uint8_t buf[64];
    int attempts = 3;

    while (attempts-- > 0) {
        if (udp_send(h, pkt, sizeof(pkt)) < 0) {
            perror("phy_connect: sendto HELLO");
            return -1;
        }

        set_recv_timeout(h->sock, 2000);
        ssize_t n = udp_recv(h, buf, sizeof(buf));
        clear_recv_timeout(h->sock);

        if (n < 0) {
            if (errno == EAGAIN || errno == EWOULDBLOCK) {
                fprintf(stderr, "phy_connect: timeout aguardando HELLO_ACK (tentativa %d)\n",
                        3 - attempts);
                continue;
            }
            perror("phy_connect: recv HELLO_ACK");
            return -1;
        }

        if (n < 11 || buf[0] != MSG_HELLO_ACK) {
            fprintf(stderr, "phy_connect: resposta inesperada (type=0x%02X)\n", buf[0]);
            return -1;
        }

        memcpy(h->mac, buf + 1, 6);

        uint32_t ip_net;
        memcpy(&ip_net, buf + 7, 4);
        h->virtual_ip = ntohl(ip_net);   /* armazena em host byte order */
        return 0;
    }

    fprintf(stderr, "phy_connect: nó central não respondeu após 3 tentativas\n");
    return -1;
}

/* ---- API pública ---- */

PhysicalHandle *phy_connect(const char *group_id, const char *host, uint16_t port) {
    if (!group_id || strlen(group_id) == 0) {
        fprintf(stderr, "phy_connect: group_id não pode ser vazio\n");
        return NULL;
    }
    if (strlen(group_id) > MAX_GROUP_ID) {
        fprintf(stderr, "phy_connect: group_id muito longo (max %d)\n", MAX_GROUP_ID);
        return NULL;
    }

    struct PhysicalHandle *h = calloc(1, sizeof(*h));
    if (!h) return NULL;

    strncpy(h->group_id, group_id, MAX_GROUP_ID);

    /* Resolve hostname */
    struct addrinfo hints = {0}, *res = NULL;
    hints.ai_family   = AF_INET;
    hints.ai_socktype = SOCK_DGRAM;
    int rc = getaddrinfo(host, NULL, &hints, &res);
    if (rc != 0) {
        fprintf(stderr, "phy_connect: getaddrinfo(%s): %s\n", host, gai_strerror(rc));
        free(h);
        return NULL;
    }
    h->server_addr = *(struct sockaddr_in *)res->ai_addr;
    h->server_addr.sin_port = htons(port);
    freeaddrinfo(res);

    /* Cria socket UDP */
    h->sock = socket(AF_INET, SOCK_DGRAM, 0);
    if (h->sock < 0) {
        perror("phy_connect: socket");
        free(h);
        return NULL;
    }

    /* Bind em porta aleatória */
    struct sockaddr_in local = {0};
    local.sin_family = AF_INET;
    local.sin_addr.s_addr = INADDR_ANY;
    local.sin_port = 0;
    if (bind(h->sock, (struct sockaddr *)&local, sizeof(local)) < 0) {
        perror("phy_connect: bind");
        close(h->sock);
        free(h);
        return NULL;
    }

    if (do_hello(h) < 0) {
        close(h->sock);
        free(h);
        return NULL;
    }

    return h;
}

void phy_mac_addr(PhysicalHandle *h, uint8_t mac_out[6]) {
    if (h) memcpy(mac_out, h->mac, 6);
}

/* Retorna host byte order — use htonl() se precisar colocar num pacote. */
uint32_t phy_virtual_ip(PhysicalHandle *h) {
    return h ? h->virtual_ip : 0;
}

int phy_medium_free(PhysicalHandle *h) {
    if (!h) return -1;

    uint8_t pkt = MSG_MEDIUM;
    if (udp_send(h, &pkt, 1) < 0) return -1;

    uint8_t buf[4];
    set_recv_timeout(h->sock, 500);
    ssize_t n = udp_recv(h, buf, sizeof(buf));
    clear_recv_timeout(h->sock);

    if (n < 2 || buf[0] != MSG_MEDIUM_OK) return -1;
    return (buf[1] == 0x01) ? 1 : 0;
}

/*
 * phy_send — dst_ip em HOST byte order (igual ao retorno de phy_virtual_ip e PHY_SERVER_IP).
 * A conversão para network byte order é feita internamente.
 */
int phy_send(PhysicalHandle *h, uint32_t dst_ip, const uint8_t *data, size_t len) {
    if (!h || !data) return -1;

    uint8_t *pkt = malloc(1 + 4 + len);
    if (!pkt) return -1;

    uint32_t dst_net = htonl(dst_ip);   /* host → network byte order */

    pkt[0] = MSG_DATA;
    memcpy(pkt + 1, &dst_net, 4);
    memcpy(pkt + 5, data, len);

    ssize_t sent = udp_send(h, pkt, 1 + 4 + len);
    free(pkt);
    return (sent < 0) ? -1 : 0;
}

/*
 * phy_recv — src_ip é preenchido em HOST byte order.
 *
 * Correção de timeout: usa clock monotônico para recalcular o tempo restante
 * a cada pacote de controle descartado, evitando retorno prematuro.
 */
ssize_t phy_recv(PhysicalHandle *h, uint32_t *src_ip, uint8_t *buf, size_t buf_len,
                 int timeout_ms) {
    if (!h || !buf) return -1;

    static uint8_t tmp[RECV_BUF_SIZE];   /* static: evita 2KB no stack a cada chamada */

    long deadline = 0;
    if (timeout_ms > 0) deadline = now_ms() + timeout_ms;

    for (;;) {
        /* Recalcula tempo restante a cada iteração (pacotes descartados consomem tempo). */
        if (timeout_ms > 0) {
            long remaining = deadline - now_ms();
            if (remaining <= 0) return 0;   /* timeout */
            set_recv_timeout(h->sock, (int)remaining);
        }

        ssize_t n = udp_recv(h, tmp, sizeof(tmp));

        if (n < 0) {
            clear_recv_timeout(h->sock);
            if (errno == EAGAIN || errno == EWOULDBLOCK) return 0; /* timeout */
            return -1;
        }

        if (n < 1) continue;
        if (tmp[0] != MSG_DELIVER) continue; /* descarta controle (MEDIUM_OK, PONG, etc.) */
        if (n < 5) continue;

        uint32_t ip_net;
        memcpy(&ip_net, tmp + 1, 4);
        if (src_ip) *src_ip = ntohl(ip_net);   /* network → host byte order */

        size_t payload_len = (size_t)(n - 5);
        if (payload_len > buf_len) payload_len = buf_len;
        memcpy(buf, tmp + 5, payload_len);

        if (timeout_ms > 0) clear_recv_timeout(h->sock);
        return (ssize_t)payload_len;
    }
}

/*
 * phy_ping — mede o RTT até o nó central em microssegundos.
 *
 * Envia MSG_PING e aguarda MSG_PONG. Retorna o RTT medido localmente em µs,
 * ou -1 em caso de erro/timeout (timeout fixo: 2 s).
 *
 * Útil para diagnosticar latência do meio simulado antes de transmitir.
 */
long phy_ping(PhysicalHandle *h) {
    if (!h) return -1;

    uint8_t pkt = MSG_PING;

    struct timespec t0, t1;
    clock_gettime(CLOCK_MONOTONIC, &t0);

    if (udp_send(h, &pkt, 1) < 0) return -1;

    uint8_t buf[8];
    set_recv_timeout(h->sock, 2000);
    ssize_t n = udp_recv(h, buf, sizeof(buf));
    clear_recv_timeout(h->sock);

    clock_gettime(CLOCK_MONOTONIC, &t1);

    if (n < 1 || buf[0] != MSG_PONG) return -1;

    long rtt_us = (long)((t1.tv_sec  - t0.tv_sec)  * 1000000L +
                         (t1.tv_nsec - t0.tv_nsec) / 1000L);
    return rtt_us;
}

/*
 * phy_disconnect — envia BYE ao nó central, fecha o socket e libera o handle.
 *
 * O BYE permite que o nó central libere o group_id imediatamente,
 * evitando recusa de reconexão em caso de crash + restart rápido.
 */
void phy_disconnect(PhysicalHandle *h) {
    if (!h) return;
    uint8_t bye = MSG_BYE;
    udp_send(h, &bye, 1);   /* best-effort: ignora erro de envio */
    close(h->sock);
    free(h);
}
