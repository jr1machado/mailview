# Funções e recursos do MailView — v0.5.0

## Portais e governança da Fase 4

- home tenant com volume, campanhas ativas, contatos, bounces, plano, domínios
  pendentes e alertas de entregabilidade;
- dashboard global com tenants, MRR/ARR observado, filas, bounces, complaints,
  webhooks e consumo de infraestrutura;
- workflow de campanha com revisão, aprovação/rejeição, agendamento,
  idempotência e cancelamento auditável;
- API keys tenant com token exibido uma vez; tags, consentimento e supressão
  para contatos;
- impersonation com MFA recente, TTL, motivo, aprovação opcional por segundo
  operador e banner persistente.

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

## Matriz funcional da release

| Domínio | Função | Situação em v0.5.0 |
|---|---|---|
| identidade | usuário local, senha, sessão e reset | implementado pelo core |
| identidade | OIDC e TOTP | implementado; membership tenant continua explícita |
| segurança | recovery codes de uso único | implementado com hash bcrypt |
| tenants | criar, consultar, suspender, reativar e offboard | implementado e auditado |
| tenants | slug reservado, alteração e alias 308 | implementado |
| tenants | domínio customizado e ownership TXT/CNAME | implementado com revalidação |
| tenants | SPF/DKIM/DMARC e tracking final | configuração operacional externa |
| RBAC | papéis tenant/plataforma e memberships | implementado |
| RBAC | custom roles premium e denial explícito | implementado |
| campanhas | CRUD, preview, test send, archive e analytics | implementado |
| campanhas | review, approve/reject, schedule e cancel | implementado em API sidecar; UI herdada parcial |
| contatos | CRUD, atributos, listas, deduplicação e export | implementado |
| contatos | tags, consentimento e supressão | implementado por API de governança; UI parcial |
| import | CSV tenant, idempotência, HMAC e cancelamento | implementado; sem retomada automática |
| conteúdo | templates, editor visual e mídia | implementado e tenant-scoped |
| entrega | SMTP/messenger herdado | implementado |
| entrega | SMTP profile tenant novo | modelo/RLS pronto; dispatcher ainda não integrado |
| eventos | opens, clicks, views, bounces e complaints | tracking/bounces implementados; catálogo ampliado |
| webhooks | entradas de bounce por provedores | implementado |
| webhooks | dispatcher tenant `mv_webhooks` | modelo/RLS pronto; dispatcher pendente |
| billing | accounts, subscriptions, invoices e dashboard | modelo/consulta implementados; gateway pendente |
| quotas | catálogo, associação e exibição | implementado; enforcement pendente |
| suporte | impersonation com MFA/TTL/aprovação/banner | implementado |
| plataforma | dashboard, incidentes e auditoria | implementado; triagem de incidentes via API |
| Enterprise | contrato de recursos dedicados | implementado; provisionamento físico externo |
| API keys | criação, listagem, revogação e hash | implementado; autenticação HTTP pelo novo catálogo pendente |
| observabilidade | health, logs, eventos e audit trail | implementado; Prometheus/Otel pendente |

## Multi-tenancy e storage

Isolamento por transação + FORCE RLS, subdomínio/domínio verificado, tenant suspenso bloqueado, worker de campanha tenant-aware, filesystem/S3 prefixado por tenant e proteção contra traversal. Jobs destacados usam envelope HMAC com tenant e validade assinados. Logs correlacionam request, tenant e usuário.

Slugs possuem validação 3–63, lista reservada, troca auditada e alias temporário com redirect 308. Domínios suportam portal, tracking, sending, return-path e public forms; criação gera TXT/CNAME, verificação consulta DNS e revalidação periódica revoga host/TLS quando a propriedade é perdida.

## RBAC do tenant

Papéis: Tenant Owner, Tenant Admin, Campaign Manager, Operator, Analyst, Viewer e Billing Manager. Permissões:

```text
campaign.create/approve/read/manage/send/review/schedule/cancel/test.tenant
analytics.read.tenant  apikey.manage.tenant  security.read.tenant
subscriber.read/manage/import/export.tenant      list.read/manage.tenant
template.read/manage.tenant  media.read/manage.tenant  bounce.read/manage.tenant
domain.manage.tenant  user.invite/manage.tenant  audit.read.tenant
smtp.manage.tenant  billing.read/manage.tenant
```

Growth/Enterprise permitem papel customizado. Grants são aditivos; denial explícito prevalece.

## Plataforma

Papéis: Platform Super Admin, Operations, Support, Security, Billing e Auditor. Permissões separam gestão/suspensão de tenant, memberships, billing, auditoria, impersonação, incidentes, segurança e administração dos papéis de plataforma.

O portal permite tenants/status, memberships/RBAC, auditoria, slugs, domínios e verificação DNS, planos/quotas, owner, roteamento Enterprise, roles/denials, assignments de plataforma, incidentes e impersonação. O portal tenant mostra uso, campanhas, contatos, bounces, plano, domínios pendentes e alertas.

Planos seed: Starter (2.000 contatos/10.000 e-mails/1 domínio/3 seats), Growth (25.000/150.000/3/10) e Enterprise (limites nulos). Eles são catálogo e registro: enforcement e cobrança não estão implementados.

## Identidade e operação

Usuários locais/API token/OIDC; TOTP MailView cifrado e recovery codes; impersonação Data Plane com TTL/motivo/MFA/auditoria; UI por permissões; i18n; health, logs, events, manutenção; binário único, install/upgrade e container não-root.

## Modelo persistente da Fase 3

Branding, sessões tenant, API keys, billing accounts, subscriptions, invoices, feature flags, SMTP profiles, sender identities, sending domains/DNS, complaints, campaign events, exports, webhooks/deliveries e mensagens transacionais possuem tabelas, integridade, índices tenant-first e RLS. O catálogo viabiliza os serviços seguintes sem introduzir linhas globais ou relacionamentos cross-tenant.

## Limites desta release

Gateway/cobrança financeira, enforcement de quotas, emissão ACME embutida, provisionador físico de infraestrutura dedicada, entrega pelo novo SMTP profile tenant, dispatcher dos novos webhooks/transacionais, worker separado, Redis/fila distribuída, retomada de import órfão, Prometheus nativo e HA/autoscaling. O workflow formal de aprovação está implementado; a integração visual completa desse workflow no editor herdado ainda é um limite. Itens não entregues não devem ser comercializados como prontos.
