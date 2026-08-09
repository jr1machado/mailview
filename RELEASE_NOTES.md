# MailView — Release notes

## MailView v0.4.0 — `mailview-v0.4.0` — 2026-08-09

Release que conclui o isolamento multi-tenant da Sprint/Fase 2 e consolida as funções administrativas já implementadas das Fases 3 e 4. É a primeira release em que binário, pacotes, imagens e pipeline usam o nome MailView de ponta a ponta.

### Isolamento e Data Plane

- `tenant_id`, integridade composta e `ENABLE` + `FORCE RLS` para contatos, listas, templates, campanhas, mídia, links, tracking e bounces;
- rotas administrativas do core passam por contexto transacional e permissões MailView;
- UI publica/consome somente permissões efetivas e bloqueia áreas incompatíveis;
- páginas públicas e tracking resolvem subdomínio ou domínio verificado, bloqueando tenant suspenso;
- worker de campanha, anexos, links e bounces preservam tenant;
- filesystem/S3 usa prefixo UUID e rejeita traversal/cross-tenant.

### Control Plane e segurança

- tenants, memberships, 7 papéis padrão, custom roles, grants e denial explícito;
- 6 papéis de plataforma e administração de assignments;
- TOTP AES-256-GCM, recovery codes bcrypt e MFA recente para ações sensíveis;
- auditoria append-only;
- domínios, planos Starter/Growth/Enterprise, quotas, usage e estado de infraestrutura;
- impersonação de suporte com razão, TTL máximo de 30 minutos, revogação e escopo apenas Data Plane;
- portal Vue para operações de plataforma.

### Importação

- CSV tenant-scoped com `email`/`name`, listas alvo, limite de upload, idempotency key e HMAC;
- diretório por tenant/job, lote transacional de 500, progresso, polling e cancelamento;
- testes de concorrência, replay e rejeição cross-tenant.

### Produto, build e operação

- binário renomeado para `mailview` e frontend package `mailview` 0.4.0;
- arquivos GoReleaser `MailView_0.4.0_<os>_<arch>.tar.gz` e imagens `ghcr.io/jr1machado/mailview`;
- workflow acionado por `mailview-v*`;
- Go 1.26.5 e dependências `goldmark`, `x/text` e `x/image` atualizadas para corrigir os achados alcançáveis do `govulncheck`;
- imagem não-root em `/mailview` e Compose local com container/rede MailView;
- Compose de produção executável e fiel à arquitetura: Caddy + aplicação monolítica + PostgreSQL, sem Redis ou serviços placeholders;
- documentação completa de arquitetura, recursos, API, integrações, requisitos, portas, operação e visão comercial.

### Compatibilidade

O módulo Go `github.com/knadh/listmonk`, prefixo env `LISTMONK_*`, schema core e, no Compose local, database/role legados foram mantidos para compatibilidade. Isso não autoriza publicar artefatos MailView com a marca upstream.

Upgrade:

```sh
./mailview --upgrade --yes --config config.toml
./mailview --config config.toml
```

Faça backup antes. A role runtime deve ser `NOSUPERUSER NOBYPASSRLS`. Novos imports e enrollments TOTP exigem chaves base64 de 32 bytes.

### Evidências de validação

A Sprint 2 registra passagem de `go test ./...`, suítes PostgreSQL com role administrativa e restrita, `npm run lint` e `npm run build`. A release repete build/test/lint/Compose após a mudança de nomenclatura; detalhes finais ficam no commit/tag.

Consulte [Issues conhecidos](ISSUES_CONHECIDOS.md), [Funcionalidades](docs/FUNCIONALIDADES.md) e [Arquitetura](docs/ARQUITETURA.md).

## MailView v0.3.0 — `mailview-v0.3.0` — 2026-08-07

Primeira tag do fork: Control Plane inicial, MFA, contexto tenant, isolamento inicial de contatos/listas e importação CSV tenant-scoped. Foi substituída por v0.4.0 para uso novo.
