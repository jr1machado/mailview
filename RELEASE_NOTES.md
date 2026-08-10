# MailView — Release notes

## MailView v0.6.0 — `v0.6.0` — 2026-08-10

Release de clareza operacional e consolidação documental. A v0.6.0 torna a
separação de ambientes por cliente visível no produto e publica uma descrição
verificável do escopo, arquitetura, integrações, requisitos e limites atuais do
fork independente MailView.

### Ambientes e experiência multi-tenant

- novo mapa visual no Control Plane distribui tenants em três faixas:
  compartilhado, em provisionamento (`dedicated_requested`) e dedicado;
- cada cliente possui cartão identificável por nome, slug, status e UUID
  abreviado, com representação de banco, fila, SMTP e storage;
- o mapa consulta o contrato real de `mv_tenant_infrastructure`, em vez de
  inferir isolamento pelo plano comercial;
- alterações de infraestrutura refletem imediatamente na distribuição visual;
- clique no cartão abre o detalhe operacional existente de domínio, quota,
  owner, slug e referências de infraestrutura;
- portal tenant passa a exibir uma faixa persistente com hostname, limite de
  tenant e identificador abreviado, reduzindo risco de operar no cliente errado;
- layout responsivo e estados vazios preservam leitura em desktop e mobile.

### Documentação, produto e operação

- README consolida declaração de fork independente, visão comercial para
  C-Levels, escopo e exclusões, arquitetura, organização do código, fluxos de
  comunicação, hardware, software, portas, build, instalação e nomenclatura;
- documentação técnica atualizada para v0.6.0 cobre Control/Data Plane, RLS,
  RBAC, funções, APIs, ferramentas, integrações, topologia e operação;
- matriz funcional diferencia recurso executável, modelo persistente,
  integração externa e item ainda pendente, evitando promessa comercial
  superior ao produto entregue;
- `ISSUES_CONHECIDOS.md` registra limites de escala, multi-tenancy, billing,
  entregabilidade, segurança, frontend e plataformas suportadas;
- metadados de versão sincronizados em `VERSION`, frontend, Docker e Compose.

### Identidade dos artefatos

- executável: `MailView` ou `MailView.exe`;
- pacote: `MailView_0.6.0_<os>_<arch>.tar.gz`;
- checksum: `MailView_0.6.0_checksums.txt`;
- release: `MailView v0.6.0`;
- imagem OCI: `ghcr.io/jr1machado/mailview:v0.6.0` — minúsculas são uma
  restrição do registry, não uma mudança da marca;
- tag Git: `v0.6.0`.

### Compatibilidade e upgrade

Não há migration de banco exclusiva desta release. O módulo Go, schema core e
prefixo `LISTMONK_*` continuam preservados por compatibilidade técnica; novos
artefatos permanecem identificados como MailView.

```sh
./MailView --upgrade --yes --config config.toml
./MailView --config config.toml
```

Faça backup de PostgreSQL, mídia, imports, secrets e dados ACME antes de trocar
a imagem ou o binário. Consulte [Issues conhecidos](ISSUES_CONHECIDOS.md).

## MailView v0.5.0 — `v0.5.0` — 2026-08-10

Release que entrega as Fases 3 e 4: completa o catálogo tenant, fecha o
workflow de campanhas e acrescenta governança operacional e de segurança aos
portais MailView.

### RBAC, campanhas e portal do cliente

- sete papéis tenant e seis papéis de plataforma com catálogo granular;
- grants aditivos, denial explícito prevalente e custom roles em Growth/Enterprise;
- invalidação de sessões após alterações sensíveis de papel/ownership;
- workflow auditável e idempotente `draft → review → approved → scheduled → sending → completed`, com rejeição e cancelamento;
- locks de linha e idempotency key impedem dupla transição/agendamento em retries;
- home tenant com volume, campanhas ativas, contatos, bounces, plano, domínios e alertas;
- contatos com tags, consentimento, fonte/data de consentimento e supressão controlada.

### Portal administrativo e suporte

- dashboard global com tenants, MRR/ARR observado em invoices pagas, e-mails, filas, bounces, complaints, webhooks, domínios, incidentes e infraestrutura dedicada;
- gestão de tenant, suspensão/reativação/offboarding, quota/plano, owner, auditoria, slug e roteamento Enterprise;
- incidentes de plataforma com severidade, vínculo opcional ao tenant, resolução e auditoria;
- impersonation com razão obrigatória, TOTP recente, TTL máximo de 30 minutos, aprovação opcional por segundo operador, banner e revogação;
- grants de impersonation jamais ampliam billing, MFA, segredos ou permissões de plataforma.

### Dados, domínios e jobs

- 17 entidades nativas da Fase 3 com `FORCE RLS`, índices tenant-first e FKs compostas;
- API keys com escopo tenant não elevável, token de exibição única e somente SHA-256 persistido;
- catálogo de versões de chave e suporte a ciphertext/referência externa para segredos SMTP;
- envelopes HMAC de jobs com tenant, job, tipo, TTL e hash do payload;
- slug reservado, troca auditada, histórico e redirect HTTP 308 temporário;
- desafio DNS TXT/CNAME, verificação real, revalidação automática, retirada de rota e revogação TLS;
- contrato Enterprise versionado para database, worker, SMTP, storage, KMS e namespace, sem persistir credenciais.

### Segurança, build e operação

- TLS 1.2/1.3, HSTS, `nosniff` e Referrer-Policy no Caddy;
- role PostgreSQL runtime `NOSUPERUSER NOBYPASSRLS`, container UID 10001 e secrets por arquivo;
- executável `MailView`/`MailView.exe`, archives e checksums `MailView_*`;
- imagem OCI `ghcr.io/jr1machado/mailview:v0.5.0` multi-arquitetura;
- documentação de arquitetura, ferramentas, funções, integrações, hardware, software e portas atualizada.

### Upgrade

```sh
./MailView --upgrade --yes --config config.toml
./MailView --config config.toml
```

Faça backup de PostgreSQL, mídia, imports, secrets e dados ACME antes do
upgrade. Consulte [Issues conhecidos](ISSUES_CONHECIDOS.md) antes de produção.

## MailView v0.4.0 — `v0.4.0` — 2026-08-09

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

- binário renomeado para `MailView` e frontend package `mailview` 0.4.0;
- arquivos GoReleaser `MailView_0.4.0_<os>_<arch>.tar.gz` e imagens `ghcr.io/jr1machado/mailview`;
- workflow acionado por tag SemVer `v*` e título/artefatos identificados como MailView;
- Go 1.26.5 e dependências `goldmark`, `x/text` e `x/image` atualizadas para corrigir os achados alcançáveis do `govulncheck`;
- imagem não-root em `/mailview` e Compose local com container/rede MailView;
- Compose de produção executável e fiel à arquitetura: Caddy + aplicação monolítica + PostgreSQL, sem Redis ou serviços placeholders;
- documentação completa de arquitetura, recursos, API, integrações, requisitos, portas, operação e visão comercial.

### Compatibilidade

O módulo Go `github.com/knadh/listmonk`, prefixo env `LISTMONK_*`, schema core e, no Compose local, database/role legados foram mantidos para compatibilidade. Isso não autoriza publicar artefatos MailView com a marca upstream.

Upgrade:

```sh
./MailView --upgrade --yes --config config.toml
./MailView --config config.toml
```

Faça backup antes. A role runtime deve ser `NOSUPERUSER NOBYPASSRLS`. Novos imports e enrollments TOTP exigem chaves base64 de 32 bytes.

### Evidências de validação

A Sprint 2 registra passagem de `go test ./...`, suítes PostgreSQL com role administrativa e restrita, `npm run lint` e `npm run build`. A release repete build/test/lint/Compose após a mudança de nomenclatura; detalhes finais ficam no commit/tag.

Consulte [Issues conhecidos](ISSUES_CONHECIDOS.md), [Funcionalidades](docs/FUNCIONALIDADES.md) e [Arquitetura](docs/ARQUITETURA.md).

## MailView v0.3.0 — `mailview-v0.3.0` — 2026-08-07

Primeira tag do fork: Control Plane inicial, MFA, contexto tenant, isolamento inicial de contatos/listas e importação CSV tenant-scoped. Foi substituída por v0.4.0 para uso novo.
