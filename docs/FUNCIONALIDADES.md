# Funções e recursos do MailView v0.4.0

## Comunicação e dados

- campanhas: CRUD, preview, teste, conteúdo, status, agendamento/envio, archive, anexos e exclusão;
- transacionais por `/api/tx`;
- templates HTML/texto, editor visual, preview e default;
- múltiplos SMTPs, grupo balanceado `email`, SMTP nomeado e postback HTTP;
- contatos com atributos, blocklist, operações em lote, export/wipe e unicidade por tenant;
- listas públicas/privadas, segmentação, single/double opt-in e preferências;
- import legado no workspace e import CSV MailView (`email`/`name`) com listas alvo, HMAC, idempotência, progresso e cancelamento;
- dashboard, aberturas, cliques, visualização web, bounces e complaints;
- páginas públicas de inscrição, archive, unsubscribe, privacy e tracking.

## Multi-tenancy e storage

Isolamento por transação + FORCE RLS, subdomínio/domínio verificado, tenant suspenso bloqueado, worker de campanha tenant-aware, filesystem/S3 prefixado por tenant e proteção contra traversal.

## RBAC do tenant

Papéis: Tenant Owner, Tenant Admin, Campaign Manager, Operator, Analyst, Viewer e Billing Manager. Permissões:

```text
campaign.create/approve/read/manage/send.tenant  analytics.read.tenant
subscriber.read/manage/import/export.tenant      list.read/manage.tenant
template.read/manage.tenant  media.read/manage.tenant  bounce.read/manage.tenant
domain.manage.tenant  user.invite/manage.tenant  audit.read.tenant
smtp.manage.tenant  billing.read/manage.tenant
```

Growth/Enterprise permitem papel customizado. Grants são aditivos; denial explícito prevalece.

## Plataforma

Papéis: Platform Super Admin, Operations, Support, Security, Billing e Auditor. Permissões separam gestão/suspensão de tenant, memberships, billing, auditoria, impersonação, segurança e administração dos papéis de plataforma.

O portal permite tenants/status, memberships/RBAC, auditoria, domínios e verificação manual, planos/quotas, owner, registro de infraestrutura, roles/denials, assignments de plataforma e impersonação.

Planos seed: Starter (2.000 contatos/10.000 e-mails/1 domínio/3 seats), Growth (25.000/150.000/3/10) e Enterprise (limites nulos). Eles são catálogo e registro: enforcement e cobrança não estão implementados.

## Identidade e operação

Usuários locais/API token/OIDC; TOTP MailView cifrado e recovery codes; impersonação Data Plane com TTL/motivo/MFA/auditoria; UI por permissões; i18n; health, logs, events, manutenção; binário único, install/upgrade e container não-root.

## Não implementado

Billing/gateway, enforcement de quotas, DNS/TLS automático por tenant, infraestrutura dedicada provisionada, SMTP tenant-scoped, worker separado, Redis/fila distribuída, retomada de import órfão, workflow formal de aprovação, Prometheus nativo e HA/autoscaling. Não devem ser comercializados como prontos.
