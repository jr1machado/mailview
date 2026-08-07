# Estado de entrega e pendências

Atualizado em 2026-08-06. Este documento descreve o estado real da branch `feature/sprint01`.

## Entregue

### Fase 0 — Fundação

- ADRs, threat model e convenções de extensão do fork.
- Compose de produção de referência, secrets por arquivo e imagem runtime não-root.
- Workflow de qualidade e segurança.

### Fase 1 — Control Plane e identidade

- Migrations MailView independentes do registry upstream.
- Tenants, memberships, papéis padrão, permissões e audit trail append-only.
- Endpoints administrativos do Control Plane protegidos pelo Super Admin atual.
- TOTP novo cifrado com AES-256-GCM quando a chave é configurada.
- Recovery codes bcrypt, de uso único no login.

### Fase 2 — Base de isolamento e primeiro agregado

- Contexto transacional de tenant com `set_config(..., true)`.
- Tabela `mv_tenant_settings` sob `FORCE ROW LEVEL SECURITY` e teste RLS com duas tenants.
- `tenant_id`, índices e FKs compostas em `subscribers`, `lists` e `subscriber_lists`.
- Data Plane tenant-scoped para CRUD básico de contatos e listas.
- Resolução opcional de tenant pelo host `{slug}.{tenant_base_domain}` e validação de membership ativa.
- Migração tenant-scoped de leitura/criação, CRUD individual e exportação CSV das rotas legadas quando `tenant_routing_enabled=true`.
- Testes PostgreSQL para migration, Control Plane, RLS e isolamento de contatos/listas.

### Importação tenant-scoped (CSV)

- `mv_import_jobs` e `mv_import_files` com `tenant_id`, `actor_id`, status, idempotency key e assinatura HMAC do payload, ambas sob `FORCE ROW LEVEL SECURITY`.
- Upload gravado em `import_storage_dir/<tenant_id>/<job_id>.csv`, prefixo isolado por tenant.
- Worker CSV (`internal/mailview/importjob`) processa em lotes de 500 linhas por `tenant.InTransaction`, resolvendo email/name e criando `subscriber_lists` apenas para listas confirmadas da própria tenant.
- Ownership das listas é validada em `CreateJob` (todas as `list_ids` precisam existir na tenant do ator) e revalidada antes do processamento via reconsulta tenant-scoped.
- Endpoints tenant-scoped: `POST/GET /api/mailview/tenants/:tenantID/data/import-jobs`, `GET .../import-jobs/:jobID`, `POST .../import-jobs/:jobID/cancel`.
- Idempotency key por tenant evita duplicar jobs em retries.
- Teste de integração cobrindo criação concorrente entre duas tenants, rejeição de lista cross-tenant e replay de idempotency key (`internal/mailview/importjob/import_integration_test.go`).
- ZIP e compatibilidade integral com o importador upstream (`internal/subimporter`) ainda não migrados — o worker atual só aceita CSV com colunas `email`/`name`.
- UI Vue de importação ainda aponta para as rotas globais legadas.

## Pendências

### Contatos e listas antes de ativar RLS

- Migrar operações em lote: bloqueio, exclusão e associação a listas.
- Migrar ZIP/importação avançada (upstream `internal/subimporter`) e todos os endpoints de detalhe ainda globais (activity, bounces, opt-in e subscriptions públicas).
- Migrar a UI Vue para o contrato tenant-scoped ou desabilitar explicitamente os fluxos legados quando o tenant routing estiver ativo.
- Habilitar `ENABLE` e `FORCE ROW LEVEL SECURITY` em `subscribers`, `lists` e `subscriber_lists` somente após os itens acima.
- Criar testes cross-tenant para CRUD, exportação, importação, lote, IDOR e jobs.

### Restante da Fase 2

- Migrar templates, campanhas, campanhas/listas, mídia, links, analytics, bounces e jobs de envio para `tenant_id` + transações tenant-scoped.
- Criar FKs compostas, índices e RLS por agregado.
- Resolver tenant para fluxos públicos (tracking, unsubscribe, formulários e arquivos) sem confiar em parâmetros editáveis pelo cliente.

### Fases posteriores

- Portal visual do cliente e portal administrativo MailView.
- Domínios personalizados, DNS verification, SMTP e tracking por tenant.
- Quotas, billing, onboarding e suspensão.
- Enterprise: SSO, infra dedicada, SIEM e chaves dedicadas.

## Regra de ativação de RLS

RLS não deve ser habilitado nas tabelas legadas enquanto existir rota, worker ou tarefa pública usando o pool global do Listmonk. Cada agregado só entra em RLS após todas as consultas correspondentes usarem o contexto transacional de tenant e houver teste cross-tenant automatizado.
