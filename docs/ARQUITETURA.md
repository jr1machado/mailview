# Arquitetura do MailView v0.4.0

## Fronteira do fork

MailView é um fork independente. O core herdado permanece em `cmd/`, `internal/core`, `internal/manager`, `models`, `queries` e `frontend`; a camada própria fica em `internal/mailview`, `cmd/mailview*.go`, tabelas `mv_*` e rotas `/api/mailview`. O módulo Go conserva `github.com/knadh/listmonk` somente por compatibilidade de imports.

Há dois trilhos de migrations: o core mantém seu migrador e MailView usa ledger `mv_schema_migrations` e advisory lock próprios.

## Componentes reais

| Componente | Responsabilidade |
|---|---|
| `mailview` | HTTP, REST, páginas públicas, SPA embarcada, campanhas, imports e bounces |
| Vue SPA | painel do tenant e portal da plataforma; Vue 2.7/Buefy/Vite |
| PostgreSQL | dados, sessões, filas persistentes, auditoria e RLS |
| Caddy/proxy | TLS, ACME e reverse proxy na topologia fornecida |
| filesystem/S3 | mídia por tenant |
| SMTP/postback | entrega externa |

Não existem serviços separados de frontend, worker ou billing, nem Redis em uso. Com `--passive`, o processo serve HTTP sem iniciar o scanner de campanhas. Imports rodam em goroutine no processo que recebeu o upload.

## Planos lógicos

- **Edge:** painel, API, formulários, unsubscribe, archive, links, pixels e webhooks; o `Host` original é preservado.
- **Control Plane:** tenants, memberships, papéis, domínios, planos, quotas, owner, infraestrutura, auditoria e impersonação.
- **Data Plane:** contatos, listas, campanhas, templates, mídia, tracking, analytics e bounces no tenant resolvido.
- **Workers internos:** campaign manager, import CSV em lotes de 500, bounce manager, manutenção e mensagens transacionais.

## Isolamento

```text
request/job → resolve tenant → verifica status/RBAC → BEGIN
 → set_config(app.tenant_id, true)
 → set_config(app.user_id, true)
 → set_config(app.request_id, true)
 → queries → COMMIT/ROLLBACK
```

A role da aplicação deve ser `NOSUPERUSER NOBYPASSRLS`; o boot recusa configuração insegura. `ENABLE` + `FORCE ROW LEVEL SECURITY` protegem `mv_tenant_settings`, `mv_tenant_domains`, `mv_import_jobs`, `mv_import_files`, `subscribers`, `lists`, `subscriber_lists`, `templates`, `campaigns`, `campaign_lists`, `campaign_views`, `media`, `campaign_media`, `links`, `link_clicks` e `bounces`. FKs compostas impedem relacionamentos cross-tenant.

Tenant é resolvido por `{slug}.{tenant_base_domain}`, hostname customizado verificado, membership da sessão, grant de impersonação ou tenant persistido no job/campanha. Tenant suspenso não atende Data Plane nem páginas públicas. O workspace legado usa UUID reservado, sem visão global.

## Persistência MailView

- identidade: `mv_tenants`, `mv_memberships`, `mv_roles`, `mv_permissions` e associações/denials;
- segurança: `mv_mfa_methods`, `mv_mfa_recovery_codes`, `mv_audit_events`, `mv_impersonation_grants`;
- produto: `mv_tenant_domains`, `mv_tenant_plans`, `mv_tenant_quotas`, `mv_tenant_usage`, `mv_tenant_infrastructure`;
- jobs/operação: `mv_import_jobs`, `mv_import_files`, `mv_schema_migrations`.

Auditoria é append-only pela aplicação, não um armazenamento WORM contra o administrador do banco.

## Segurança

TOTP RFC 6238, AES-256-GCM, recovery codes bcrypt; impersonação com TOTP recente, motivo, alvo membro, TTL máximo de 30 minutos e auditoria; HMAC e diretório segregado para imports; normalização/prefixo de tenant em mídia; container UID 10001/read-only; secrets `_FILE`; TLS no proxy e banco em rede interna.

## Escala e disponibilidade

Réplicas `--passive` aumentam capacidade HTTP; recomenda-se só uma instância ativa para o scanner nesta release. Todos os nós compartilham PostgreSQL e storage; em cluster prefira S3. O Compose mínimo tem pontos únicos de falha e não fornece eleição de líder, fila distribuída, HA ou autoscaling.

```text
DNS → Caddy :80/:443 → mailview :9000 → PostgreSQL :5432
                              ├→ SMTP/postback
                              ├→ S3 opcional
                              └→ OIDC opcional
```

Frontend/email-builder são compilados, e `stuffbin` incorpora SPA, SQL, templates e i18n no binário. GoReleaser gera `MailView_*` e imagens multi-arquitetura.
