# Estado de entrega e pendências

Atualizado em 2026-08-09. Este documento descreve o estado do diretório de trabalho atual.

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
- `ENABLE` e `FORCE ROW LEVEL SECURITY` ativos em `subscribers`, `lists` e `subscriber_lists`; acessos sem contexto ficam restritos ao `legacy-workspace`.
- RBAC tenant aplicado às rotas autenticadas de contatos/listas, com permissões distintas para leitura, gestão, importação e exportação.
- Operações em lote por IDs (bloqueio, exclusão e associação a listas) tenant-scoped, atômicas e com rejeição cross-tenant.
- `tenant_id`, FKs compostas, índices e `FORCE RLS` em templates, campanhas, relações campanha/lista, views, mídia, mídia de campanha, links, cliques e bounces.
- APIs legadas consumidas pela interface principal executadas em transação tenant-scoped para campanhas, templates, mídia, analytics, bounces e dashboard.
- Perfil e navegação Vue baseados nas permissões efetivas da membership; APIs administrativas globais bloqueadas em hosts tenant.
- Resolução server-side do tenant por subdomínio ou domínio verificado nos fluxos públicos de inscrição, opt-in, unsubscribe, privacidade, arquivos e tracking.
- Worker de envio, anexos, criação de links e gravação de bounces isolados pela tenant da campanha/assinante.
- Provider local de mídia com prefixo físico por tenant e proteção contra traversal.
- Teste PostgreSQL cross-tenant cobrindo todos os doze conjuntos sob `FORCE RLS`.
- Tela Vue de importação conectada aos jobs CSV tenant-scoped, com status, progresso e cancelamento entre lotes.

### Operação de plataforma

- Papéis fixos de plataforma e permissões granulares separados dos papéis de tenant, com atribuição/revogação auditada.
- Portal administrativo para tenants, suspensão/reativação, planos/quota, domínios, troca de owner e solicitação de infraestrutura dedicada.
- Grants de impersonation com justificativa, MFA recente, expiração curta e revogação, limitados ao Data Plane do tenant alvo.
- Navegação administrativa baseada nas permissões efetivas do operador de plataforma, mantendo o Super Admin legado como ponte de compatibilidade.

### Importação tenant-scoped (CSV)

- `mv_import_jobs` e `mv_import_files` com `tenant_id`, `actor_id`, status, idempotency key e assinatura HMAC do payload, ambas sob `FORCE ROW LEVEL SECURITY`.
- Upload gravado em `import_storage_dir/<tenant_id>/<job_id>.csv`, prefixo isolado por tenant.
- Worker CSV (`internal/mailview/importjob`) processa em lotes de 500 linhas por `tenant.InTransaction`, resolvendo email/name e criando `subscriber_lists` apenas para listas confirmadas da própria tenant.
- Ownership das listas é validada em `CreateJob` (todas as `list_ids` precisam existir na tenant do ator) e revalidada antes do processamento via reconsulta tenant-scoped.
- Endpoints tenant-scoped: `POST/GET /api/mailview/tenants/:tenantID/data/import-jobs`, `GET .../import-jobs/:jobID`, `POST .../import-jobs/:jobID/cancel`.
- Idempotency key por tenant evita duplicar jobs em retries.
- Teste de integração cobrindo criação concorrente entre duas tenants, rejeição de lista cross-tenant e replay de idempotency key (`internal/mailview/importjob/import_integration_test.go`).
- ZIP e compatibilidade integral com o importador upstream (`internal/subimporter`) ainda não migrados — o worker atual só aceita CSV com colunas `email`/`name`.

## Pendências

### Contatos, listas e importação

- Migrar ZIP/importação avançada, atributos e modos adicionais do upstream `internal/subimporter`; a interface tenant expõe somente o contrato CSV seguro (`email`/`name`).
- Migrar expressões SQL em lote para uma linguagem de filtros tenant-safe; no momento elas são recusadas em hosts tenant.
- Completar testes HTTP de RBAC/IDOR, além dos testes de serviço e PostgreSQL existentes.

### Operação multi-tenant posterior

- Tornar perfis SMTP, remetentes e configurações de entrega específicos por tenant; o transporte de e-mail ainda usa a configuração operacional global.
- Automatizar verificação DNS e ciclo de vida dos domínios personalizados; o roteamento público só aceita registros já marcados como verificados.
- Adicionar testes HTTP end-to-end da interface principal e dos fluxos públicos em hostnames de tenants distintos.
- Avaliar storage de objetos por tenant para produção; o provider filesystem já usa prefixo isolado.

### Fases posteriores

- Evoluir o portal visual do cliente e o portal administrativo MailView além da adaptação tenant-aware da interface atual.
- DNS verification automatizada, SMTP e identidade de remetente por tenant.
- Quotas, billing, onboarding e suspensão.
- Enterprise: SSO, infra dedicada, SIEM e chaves dedicadas.

## Regra de ativação de RLS para os próximos agregados

Cada novo agregado só entra em RLS após suas consultas SaaS usarem o contexto transacional de tenant e haver teste cross-tenant automatizado. Durante a migração incremental, consultas upstream sem contexto devem ficar confinadas ao workspace legado e nunca receber uma política de acesso global.
