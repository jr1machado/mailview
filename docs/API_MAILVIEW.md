# API MailView — v0.6.0

Rotas administrativas exigem autenticação. Data Plane exige tenant ativo, membership/grant, permissão e transação RLS.

## Portal tenant e workflow da Fase 4

| Método | Rota | Permissão |
|---|---|---|
| `GET` | `/api/mailview/home` | `analytics.read.tenant` |
| `GET` | `/api/mailview/campaigns/:id/workflow` | `campaign.read.tenant` |
| `POST` | `/api/mailview/campaigns/:id/workflow/transitions` | conforme o estado de destino |
| `GET/POST` | `/api/mailview/api-keys` | `apikey.manage.tenant` |
| `DELETE` | `/api/mailview/api-keys/:keyID` | `apikey.manage.tenant` |
| `GET/PUT` | `/api/mailview/contacts/:id/governance` | `subscriber.read/manage.tenant` |

Transições aceitam `idempotency_key` no JSON ou `Idempotency-Key` no header.
Agendamento exige `scheduled_at` futuro; aprovação/rejeição exige
`campaign.approve.tenant`; rejeição exige justificativa. O token de uma API key
aparece uma única vez na criação e nunca nas listagens.

Exemplo de transição:

```json
{
  "to_state": "scheduled",
  "scheduled_at": "2026-08-10T14:00:00Z",
  "idempotency_key": "schedule-campaign-42-revision-3",
  "reason": "janela aprovada pelo marketing"
}
```

Governança de contato aceita `tags`, `consent_status`
(`unknown|granted|withdrawn`), `consent_source`, `consent_at` e `suppressed`.
Supressão também coloca o contato em blocklist; retirar a flag não reativa
automaticamente um contato bloqueado por outro motivo.

Criação de API key aceita nome, lista de permissões tenant e validade opcional.
O serviço rejeita códigos `.platform`, duplicatas e qualquer permissão que o
ator não possua, evitando elevação por delegação.

## Data Plane em paths compatíveis

| Grupo | Paths | Permissão |
|---|---|---|
| dashboard | `/api/dashboard/charts`, `/counts` | `analytics.read.tenant` |
| contatos | `/api/subscribers*` | `subscriber.read/manage/export.tenant` |
| listas | `/api/lists*` | `list.read/manage.tenant` |
| campanhas | `/api/campaigns*` | `campaign.read/manage/send.tenant`, analytics |
| templates | `/api/templates*` | `template.read/manage.tenant` |
| mídia | `/api/media*` | `media.read/manage.tenant` |
| bounces | `/api/bounces*` | `bounce.read/manage.tenant` |
| import | `/api/mailview/data/import-jobs*` | `subscriber.import.tenant` |

## Control Plane `/api/mailview`

| Método | Rota | Função |
|---|---|---|
| `GET/POST` | `/tenants` | listar/criar tenants |
| `GET/PATCH` | `/tenants/:tenantID` | detalhe e alteração de status |
| `POST` | `/tenants/:tenantID/slug` | trocar slug e criar alias 308 |
| `GET/POST` | `/tenants/:tenantID/roles` | listar/criar papel customizado |
| `POST/DELETE` | `/tenants/:tenantID/roles/:roleID/permissions/:code/deny` | negar/remover negação explícita |
| `GET/POST` | `/tenants/:tenantID/memberships` | listar/criar membership |
| `PUT` | `/tenants/:tenantID/memberships/:membershipID/roles` | substituir papéis do membro |
| `GET` | `/tenants/:tenantID/audit-events` | consultar auditoria tenant |
| `GET/POST` | `/tenants/:tenantID/data/lists` | listar/criar listas no contexto tenant |
| `GET/POST` | `/tenants/:tenantID/data/subscribers` | listar/criar contatos no contexto tenant |
| `GET/POST` | `/tenants/:tenantID/data/import-jobs` | listar/criar importações CSV |
| `GET` | `/tenants/:tenantID/data/import-jobs/:jobID` | progresso e resultado do job |
| `POST` | `/tenants/:tenantID/data/import-jobs/:jobID/cancel` | solicitar cancelamento |
| `GET/POST` | `/tenants/:tenantID/domains` | listar/criar domínios |
| `POST` | `/tenants/:tenantID/domains/:domainID/verify` | consultar ownership no DNS |
| `POST` | `/tenants/:tenantID/domains/:domainID/revoke` | revogar domínio |
| `POST` | `/tenants/:tenantID/domains/:domainID/tls` | controller reportar estado TLS |
| `POST` | `/domains/revalidate` | revalidar domínios vencidos |
| `GET/PUT` | `/tenants/:tenantID/quota` | consultar/aplicar plano de quota |
| `GET` | `/plans` | listar catálogo de planos |
| `GET` | `/dashboard` | métricas globais da plataforma |
| `POST` | `/tenants/:tenantID/owner` | redefinir owner e invalidar sessão |
| `GET/POST` | `/tenants/:tenantID/infrastructure` | consultar/configurar rota de infraestrutura |

`POST .../slug` recebe `slug` e `redirect_days` (1–365). Criação de domínio recebe `hostname`, `purpose` e `verification_method` (`txt|cname`) e devolve `verification_name`/`verification_value`. `verify` consulta DNS ao vivo. Infraestrutura `dedicated` exige `database_ref`, `worker_ref`, `smtp_ref`, `storage_ref`, `encryption_key_ref` e `docker_namespace`.

O mapa visual da v0.6.0 consome a listagem de tenants e o `GET` de
infraestrutura. Ele é uma representação do contrato acima e não altera a
fronteira de autorização nem aceita `tenant_id` proveniente de um campo visual.

## Formato, contexto e erros

Respostas JSON seguem o envelope `{"data": ...}`. IDs MailView são UUID;
entidades compatíveis do core podem conservar IDs inteiros. Entradas inválidas,
conflitos, ausência e falta de permissão são convertidas para códigos HTTP 4xx;
falhas inesperadas retornam 5xx e devem ser correlacionadas pelos logs. O
cliente deve tratar retries de mutação somente quando a rota possuir
idempotency key explícita.

Em Data Plane, `tenant_id`, `user_id` e `request_id` são colocados com
`SET LOCAL` na transação. Em páginas públicas o hostname determina o tenant; em
jobs, um envelope HMAC determina o tenant depois da validação da assinatura.

## Plataforma

`GET /api/mailview/platform/roles`, `GET/POST /assignments`, `DELETE /assignments/:userID/:roleID`, protegidos por `platform.roles.manage`.

`POST/GET /api/mailview/platform/impersonation`, `POST /:grantID/revoke` e
`POST /:grantID/approve`, protegidos por `support.impersonate.platform`. Criação
exige TOTP recente, razão ≥10 caracteres e TTL ≤30 minutos. Com
`require_approval=true`, um operador diferente deve aprovar o grant. O grant é
enviado em `X-MailView-Impersonation` apenas no host tenant e não concede acesso
a billing, MFA, segredos ou plataforma.

Incidentes usam `GET/POST /api/mailview/platform/incidents` e
`POST /api/mailview/platform/incidents/:incidentID/resolve`, protegidos por
`incident.manage.platform` (Super Admin, Operations e Security).

Incidentes aceitam `title`, `severity` (`low|medium|high|critical`), `details`
e `tenant_id` opcional. A resolução registra operador, horário e audit event.

`POST /api/mailview/profile/mfa/recovery-codes` substitui os recovery codes do usuário autenticado.

## Público e integrações

Login/2FA/reset/OIDC; `/api/public/lists|subscription|archive`; `/subscription/*`; `/archive*`; tracking `/link/*`, `/campaign/*` e pixel; `POST /webhooks/bounce` ou `/webhooks/service/:service`; health `/health` e `/api/health`.

Settings, SMTP test, logs/events, users, roles core, maintenance e `/api/tx` conservam os paths herdados e permissões em `permissions.json`. Formatos do core estão em `docs/docs/content/apis`; as regras MailView desta página prevalecem em host tenant.
