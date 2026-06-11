#ifndef LIBPHYSICAL_H
#define LIBPHYSICAL_H

/*
 * libphysical — API do meio físico simulado
 * Projeto TCP/IP do Zero — Capture the Flag
 *
 * Esta biblioteca abstrai o protocolo UDP entre o seu programa
 * e o nó central (physical_medium). Você não precisa se preocupar
 * com sockets — apenas com o que enviar e o que receber.
 *
 * ── Byte order ────────────────────────────────────────────────────────────────
 * Todas as funções desta API usam HOST byte order (o padrão do seu programa).
 * A conversão para network byte order acontece dentro da lib — você não precisa
 * chamar htonl/ntohl em nenhum dos argumentos ou retornos aqui.
 *
 * Exemplos corretos:
 *   phy_send(h, PHY_SERVER_IP, data, len);           // OK — constante já em host order
 *   phy_send(h, phy_virtual_ip(h), data, len);       // OK — retorno já em host order
 *
 * Exemplos ERRADOS (não faça isso):
 *   phy_send(h, htonl(PHY_SERVER_IP), data, len);   // ERRADO — converte duas vezes
 *
 * ── Uso básico ────────────────────────────────────────────────────────────────
 *
 *   PhysicalHandle *h = phy_connect("grupo_a", "10.0.1.100", 9000);
 *   if (!h) { perror("connect"); exit(1); }
 *
 *   uint8_t mac[6];
 *   phy_mac_addr(h, mac);
 *   printf("Meu MAC: %02x:%02x:%02x:%02x:%02x:%02x\n",
 *          mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]);
 *
 *   // Diagnóstico: mede RTT com o nó central
 *   long rtt = phy_ping(h);
 *   if (rtt >= 0) printf("RTT: %ld µs\n", rtt);
 *
 *   // CSMA/CA: verifica se o meio está livre antes de transmitir
 *   while (phy_medium_free(h) != 1)
 *       usleep(10000);
 *
 *   // Envia bytes brutos para o servidor (10.0.0.1)
 *   uint8_t payload[] = "oi servidor";
 *   phy_send(h, PHY_SERVER_IP, payload, sizeof(payload) - 1);
 *
 *   // Recebe resposta (timeout de 3 s)
 *   uint32_t src_ip;
 *   uint8_t buf[4096];
 *   ssize_t n = phy_recv(h, &src_ip, buf, sizeof(buf), 3000);
 *   if (n > 0) printf("Recebido: %.*s\n", (int)n, buf);
 *
 *   phy_disconnect(h);
 */

#include <stddef.h>
#include <stdint.h>
#include <sys/types.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Handle opaco — não acesse os campos internamente. */
typedef struct PhysicalHandle PhysicalHandle;

/* IP virtual do servidor de flags na rede simulada: 10.0.0.1 (host byte order). */
#define PHY_SERVER_IP  0x0A000001u

/*
 * phy_connect — conecta ao nó central e registra o grupo.
 *
 * group_id : identificador único do grupo (ex: "grupo_a"), máx 16 chars
 * host     : IP ou hostname do nó central
 * port     : porta UDP do nó central (normalmente 9000)
 *
 * Retorna um handle em sucesso, NULL em erro.
 * Após conectar, phy_mac_addr(), phy_virtual_ip() e phy_ping() estão disponíveis.
 */
PhysicalHandle *phy_connect(const char *group_id,
                             const char *host,
                             uint16_t    port);

/*
 * phy_mac_addr — copia o MAC virtual de 6 bytes para mac_out.
 * Use este MAC como endereço de origem em quadros IEEE 802.
 * mac_out deve ter ao menos 6 bytes.
 */
void phy_mac_addr(PhysicalHandle *h, uint8_t mac_out[6]);

/*
 * phy_virtual_ip — retorna o IP virtual do grupo em HOST byte order.
 * Exemplo de retorno: 0x0A000002 representa 10.0.0.2
 *
 * Se precisar colocar este valor dentro de um pacote na rede, use htonl():
 *   uint32_t ip_para_pacote = htonl(phy_virtual_ip(h));
 */
uint32_t phy_virtual_ip(PhysicalHandle *h);

/*
 * phy_ping — mede o RTT até o nó central.
 *
 * Envia um PING e aguarda o PONG. Útil para checar a latência do meio
 * antes de calibrar timeouts nas camadas superiores.
 *
 * Retorna o RTT em microssegundos (µs), ou -1 em erro/timeout.
 */
long phy_ping(PhysicalHandle *h);

/*
 * phy_medium_free — consulta se o meio está livre (CSMA/CA).
 *
 * Retorna:
 *    1  : meio livre, pode transmitir
 *    0  : meio ocupado, aguarde e tente novamente
 *   -1  : erro de comunicação com o nó central
 *
 * Chame esta função antes de cada phy_send() para implementar CSMA/CA.
 */
int phy_medium_free(PhysicalHandle *h);

/*
 * phy_send — envia bytes brutos para um IP virtual de destino.
 *
 * dst_ip : IP do destino em HOST byte order (use PHY_SERVER_IP ou phy_virtual_ip())
 * data   : ponteiro para os dados — a lib não interpreta o conteúdo
 * len    : número de bytes
 *
 * Retorna 0 em sucesso, -1 em erro.
 */
int phy_send(PhysicalHandle  *h,
             uint32_t         dst_ip,
             const uint8_t   *data,
             size_t           len);

/*
 * phy_recv — recebe bytes do nó central.
 *
 * Bloqueia até chegar um pacote de dados ou o timeout esgotar.
 * Pacotes de controle (MEDIUM_OK, PONG, etc.) são descartados automaticamente.
 * O timeout é respeitado mesmo que muitos pacotes de controle cheguem antes.
 *
 * src_ip     : preenchido com o IP virtual de origem em HOST byte order
 * buf        : buffer do chamador para os dados recebidos
 * buf_len    : tamanho do buffer
 * timeout_ms : timeout em milissegundos; 0 = bloqueio indefinido
 *
 * Retorna:
 *   > 0  : número de bytes recebidos
 *   = 0  : timeout esgotado sem dados
 *   = -1 : erro (ver errno)
 */
ssize_t phy_recv(PhysicalHandle *h,
                 uint32_t       *src_ip,
                 uint8_t        *buf,
                 size_t          buf_len,
                 int             timeout_ms);

/*
 * phy_disconnect — encerra a conexão com o nó central e libera o handle.
 *
 * Envia um BYE ao nó central para liberar o group_id imediatamente.
 * Isso evita recusa de reconexão se o programa reiniciar rapidamente.
 * Após esta chamada, o handle não deve ser usado.
 */
void phy_disconnect(PhysicalHandle *h);

#ifdef __cplusplus
}
#endif

#endif /* LIBPHYSICAL_H */
