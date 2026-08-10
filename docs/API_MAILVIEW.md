# API MailView — v0.5.0

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

- `GET/POST /tenants`; `GET/PATCH /tenants/:tenantID`; `POST /tenants/:tenantID/slug`;
- `GET/POST /tenants/:tenantID/roles` e `POST/DELETE .../roles/:roleID/permissions/:code/deny`;
- `GET/POST /tenants/:tenantID/memberships` e `PUT .../:membershipID/roles`;
- `GET /tenants/:tenantID/audit-events`;
- `GET/POST /tenants/:tenantID/data/lists|subscribers|import-jobs`, detalhe/cancelamento de job;
- `GET/POST /tenants/:tenantID/domains`, `POST .../:domainID/verify|revoke|tls`; `POST /domains/revalidate`;
- `GET/PUT /tenants/:tenantID/quota`, `GET /plans`, `GET /dashboard`;
- `POST /tenants/:tenantID/owner`; `GET/POST /tenants/:tenantID/infrastructure`.

`POST .../slug` recebe `slug` e `redirect_days` (1–365). Criação de domínio recebe `hostname`, `purpose` e `verification_method` (`txt|cname`) e devolve `verification_name`/`verification_value`. `verify` consulta DNS ao vivo. Infraestrutura `dedicated` exige `database_ref`, `worker_ref`, `smtp_ref`, `storage_ref`, `encryption_key_ref` e `docker_namespace`.

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
