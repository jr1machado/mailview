# API MailView v0.4.0

Rotas administrativas exigem autenticação. Data Plane exige tenant ativo, membership/grant, permissão e transação RLS.

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

- `GET/POST /tenants`; `GET/PATCH /tenants/:tenantID`;
- `GET/POST /tenants/:tenantID/roles` e `POST/DELETE .../roles/:roleID/permissions/:code/deny`;
- `GET/POST /tenants/:tenantID/memberships` e `PUT .../:membershipID/roles`;
- `GET /tenants/:tenantID/audit-events`;
- `GET/POST /tenants/:tenantID/data/lists|subscribers|import-jobs`, detalhe/cancelamento de job;
- `GET/POST /tenants/:tenantID/domains`, `POST .../:domainID/verify|revoke`;
- `GET/PUT /tenants/:tenantID/quota`, `GET /plans`, `GET /dashboard`;
- `POST /tenants/:tenantID/owner|infrastructure`.

## Plataforma

`GET /api/mailview/platform/roles`, `GET/POST /assignments`, `DELETE /assignments/:userID/:roleID`, protegidos por `platform.roles.manage`.

`POST/GET /api/mailview/platform/impersonation` e `POST /:grantID/revoke`, protegidos por `support.impersonate.platform`. Criação exige TOTP recente, razão ≥10 caracteres e TTL ≤30 minutos. O grant não concede billing/plataforma.

`POST /api/mailview/profile/mfa/recovery-codes` substitui os recovery codes do usuário autenticado.

## Público e integrações

Login/2FA/reset/OIDC; `/api/public/lists|subscription|archive`; `/subscription/*`; `/archive*`; tracking `/link/*`, `/campaign/*` e pixel; `POST /webhooks/bounce` ou `/webhooks/service/:service`; health `/health` e `/api/health`.

Settings, SMTP test, logs/events, users, roles core, maintenance e `/api/tx` conservam os paths herdados e permissões em `permissions.json`. Formatos do core estão em `docs/docs/content/apis`; as regras MailView desta página prevalecem em host tenant.
