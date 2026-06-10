# RFC-LP-001

## LibPhysical — Especificação da Biblioteca de Meio Físico Simulado

**Versão:** 1.0
**Projeto:** TCP/IP do Zero — Capture the Flag
**Status:** Experimental / Didático

---

# 1. Introdução

A **LibPhysical** fornece uma API em C para acesso a um meio físico simulado, implementado por um nó central chamado `physical_medium`.

A biblioteca abstrai a comunicação UDP real entre o programa do aluno e o nó central. O aluno interage apenas com uma API simples para:

* conectar-se ao meio físico simulado;
* obter MAC virtual;
* obter IP virtual;
* consultar se o meio está livre;
* enviar bytes brutos;
* receber bytes brutos;
* medir latência com o nó central;
* encerrar a conexão.

A LibPhysical **não implementa** Ethernet, IEEE 802, IP ou TCP. Esses protocolos serão implementados pelos alunos sobre os bytes transportados pela biblioteca.

---

# 2. Convenção de Byte Order

Toda a API pública da LibPhysical usa **host byte order**.

Isso significa que o usuário da biblioteca **não deve chamar `htonl()` ou `ntohl()`** ao usar:

```c
PHY_SERVER_IP
phy_virtual_ip()
phy_send()
phy_recv()
```

A conversão para **network byte order** acontece internamente na biblioteca.

Exemplo correto:

```c
phy_send(h, PHY_SERVER_IP, data, len);
```

Exemplo incorreto:

```c
phy_send(h, htonl(PHY_SERVER_IP), data, len);
```

---

# 3. Entidades

## 3.1 Cliente

Programa escrito pelo aluno usando `libphysical.h`.

## 3.2 Nó Central

Servidor `physical_medium`, responsável por:

* registrar grupos;
* atribuir MAC virtual;
* atribuir IP virtual;
* encaminhar mensagens;
* simular ocupação do meio;
* responder a diagnósticos;
* receber encerramento de conexão.

---

# 4. Tipos de Mensagem

| Nome      |  Valor | Direção      |
| --------- | -----: | ------------ |
| HELLO     | `0x01` | Cliente → Nó |
| HELLO_ACK | `0x02` | Nó → Cliente |
| DATA      | `0x03` | Cliente → Nó |
| DELIVER   | `0x04` | Nó → Cliente |
| MEDIUM    | `0x05` | Cliente → Nó |
| MEDIUM_OK | `0x06` | Nó → Cliente |
| PING      | `0x07` | Cliente → Nó |
| PONG      | `0x08` | Nó → Cliente |
| BYE       | `0x09` | Cliente → Nó |
| ERROR     | `0xFF` | Nó → Cliente |

---

# 5. Registro Inicial

Antes de transmitir dados, o cliente deve se registrar no nó central.

Esse processo é realizado automaticamente por:

```c
PhysicalHandle *phy_connect(const char *group_id,
                            const char *host,
                            uint16_t port);
```

---

## 5.1 Mensagem HELLO

Formato:

```text
+--------+------------------+
| Type   | Group ID         |
+--------+------------------+
| 1 byte | 16 bytes         |
+--------+------------------+
```

Campos:

| Campo    |  Tamanho | Descrição                                    |
| -------- | -------: | -------------------------------------------- |
| Type     |   1 byte | Valor `0x01`                                 |
| Group ID | 16 bytes | Identificador do grupo, preenchido com zeros |

O `group_id` deve possuir no máximo 16 caracteres.

---

## 5.2 Mensagem HELLO_ACK

Formato:

```text
+--------+---------+------------+
| Type   | MAC     | Virtual IP |
+--------+---------+------------+
| 1 byte | 6 bytes | 4 bytes    |
+--------+---------+------------+
```

Campos:

| Campo      | Tamanho | Descrição                                   |
| ---------- | ------: | ------------------------------------------- |
| Type       |  1 byte | Valor `0x02`                                |
| MAC        | 6 bytes | MAC virtual atribuído ao grupo              |
| Virtual IP | 4 bytes | IP virtual atribuído, em network byte order |

A biblioteca converte o IP recebido para **host byte order** antes de armazená-lo.

---

# 6. Envio de Dados

O envio de bytes brutos é feito por:

```c
int phy_send(PhysicalHandle *h,
             uint32_t dst_ip,
             const uint8_t *data,
             size_t len);
```

O parâmetro `dst_ip` deve estar em **host byte order**.

A biblioteca encapsula os dados na mensagem `DATA`.

---

## 6.1 Mensagem DATA

Formato:

```text
+--------+------------+----------+
| Type   | Dst IP     | Payload  |
+--------+------------+----------+
| 1 byte | 4 bytes    | N bytes  |
+--------+------------+----------+
```

Campos:

| Campo   | Tamanho | Descrição                                    |
| ------- | ------: | -------------------------------------------- |
| Type    |  1 byte | Valor `0x03`                                 |
| Dst IP  | 4 bytes | IP virtual de destino, em network byte order |
| Payload | N bytes | Bytes brutos enviados pelo usuário           |

A LibPhysical não interpreta o payload.

---

# 7. Recepção de Dados

A recepção é feita por:

```c
ssize_t phy_recv(PhysicalHandle *h,
                 uint32_t *src_ip,
                 uint8_t *buf,
                 size_t buf_len,
                 int timeout_ms);
```

A função bloqueia até receber um pacote de dados ou até o timeout expirar.

Mensagens de controle, como `MEDIUM_OK` e `PONG`, são descartadas por `phy_recv()`.

---

## 7.1 Mensagem DELIVER

Formato:

```text
+--------+------------+----------+
| Type   | Src IP     | Payload  |
+--------+------------+----------+
| 1 byte | 4 bytes    | N bytes  |
+--------+------------+----------+
```

Campos:

| Campo   | Tamanho | Descrição                                   |
| ------- | ------: | ------------------------------------------- |
| Type    |  1 byte | Valor `0x04`                                |
| Src IP  | 4 bytes | IP virtual de origem, em network byte order |
| Payload | N bytes | Bytes entregues ao usuário                  |

A biblioteca converte `Src IP` para **host byte order** antes de preencher `src_ip`.

Retornos de `phy_recv()`:

| Retorno | Significado               |
| ------: | ------------------------- |
|   `> 0` | Número de bytes recebidos |
|     `0` | Timeout                   |
|    `-1` | Erro                      |

---

# 8. Consulta do Meio

A função:

```c
int phy_medium_free(PhysicalHandle *h);
```

consulta se o meio físico simulado está livre.

Ela é usada como base para implementação de CSMA/CA pelos alunos.

---

## 8.1 Mensagem MEDIUM

Formato:

```text
+--------+
| Type   |
+--------+
| 1 byte |
+--------+
```

Valor:

```text
0x05
```

---

## 8.2 Mensagem MEDIUM_OK

Formato:

```text
+--------+--------+
| Type   | Free   |
+--------+--------+
| 1 byte | 1 byte |
+--------+--------+
```

Campos:

| Campo |  Valor | Significado         |
| ----- | -----: | ------------------- |
| Type  | `0x06` | Resposta à consulta |
| Free  | `0x00` | Meio ocupado        |
| Free  | `0x01` | Meio livre          |

Retornos de `phy_medium_free()`:

| Retorno | Significado  |
| ------: | ------------ |
|     `1` | Meio livre   |
|     `0` | Meio ocupado |
|    `-1` | Erro         |

---

# 9. Diagnóstico de Conectividade

A função:

```c
long phy_ping(PhysicalHandle *h);
```

mede o RTT até o nó central.

Ela envia uma mensagem `PING` e aguarda uma mensagem `PONG`.

---

## 9.1 Mensagem PING

Formato:

```text
+--------+
| Type   |
+--------+
| 1 byte |
+--------+
```

Valor:

```text
0x07
```

---

## 9.2 Mensagem PONG

Formato:

```text
+--------+----------+
| Type   | RTT Hint |
+--------+----------+
| 1 byte | 2 bytes  |
+--------+----------+
```

Campos:

| Campo    | Tamanho | Descrição                                              |
| -------- | ------: | ------------------------------------------------------ |
| Type     |  1 byte | Valor `0x08`                                           |
| RTT Hint | 2 bytes | Campo reservado para sugestão de RTT em microssegundos |

A implementação atual mede o RTT localmente no cliente usando relógio monotônico.

Retornos de `phy_ping()`:

| Retorno | Significado                  |
| ------: | ---------------------------- |
|  `>= 0` | RTT medido em microssegundos |
|    `-1` | Erro ou timeout              |

---

# 10. Encerramento

A função:

```c
void phy_disconnect(PhysicalHandle *h);
```

envia uma mensagem `BYE` ao nó central, fecha o socket e libera a memória do handle.

O envio de `BYE` é best-effort: falhas no envio são ignoradas.

---

## 10.1 Mensagem BYE

Formato:

```text
+--------+
| Type   |
+--------+
| 1 byte |
+--------+
```

Valor:

```text
0x09
```

O nó central pode usar essa mensagem para liberar imediatamente o `group_id`.

---

# 11. Mensagens de Erro

O nó central pode enviar uma mensagem `ERROR`.

---

## 11.1 Mensagem ERROR

Formato:

```text
+--------+----------+
| Type   | Message  |
+--------+----------+
| 1 byte | N bytes  |
+--------+----------+
```

Campos:

| Campo   | Tamanho | Descrição                      |
| ------- | ------: | ------------------------------ |
| Type    |  1 byte | Valor `0xFF`                   |
| Message | N bytes | Texto UTF-8 descrevendo o erro |

A biblioteca atual não expõe diretamente mensagens `ERROR` pela API pública.

---

# 12. API Pública

## 12.1 `phy_connect`

```c
PhysicalHandle *phy_connect(const char *group_id,
                            const char *host,
                            uint16_t port);
```

Conecta ao nó central e registra o grupo.

Retorna:

| Retorno         | Significado |
| --------------- | ----------- |
| Ponteiro válido | Sucesso     |
| `NULL`          | Erro        |

---

## 12.2 `phy_mac_addr`

```c
void phy_mac_addr(PhysicalHandle *h,
                  uint8_t mac_out[6]);
```

Copia o MAC virtual para `mac_out`.

---

## 12.3 `phy_virtual_ip`

```c
uint32_t phy_virtual_ip(PhysicalHandle *h);
```

Retorna o IP virtual do grupo em **host byte order**.

Exemplo:

```c
uint32_t ip = phy_virtual_ip(h);
```

Para imprimir com `inet_ntoa()`, converta para network byte order:

```c
struct in_addr addr;
addr.s_addr = htonl(ip);
printf("%s\n", inet_ntoa(addr));
```

---

## 12.4 `phy_ping`

```c
long phy_ping(PhysicalHandle *h);
```

Mede o RTT até o nó central.

Retorna:

| Retorno | Significado           |
| ------: | --------------------- |
|  `>= 0` | RTT em microssegundos |
|    `-1` | Erro ou timeout       |

---

## 12.5 `phy_medium_free`

```c
int phy_medium_free(PhysicalHandle *h);
```

Consulta se o meio está livre.

Retorna:

| Retorno | Significado  |
| ------: | ------------ |
|     `1` | Meio livre   |
|     `0` | Meio ocupado |
|    `-1` | Erro         |

---

## 12.6 `phy_send`

```c
int phy_send(PhysicalHandle *h,
             uint32_t dst_ip,
             const uint8_t *data,
             size_t len);
```

Envia bytes brutos para um IP virtual.

Retorna:

| Retorno | Significado |
| ------: | ----------- |
|     `0` | Sucesso     |
|    `-1` | Erro        |

---

## 12.7 `phy_recv`

```c
ssize_t phy_recv(PhysicalHandle *h,
                 uint32_t *src_ip,
                 uint8_t *buf,
                 size_t buf_len,
                 int timeout_ms);
```

Recebe bytes brutos.

Retorna:

| Retorno | Significado     |
| ------: | --------------- |
|   `> 0` | Bytes recebidos |
|     `0` | Timeout         |
|    `-1` | Erro            |

---

## 12.8 `phy_disconnect`

```c
void phy_disconnect(PhysicalHandle *h);
```

Encerra a conexão com o nó central e libera recursos.

Após esta chamada, o handle não deve ser usado novamente.

---

# 13. Endereços Reservados

## 13.1 Servidor Virtual

```c
#define PHY_SERVER_IP 0x0A000001u
```

Representa o IP virtual:

```text
10.0.0.1
```

A constante está em **host byte order**.

---

# 14. Limitações da Versão Atual

A versão atual possui as seguintes limitações:

* `phy_ping()` possui timeout fixo de 2 segundos.
* `phy_recv()` usa buffer interno de 2048 bytes.
* Payloads recebidos acima do tamanho do buffer do usuário são truncados.
* `ERROR` não é exposto como mensagem estruturada para a aplicação.
* `BYE` é enviado em modo best-effort.
* A biblioteca não implementa confiabilidade, ordenação, retransmissão ou verificação de integridade.


