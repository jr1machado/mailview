# Arquitetura do MailView — v0.6.0

## Controles da Fase 4

O workflow de campanhas é um sidecar tenant-scoped (`mv_campaign_workflows` e
eventos únicos por idempotency key) sobre a tabela compatível `campaigns`.
Cada transição usa lock de linha, RLS e auditoria; somente estados de execução
são refletidos no enum legado.

API keys são one-way (hash SHA-256 de tokens de alta entropia). Segredos
recuperáveis usam ciphertext com versão de chave ou referência a secret
manager. Impersonation não participa da autorização de plataforma, billing,
MFA ou segredos. O proxy encerra TLS 1.2/1.3 e publica HSTS.

## Fronteira do fork

MailView é um fork independente. O core herdado permanece em `cmd/`, `internal/core`, `internal/manager`, `models`, `queries` e `frontend`; a camada própria fica em `internal/mailview`, `cmd/mailview*.go`, tabelas `mv_*` e rotas `/api/mailview`. O módulo Go conserva `github.com/knadh/listmonk` somente por compatibilidade de imports.

Há dois trilhos de migrations: o core mantém seu migrador e MailView usa ledger `mv_schema_migrations` e advisory lock próprios.

## Componentes reais

| Componente | Responsabilidade |
|---|---|
| `MailView` | HTTP, REST, páginas públicas, SPA embarcada, campanhas, imports e bounces |
| Vue SPA | painel do tenant e portal da plataforma; Vue 2.7/Buefy/Vite |
| PostgreSQL | dados, sessões, filas persistentes, auditoria e RLS |
| Caddy/proxy | TLS, ACME e reverse proxy na topologia fornecida |
| filesystem/S3 | mídia por tenant |
| SMTP/postback | entrega externa |

Não existem serviços separados de frontend, worker ou billing, nem Redis em uso. Com `--passive`, o processo serve HTTP sem iniciar o scanner de campanhas. Imports rodam em goroutine no processo que recebeu o upload.

## Representação visual dos ambientes

O Control Plane representa o contrato de roteamento real de cada tenant em três
faixas. A UI lê `GET /api/mailview/tenants` e, para cada tenant, consulta
`GET /api/mailview/tenants/:tenantID/infrastructure`:

```text
MailView Control Plane
        │ roteamento + tenant boundary
        ├─ shared ───────────── recursos comuns + RLS/FKs tenant-first
        ├─ dedicated_requested  referências em provisionamento
        └─ dedicated ────────── DB/fila/SMTP/storage/KMS/namespace referenciados
```

O cartão não afirma provisionamento físico: ele exibe o estado persistido pelo
contrato `mv_tenant_infrastructure`. Em `shared`, o isolamento é lógico e
obrigatório no mesmo PostgreSQL; em `dedicated`, a aplicação exige todas as
referências antes de ativar a rota. A faixa exibida no portal do cliente usa o
tenant resolvido no servidor e mostra hostname e UUID abreviado, sem permitir
que o navegador escolha ou envie um `tenant_id` arbitrário.

## Planos lógicos

- **Edge:** painel, API, formulários, unsubscribe, archive, links, pixels e webhooks; o `Host` original é preservado.
- **Control Plane:** tenants, memberships, papéis, domínios, planos, quotas, owner, infraestrutura, auditoria e impersonação.
- **Data Plane:** contatos, listas, campanhas, templates, mídia, tracking, analytics e bounces no tenant resolvido.
- **Workers internos:** campaign manager, import CSV em lotes de 500, bounce manager, manutenção e mensagens transacionais.

## Fluxos principais

### Requisição autenticada tenant

```text
HTTPS → sessão/usuário → Host resolve tenant → status ativo → membership
 → permissão efetiva (denial vence grant) → tenant.Begin → SET LOCAL/RLS
 → query/efeito → audit event → COMMIT
```

O cliente não fornece `tenant_id` em campos editáveis. Em host de plataforma,
as APIs globais legadas não ganham contexto tenant; em host tenant, endpoints
globais sensíveis são bloqueados.

### Campanha

```text
draft → review → approved → scheduled → sending → completed
          └────────→ rejected       └────────────→ cancelled
```

O sidecar serializa a linha, valida a transição, persiste evento com chave de
idempotência, sincroniza somente estados executáveis com `campaigns` e grava
auditoria na mesma transação.

### Import assíncrono

```text
upload CSV → diretório <tenant>/<job> → assinatura HMAC → envelope com TTL
 → worker verifica assinatura/hash → tenant.Begin → lotes de 500 → progresso
```

### Impersonation

```text
Support + permissão → MFA recente + motivo + TTL → aprovação opcional externa
 → header de grant no host tenant → identidade efetiva limitada ao Data Plane
```

Billing, MFA, secrets e Control Plane nunca consultam o grant. A UI apresenta
banner enquanto o grant local estiver ativo.

## Isolamento

```text
request/job → resolve tenant → verifica status/RBAC → BEGIN
 → set_config(app.tenant_id, true)
 → set_config(app.user_id, true)
 → set_config(app.request_id, true)
 → queries → COMMIT/ROLLBACK
```

A role da aplicação deve ser `NOSUPERUSER NOBYPASSRLS`; o boot recusa configuração insegura. `ENABLE` + `FORCE ROW LEVEL SECURITY` protegem os agregados herdados tenant-aware, settings/domínios/imports e as 17 entidades nativas acrescentadas na Fase 3. Estas últimas usam política estrita: sem `app.tenant_id`, a consulta retorna zero linhas; o fallback `legacy-workspace` existe somente nos agregados herdados. FKs compostas impedem relacionamentos cross-tenant.

Tenant é resolvido por `{slug}.{tenant_base_domain}`, alias de slug ainda válido, hostname customizado verificado, membership da sessão, grant de impersonação ou envelope de job assinado. Alterações de slug geram alias auditado e redirect 308. Tenant suspenso não atende Data Plane nem páginas públicas. O workspace legado usa UUID reservado, sem visão global.

Jobs usam envelope HMAC que cobre `tenant_id`, `job_id`, tipo, emissão, expiração e hash do payload. O worker extrai o tenant apenas após verificar a assinatura e então abre `tenant.Begin`.

## Persistência MailView

- identidade: `mv_tenants`, `mv_memberships`, `mv_roles`, `mv_permissions` e associações/denials;
- segurança: `mv_mfa_methods`, `mv_mfa_recovery_codes`, `mv_audit_events`, `mv_impersonation_grants`, API keys e versões de chave;
- workflow: `mv_campaign_workflows` e `mv_campaign_workflow_events`, ambos sob RLS;
- operação global: `mv_platform_incidents`, papéis e assignments de plataforma;
- produto: branding, domínios, planos, quotas, usage, feature flags e infraestrutura;
- billing: accounts, subscriptions e invoices (modelo persistente, sem gateway nesta fase);
- envio: SMTP profiles, sender identities, sending domains/DNS, complaints, campaign events, webhooks/deliveries e transacionais;
- acesso: tenant sessions e API keys;
- jobs/operação: `mv_import_jobs`, `mv_import_files`, `mv_schema_migrations`.

Auditoria é append-only pela aplicação, não um armazenamento WORM contra o administrador do banco.

## Segurança

TOTP RFC 6238, AES-256-GCM, recovery codes bcrypt; impersonação com TOTP recente, motivo, alvo membro, TTL máximo de 30 minutos e auditoria; HMAC e diretório segregado para imports; normalização/prefixo de tenant em mídia; container UID 10001/read-only; secrets `_FILE`; TLS no proxy e banco em rede interna.

Domínios começam `pending`; o servidor fornece o registro TXT/CNAME esperado e consulta DNS antes de ativar. Uma rotina periódica revalida propriedade; falha remove o host do roteamento e revoga o estado do certificado. O controller de certificados reporta `pending|issued|failed|revoked` sem enviar a chave privada à aplicação.

No plano Enterprise, `mv_tenant_infrastructure` é a fonte de roteamento e guarda apenas referências para database, worker/queue, SMTP, storage, KMS e namespace. O modo `dedicated` só é aceito quando todas estão presentes e incrementa `routing_version`.

### Classificação e criptografia

- PII, conteúdo, billing e audit events ficam sob controles PostgreSQL/RLS;
- API keys MailView armazenam somente SHA-256 e prefixo de identificação;
- TOTP usa AES-256-GCM e versão de chave;
- SMTP recuperável aceita ciphertext versionado ou referência de secret manager;
- imports usam HMAC; senhas e recovery codes usam hashes apropriados;
- disco, backup, KMS e rotação física pertencem à plataforma de deploy;
- TLS termina no Caddy com mínimo 1.2, preferência 1.3 e HSTS.

## Escala e disponibilidade

Réplicas `--passive` aumentam capacidade HTTP; recomenda-se só uma instância ativa para o scanner nesta release. Todos os nós compartilham PostgreSQL e storage; em cluster prefira S3. O Compose mínimo tem pontos únicos de falha e não fornece eleição de líder, fila distribuída, HA ou autoscaling.

```text
DNS → Caddy :80/:443 → MailView :9000 → PostgreSQL :5432
                              ├→ SMTP/postback
                              ├→ S3 opcional
                              └→ OIDC opcional
```

Frontend/email-builder são compilados, e `stuffbin` incorpora SPA, SQL, templates e i18n no binário `MailView`. GoReleaser gera arquivos `MailView_*`, checksum MailView e imagens multi-arquitetura.
