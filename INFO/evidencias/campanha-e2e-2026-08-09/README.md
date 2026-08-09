# Evidência — campanha E2E completa

Data: 2026-08-09 (UTC)
Resultado final: **APROVADO**

## Escopo

Foi executado o ciclo real da aplicação, sem inserção manual dos dados da campanha:

1. autenticação e criação do tenant pelo Control Plane;
2. criação de lista, dois contatos confirmados e template pelas APIs tenant-scoped;
3. criação e início da campanha pela API principal;
4. seleção dos destinatários e renderização pelo worker;
5. entrega SMTP para um capturador local sem retransmissão externa;
6. validação do archive, link, pixel, preferências e unsubscribe públicos;
7. validação de métricas e isolamento usando um segundo hostname de tenant.

O teste usou um container e banco PostgreSQL temporários. A instância principal não teve dados ou configurações alterados.

## Identificadores do cenário aprovado

| Recurso | Identificador |
|---|---|
| Tenant | `2ebd77d6-d37b-46ef-aaa1-cf0a30ee988f` |
| Host tenant | `e2e-campaign.mailview.test` |
| Lista | ID `3`, UUID `ea280011-44c8-4db5-bdb7-3fc8d0227784` |
| Template | ID `5` |
| Campanha | ID `3`, UUID `a4d46856-a221-4b79-81dd-a3be313d3a9d` |
| Link rastreado | UUID `543a7fa0-f88f-4cc3-815c-59e8345dd24e` |
| Destinatária Ana | ID `3`, UUID `7d56b9b6-761f-4951-880f-0bd898771477` |
| Destinatário Bruno | ID `4`, UUID `05b2d2de-2167-45e2-9143-434f916b15e9` |

Todos os endereços usados terminam em `example.test` e não são entregáveis na internet.

## Prova do envio

O worker registrou:

```text
2026/08/09 12:57:09.283903 manager.go:469: start processing campaign (Campanha E2E Final com Tracking)
2026/08/09 12:57:09.864331 pipe.go:236: campaign (Campanha E2E Final com Tracking) finished
```

Estado final retornado por `GET /api/campaigns/3` no host correto:

```json
{
  "id": 3,
  "uuid": "a4d46856-a221-4b79-81dd-a3be313d3a9d",
  "status": "finished",
  "to_send": 2,
  "sent": 2,
  "views": 1,
  "clicks": 1,
  "bounces": 0,
  "started_at": "2026-08-09T12:57:09.18935Z",
  "updated_at": "2026-08-09T12:57:09.790225Z",
  "archive": true,
  "archive_slug": "campanha-e2e-final-tracking"
}
```

O capturador SMTP recebeu exatamente duas mensagens:

```text
captured message=3 from=<sender@example.test> to=[<bruno.e2e@example.test>] bytes=1290
captured message=4 from=<sender@example.test> to=[<ana.e2e@example.test>] bytes=1284
```

Artefatos preservados:

| Destinatário | Arquivo | SHA-256 do artefato versionado |
|---|---|---|
| Ana | [ana.eml](ana.eml) | `b67c5f66049cd556e24cde2b3cf98302cdb30403573db549df12d82a9db0741c` |
| Bruno | [bruno.eml](bruno.eml) | `c63c37df06623dbd455b84dc4bde2ba9a7319412ec6d361fd048539d566d21c2` |

As mensagens comprovam:

- assunto e saudação personalizados por contato;
- `X-Listmonk-Campaign` e `X-Listmonk-Subscriber` corretos;
- `List-Unsubscribe` e `List-Unsubscribe-Post: List-Unsubscribe=One-Click`;
- URL de clique convertida para `/link/{link}/{campaign}/{subscriber}`;
- pixel convertido para `/campaign/{campaign}/{subscriber}/px.png`;
- template base e corpo da campanha renderizados juntos.

Como `privacy.individual_tracking=false` no cenário, clique e pixel usam intencionalmente o UUID zero e produzem métricas agregadas, não identificação individual.

## Provas HTTP pós-entrega

| Fluxo | Resultado |
|---|---|
| Link no host correto | `307 Temporary Redirect` para `https://example.test/e2e-destination` |
| Pixel no host correto | `200 OK`, `Content-Type: image/png`, 74 bytes |
| Archive no host correto | `200 OK`, HTML com 452 bytes |
| Preferências da Ana | `200 OK`, HTML com 4009 bytes |
| Unsubscribe da Ana | `200 OK` |

Após uma abertura e um clique, o banco registrou:

```text
 campaign_id |  status  | to_send | sent |              tenant_id               | views | clicks
-------------+----------+---------+------+--------------------------------------+-------+--------
           3 | finished |       2 |    2 | 2ebd77d6-d37b-46ef-aaa1-cf0a30ee988f |     1 |      1
```

O link persistido pertence ao mesmo tenant:

```text
                 url                  |              tenant_id               | clicks
--------------------------------------+--------------------------------------+--------
 https://example.test/e2e-destination | 2ebd77d6-d37b-46ef-aaa1-cf0a30ee988f |      1
```

O unsubscribe alterou somente a assinatura selecionada:

```text
         email          | subscription_status |              tenant_id
------------------------+---------------------+--------------------------------------
 ana.e2e@example.test   | unsubscribed        | 2ebd77d6-d37b-46ef-aaa1-cf0a30ee988f
 bruno.e2e@example.test | confirmed           | 2ebd77d6-d37b-46ef-aaa1-cf0a30ee988f
```

## Prova negativa de isolamento

Foi criado o tenant de controle `e2e-isolation.mailview.test`, também com membership ativa para o usuário do teste. A campanha do tenant original foi então consultada pelo hostname de controle:

| Tentativa pelo tenant errado | Resultado |
|---|---|
| `GET /archive/campanha-e2e-final-tracking` | `404 Not Found` |
| `GET /api/campaigns/3` | `400 Bad Request`, `Campaign not found` |
| `GET /link/{link}/{campaign}/{subscriber}` | `400 Bad Request`, sem redirect para o destino |

Logo, conhecer ID, UUID, slug ou URL de tracking não permitiu atravessar a fronteira do tenant.

## Defeitos encontrados e corrigidos durante o ensaio

O ensaio começou em banco vazio e revelou quatro problemas que testes unitários não alcançavam:

1. `--install` preparava queries tenant-aware antes das migrations MailView. A instalação agora aplica as migrations de extensão antes de preparar os statements em `cmd/install.go`.
2. `sqlx.Tx.Stmtx()` descartava o modo `Unsafe` herdado do registry upstream. `models/queries.go` agora o preserva explicitamente.
3. Resolvers `SECURITY DEFINER` pertencentes à role restrita não atravessavam `FORCE RLS`. O worker agora aprende campanha→tenant enquanto enumera tenants dentro de RLS e usa busca tenant-scoped como fallback; bounces seguem a mesma regra, sem `BYPASSRLS`.
4. Filtros de listas baseados no RBAC legado ainda podiam interferir em campanhas tenant. `cmd/campaigns.go` agora usa as permissões MailView da rota e deixa RLS/FKs validarem ownership.

Depois das correções, o mesmo banco retomou a campanha que estava em `running`, entregou as mensagens e fechou os contadores sem intervenção manual.

## Limite desta evidência

O teste prova o ciclo da aplicação e o protocolo SMTP até a aceitação `250` por um servidor local. Ele não mede reputação, DNS, SPF/DKIM/DMARC, caixa de entrada de provedores externos ou entregabilidade na internet. Esses itens exigem domínio e infraestrutura SMTP reais por tenant.
