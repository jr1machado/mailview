<a href="https://zerodha.tech"><img src="https://zerodha.tech/static/images/github-badge.svg" align="right" /></a>

# MailView

**MailView é um fork independente do [listmonk](https://listmonk.app)**, mantido em repositório próprio (`github.com/jr1machado/mailview`), em transformação para uma plataforma **SaaS multi-tenant** de e-mail marketing e mailing lists. Este README documenta exclusivamente o estado do **MailView** — arquitetura, portas, requisitos, funcionalidades entregues e visão comercial da versão atual.

> Este projeto reaproveita o motor de envio, o modelo de campanhas e o admin do listmonk (AGPLv3) como base de código, mas segue um roadmap, uma modelagem de dados (`mv_*`) e uma superfície de API (`/api/mailview/*`) próprios, documentados em [`INFO/`](INFO/). Não é o projeto oficial listmonk/listmonk e não deve ser confundido com ele em anúncios, imagens Docker ou pacotes publicados.

[![mailview-dashboard](https://github.com/user-attachments/assets/689b5fbb-dd25-4956-a36f-e3226a65f9c4)](https://listmonk.app)

---

## Sumário

- [O que é o MailView](#o-que-é-o-mailview)
- [Arquitetura](#arquitetura)
- [Requisitos de hardware e software](#requisitos-de-hardware-e-software)
- [Portas e comunicação](#portas-e-comunicação)
- [Crescimento horizontal (workers como nodes)](#crescimento-horizontal-workers-como-nodes)
- [Instalação](#instalação)
- [Funcionalidades e recursos implementados até esta release](#funcionalidades-e-recursos-implementados-até-esta-release)
- [Segurança e isolamento de dados](#segurança-e-isolamento-de-dados)
- [Visão comercial — MailView para C-Levels e executivos](#visão-comercial--mailview-para-c-levels-e-executivos)
- [Documentação relacionada](#documentação-relacionada)
- [Desenvolvimento](#desenvolvimento)
- [Licença](#licença)

---

## O que é o MailView

MailView pega o núcleo de envio de e-mail em massa do listmonk — campanhas, listas, assinantes, templates, bounces, analytics — e adiciona uma camada de **Control Plane multi-tenant** por cima: tenants, papéis (RBAC) por tenant, autenticação de dois fatores própria, trilha de auditoria append-only e isolamento de dados por `tenant_id` + Row-Level Security no PostgreSQL.

O código do produto MailView fica isolado do core herdado em `internal/mailview/` e `cmd/mailview*.go`, para que o fork continue sincronizável com melhorias do listmonk upstream sem conflitos estruturais (ver [`INFO/ADRs.md`](INFO/ADRs.md) e a convenção de extensão em [`INFO/Fase-0.md`](INFO/Fase-0.md)).

## Arquitetura

```
                         ┌─────────────────────────┐
        HTTPS 443/80     │   proxy (edge)           │
  Internet ─────────────▶│   TLS termination        │
                         └────────────┬─────────────┘
                                      │ rede interna "app"
                    ┌─────────────────┼─────────────────┐
                    ▼                                     ▼
          ┌───────────────────┐                 ┌───────────────────┐
          │  frontend (Vue)     │                 │  api (Go, :9000)   │
          │  build estático      │◀── mesma origem ─│  binário único:    │
          └───────────────────┘   hoje              │  admin UI + REST + │
                                                     │  campaign manager  │
                                                     │  + mailview/*      │
                                                     └─────────┬─────────┘
                                                               │ rede interna "data"
                                          ┌────────────────────┼────────────────────┐
                                          ▼                                          ▼
                               ┌───────────────────┐                     ┌───────────────────┐
                               │ postgres :5432      │                     │ redis (reservado)   │
                               │ dado central único   │                     │ ainda sem uso no    │
                               │ RLS por tenant        │                     │ código da aplicação │
                               └───────────────────┘                     └───────────────────┘
```

Componentes reais nesta release:

| Componente | O que é hoje | Onde está no código |
|---|---|---|
| **api** | Binário Go único que serve API REST, admin UI embarcada, envio de campanhas (`internal/manager`) e as extensões MailView (`/api/mailview/*`) | `cmd/main.go` |
| **frontend** | SPA Vue 3 + Buefy, compilada para estático e embarcada no binário via `stuffbin` | `frontend/` |
| **postgres** | Fonte única da verdade — schema do listmonk + tabelas `mv_*` do MailView, isoladas via migration runner próprio | `schema.sql`, `internal/mailview/migrations/` |
| **proxy** | Camada de borda (TLS, roteamento) na topologia de produção de referência | `deploy/compose.production.yml` |
| **redis** | Declarado na topologia de produção como reserva de capacidade (cache/sessão futura). **Nenhum código do MailView usa Redis hoje** — sessões usam o backend Postgres do listmonk | `deploy/compose.production.yml` |
| **worker** (imagem) | Placeholder de topologia da Fase 0 para uma futura imagem de worker dedicada. **Ainda não existe um binário/entrypoint de worker separado**: hoje o mesmo binário `api` processa campanhas, e escala-se rodando réplicas com `--passive` (ver seção de crescimento horizontal) | `deploy/compose.production.yml`, `cmd/init.go` |

## Requisitos de hardware e software

### Software

| Item | Versão usada nesta release | Observação |
|---|---|---|
| Go | 1.26.1 (`go.mod`, CI) | build do binário `api` |
| PostgreSQL | Validado em CI/testes de integração com `postgres:16-alpine`; compose de desenvolvimento usa `postgres:17-alpine` | requer extensão `pgcrypto` (`schema.sql`); recomenda-se PostgreSQL 14+ |
| Node.js + Yarn | LTS atual | build do frontend Vue (`frontend/`); não há build de frontend no pipeline de CI atual, ver [issues conhecidos](ISSUES_CONHECIDOS.md) |
| Docker + Docker Compose | qualquer versão com suporte a `docker compose` v2 | deploy de referência (dev e produção) |

### Hardware (recomendação operacional, não benchmark formal)

Não há benchmark de carga publicado para o MailView até esta release. Os números abaixo são um ponto de partida operacional para dimensionar um piloto e devem ser validados com teste de carga antes de produção com volume:

| Cenário | CPU | RAM | Disco |
|---|---|---|---|
| Piloto / staging (1 réplica `api` + Postgres no mesmo host) | 2 vCPU | 2 GB | 20 GB SSD (cresce com volume de assinantes/uploads) |
| Produção inicial (réplicas `api` separadas do Postgres gerenciado) | 1–2 vCPU por réplica `api` | 1–2 GB por réplica `api` | Postgres dimensionado à parte pelo volume de contatos/eventos |

## Portas e comunicação

| Porta | Protocolo | Serviço | Direção | Observação |
|---|---|---|---|---|
| `9000/tcp` | HTTP | `api` (admin UI + REST + `/api/mailview/*`) | entrada | `app.address` no `config.toml`; `EXPOSE 9000` no `Dockerfile` |
| `80/tcp`, `443/tcp` | HTTP/HTTPS | `proxy` (borda) | entrada pública | único ponto exposto à Internet na topologia de produção (`deploy/compose.production.yml`) |
| `5432/tcp` | PostgreSQL | `postgres` | interno (`api` → `postgres`) | rede Docker `data`, `internal: true`; nunca exposto à rede `edge` |
| `25/465/587/tcp` | SMTP | provedor de e-mail externo | saída (`api` → provedor SMTP) | configurado por mensageiro (`internal/messenger/email`), fora do MailView |

**Não existe hoje um canal de comunicação central ↔ worker distinto do acesso ao Postgres compartilhado.** O "worker" da topologia de produção é, nesta release, o mesmo binário `api`: todas as réplicas leem/escrevem no mesmo Postgres pela porta `5432`; a única diferenciação de papel entre réplicas é a flag `--passive` (ver abaixo), não um protocolo de fila ou RPC entre nodes.

## Crescimento horizontal (workers como nodes)

O modelo de escala real do MailView nesta release é **réplicas idênticas do binário `api` atrás do proxy**, todas apontando para o mesmo PostgreSQL:

1. Toda réplica serve a admin UI e a API REST na porta `9000`.
2. Apenas as réplicas iniciadas **sem** `--passive` processam a fila de campanhas (`internal/manager`, `ScanCampaigns`); réplicas iniciadas com `--passive` atendem tráfego HTTP mas nunca disparam envio de campanha (`cmd/init.go`, flag `passive`).
3. Adicionar capacidade de leitura/API é literalmente subir mais réplicas `api --passive` atrás do proxy — sem coordenação adicional, porque não há estado local: sessão, fila de import e fila de campanha vivem no Postgres.
4. Adicionar capacidade de envio de campanha hoje é uma decisão operacional manual (qual réplica roda sem `--passive`), não um scheduler automático de workers — isso é um limite conhecido, listado em [`ISSUES_CONHECIDOS.md`](ISSUES_CONHECIDOS.md).
5. A imagem `worker` dedicada e a fila assíncrona via Redis/mensageria fazem parte do roadmap de arquitetura (seção 5.6 da [Bíblia de Arquitetura](INFO/Biblia-Projeto.md)), mas **não estão implementadas nesta release** — documentamos aqui para não confundir a topologia de referência (`deploy/compose.production.yml`) com o que já roda em produção hoje.

Importação de contatos (nova nesta release) já segue um modelo mais próximo de job/worker dentro do próprio processo: cada upload de CSV cria um job em `mv_import_jobs`, processado em uma goroutine em lotes de 500 linhas por transação tenant-scoped (`internal/mailview/importjob`). Isso não depende de Redis nem de um binário de worker separado — é o primeiro componente do produto pensado como fila, ainda rodando dentro do próprio processo `api`.

## Instalação

### Docker (produção, topologia de referência)

```sh
cd deploy
cp .env.example .env            # aponte para imagens MailView publicadas pelo seu pipeline
mkdir -p secrets && chmod 700 secrets
# crie os arquivos de secrets (database_url, postgres_user, postgres_password, postgres_db)
docker compose --env-file .env -f compose.production.yml config
docker compose --env-file .env -f compose.production.yml up -d
```

Veja [`deploy/README.md`](deploy/README.md). **Não reutilize o `docker-compose.yml` da raiz em produção** — ele é o ambiente de desenvolvimento herdado do listmonk (Postgres local sem TLS, sem secrets por arquivo).

### Binário

```sh
./listmonk --new-config                          # gera config.toml — edite mailview.* antes de subir
./listmonk --install                              # schema base (idempotente com --idempotent --yes)
./listmonk --upgrade --yes --config config.toml   # aplica migrations do listmonk + do MailView
./listmonk                                         # sobe a API/admin em http://localhost:9000
```

Configure ao menos `mailview.mfa_encryption_key` e `mailview.import_signing_key` (bases64 de 32 bytes) antes de habilitar TOTP e importação de contatos — sem elas, essas duas funcionalidades ficam desabilitadas de propósito em vez de operar sem cifra/assinatura.

## Funcionalidades e recursos implementados até esta release

### Herdado do listmonk (core inalterado)
Campanhas, listas, assinantes, templates HTML/texto, bounces, tracking de abertura/clique, dashboard de analytics, importação via ZIP/CSV legada, webhooks de bounce, múltiplos mensageiros (SMTP/HTTP), i18n, admin UI Vue completa.

### Fase 0 — Fundação
- ADRs, threat model e convenção de extensão do fork versionados (`INFO/ADRs.md`, `INFO/Threat-Model.md`).
- Imagem de runtime não-root (usuário `mailview` uid/gid 10001), `EXPOSE 9000` explícito.
- Topologia de produção de referência com redes segregadas (`edge`/`app`/`data`), secrets por arquivo, sem `:latest`.
- Pipeline de CI: `go vet`, `go test`, build, scan de segredos (gitleaks) e vulnerabilidades Go (govulncheck).

### Fase 1 — Control Plane e identidade
- Executor de migrations MailView independente (`internal/mailview/migrations`), com ledger (`mv_schema_migrations`) e lock consultivo — nunca entra no `migList` do upstream.
- Tenants (`mv_tenants`), memberships (`mv_memberships`), papéis e permissões (`mv_roles`, `mv_permissions`, `mv_role_permissions`) e auditoria append-only (`mv_audit_events`).
- Criação de tenant cria automaticamente 7 papéis padrão em uma transação: **Tenant Owner, Tenant Admin, Campaign Manager, Operator, Analyst, Viewer, Billing Manager**, com permissões granulares (`campaign.*`, `subscriber.*`, `template.manage`, `domain.manage`, `user.*`, `audit.read`, `billing.*`).
- MFA TOTP (RFC 6238) com segredo cifrado em repouso com AES-256-GCM (`mailview.mfa_encryption_key`); 10 recovery codes de uso único, guardados como hash bcrypt.
- API administrativa protegida pelo Super Admin do listmonk (ponte temporária até o papel de plataforma dedicado existir):

  | Método | Rota | Função |
  |---|---|---|
  | `GET/POST` | `/api/mailview/tenants` | Lista / cria tenant + owner |
  | `GET/PATCH` | `/api/mailview/tenants/:tenantID` | Consulta / altera status do tenant |
  | `GET` | `/api/mailview/tenants/:tenantID/roles` | Lista papéis padrão do tenant |
  | `GET/POST` | `/api/mailview/tenants/:tenantID/memberships` | Lista / cria membership |
  | `PUT` | `/api/mailview/tenants/:tenantID/memberships/:membershipID/roles` | Substitui papéis de uma membership |
  | `GET` | `/api/mailview/tenants/:tenantID/audit-events` | Lê trilha de auditoria do tenant |
  | `POST` | `/api/mailview/profile/mfa/recovery-codes` | Gera 10 recovery codes para o usuário autenticado com TOTP |

### Fase 2 — Isolamento por tenant (em andamento)
- `tenant.Context` + `tenant.Begin`/`tenant.InTransaction`: todo acesso tenant-scoped roda dentro de uma transação que fixa `app.tenant_id`, `app.user_id` e `app.request_id` via `set_config(..., true)` — nunca via parâmetro editável pelo cliente.
- `mv_tenant_settings` sob `FORCE ROW LEVEL SECURITY`, com teste de integração cross-tenant.
- `subscribers`, `lists` e `subscriber_lists` já têm `tenant_id`, índices e FKs compostas; políticas RLS estão criadas mas **ainda não ativadas** nessas três tabelas — a ativação depende de migrar todas as rotas legadas globais (regra explícita em `INFO/pendencias.md`).
- Data Plane tenant-scoped para CRUD de contatos e listas em `/api/mailview/tenants/:tenantID/data/{lists,subscribers}`.
- Resolução opcional de tenant por subdomínio (`{slug}.{tenant_base_domain}`, `mailview.tenant_routing_enabled`), validando membership ativa antes de servir a requisição.
- **Importação tenant-scoped de contatos (novo nesta release)**:
  - `mv_import_jobs` / `mv_import_files`, ambas sob `FORCE ROW LEVEL SECURITY`, com idempotency key por tenant e assinatura HMAC do arquivo (`mailview.import_signing_key`).
  - Upload gravado em prefixo isolado por tenant (`import_storage_dir/<tenant_id>/<job_id>.csv`).
  - Worker CSV em lotes de 500 linhas por `tenant.InTransaction`, revalidando ownership das listas e a assinatura do arquivo antes de processar.
  - Endpoints: `POST/GET /api/mailview/tenants/:tenantID/data/import-jobs`, `GET .../import-jobs/:jobID`, `POST .../import-jobs/:jobID/cancel`.
  - Teste de integração cobrindo importação concorrente entre duas tenants, rejeição de lista cross-tenant e replay de idempotency key.

Veja o detalhamento completo, incluindo o que falta antes de ativar RLS nas tabelas legadas, em [`INFO/pendencias.md`](INFO/pendencias.md).

## Segurança e isolamento de dados

- **Isolamento**: banco compartilhado + `tenant_id` obrigatório + Row-Level Security é a estratégia padrão (ADR-001); planos Enterprise com banco/worker/SMTP dedicado ficam previstos para fases futuras (ADR-002).
- **Segredos**: nunca em Compose/imagem; Docker secrets por arquivo (`LISTMONK_*_FILE`), scan de segredos no CI (gitleaks) e `.dockerignore` endurecido.
- **MFA**: TOTP obrigatório para operações administrativas sensíveis, com recovery codes de uso único.
- **Auditoria**: toda mutação relevante do Control Plane grava em `mv_audit_events` (append-only).
- Matriz completa de ameaças e controles obrigatórios em [`INFO/Threat-Model.md`](INFO/Threat-Model.md).

## Visão comercial — MailView para C-Levels e executivos

**MailView é a base para transformar uma ferramenta de e-mail marketing self-hosted em um produto SaaS multi-tenant vendável**, sem abrir mão de dados dentro de infraestrutura própria — um diferencial direto contra ESPs (email service providers) de terceiros que exigem que a base de contatos e o histórico de envio saiam da empresa.

### Dores que o MailView resolve

- **Custo por contato/e-mail imprevisível.** Plataformas SaaS de e-mail marketing cobram por volume de contatos ou de disparos, e o custo escala junto com a base — muitas vezes mais rápido que a receita que ela gera. MailView roda em infraestrutura própria ou de um único provedor cloud, com custo dominado por CPU/armazenamento, não por número de contatos.
- **Dados de clientes fora do perímetro da empresa.** Enviar a base de contatos para um ESP terceirizado é, na prática, terceirizar um ativo de dados sensível (PII, comportamento de engajamento). MailView mantém o dado no banco da própria empresa (ou do cliente final, no modelo white-label), com RLS por tenant como barreira técnica, não só contratual.
- **Falta de controle de acesso granular por equipe/cliente.** Ferramentas genéricas de e-mail marketing raramente separam "quem pode aprovar o disparo" de "quem pode editar o template" de "quem só pode ver relatório". O RBAC do MailView já nasce com sete papéis padrão por tenant (Owner, Admin, Campaign Manager, Operator, Analyst, Viewer, Billing Manager), prontos para mapear a estrutura real de uma equipe de marketing/CS.
- **Auditoria inexistente em ferramentas internas caseiras.** Times que hoje usam scripts ou uma instalação única e compartilhada do listmonk não têm trilha de quem alterou o quê. O MailView grava toda ação administrativa relevante em uma trilha de auditoria append-only, pré-requisito comum de compliance (LGPD/SOC2) que hoje falta em soluções internas ad hoc.
- **Importação de base sem isolamento em ambientes multi-cliente.** Agências e plataformas que gerenciam e-mail marketing para múltiplos clientes num único banco compartilhado historicamente correm risco de vazamento de lista entre clientes durante import/export. O worker de importação desta release já isola upload, processamento e assinatura de arquivo por tenant desde o primeiro dia.

### Casos de uso relevantes

1. **Agência de marketing digital ou CS** que opera e-mail marketing para dezenas de clientes hoje em planilhas, instâncias isoladas ou ferramentas de terceiros por cliente — MailView consolida tudo em uma única plataforma com isolamento técnico por tenant, sem misturar bases nem faturas.
2. **SaaS vertical (ex.: sistema de gestão para clínicas, imobiliárias, e-commerce) que quer embutir e-mail marketing como feature paga** — MailView vira o motor white-label por trás de "Campanhas" dentro do produto do cliente, com RBAC e billing por tenant prontos para virar um add-on de receita recorrente.
3. **Empresa média/grande com política de dados restritiva (financeiro, saúde, jurídico)** que precisa de e-mail marketing mas não pode enviar a base de contatos para um ESP terceirizado — MailView roda dentro do perímetro de rede da empresa, com auditoria e MFA nativos para atender requisitos de segurança interna.
4. **Operação de growth/CRM com múltiplas marcas ou unidades de negócio** que hoje mantém contas separadas em uma ferramenta paga por assento — o Control Plane do MailView consolida a operação técnica em um único cluster, com contas (tenants) e billing separados por unidade.

### Por que investir agora

O core de envio (campanhas, templates, bounce handling, analytics) já é maduro — é herdado de um produto open source usado em produção por milhares de instalações. O investimento incremental do MailView está concentrado exatamente onde está o risco e o diferencial competitivo de virar SaaS: **isolamento multi-tenant, identidade, RBAC e auditoria** — a parte que normalmente consome mais tempo de engenharia e mais rodadas de due diligence de segurança antes de uma venda B2B. Essa camada já está em construção com RLS, testes cross-tenant automatizados e uma matriz de ameaças formal desde a Fase 0, reduzindo o risco de retrabalho de segurança mais tarde no roadmap.

## Documentação relacionada

- [`RELEASE_NOTES.md`](RELEASE_NOTES.md) — notas desta versão.
- [`ISSUES_CONHECIDOS.md`](ISSUES_CONHECIDOS.md) — limitações e problemas conhecidos desta versão.
- [`INFO/Biblia-Projeto.md`](INFO/Biblia-Projeto.md) — arquitetura-alvo completa do produto SaaS (visão de longo prazo, nem tudo implementado ainda).
- [`INFO/ADRs.md`](INFO/ADRs.md) — decisões de arquitetura aceitas.
- [`INFO/Threat-Model.md`](INFO/Threat-Model.md) — matriz de ameaças e invariantes de segurança.
- [`INFO/Fase-0.md`](INFO/Fase-0.md), [`INFO/Fase-1.md`](INFO/Fase-1.md), [`INFO/Fase-2.md`](INFO/Fase-2.md) — o que cada fase entregou.
- [`INFO/pendencias.md`](INFO/pendencias.md) — estado de entrega e pendências, atualizado a cada incremento.
- [`deploy/README.md`](deploy/README.md) — deploy de produção de referência.

## Desenvolvimento

MailView é um fork do listmonk e herda sua licença AGPLv3. O backend é Go; o frontend é Vue 3 com Buefy. Para o fluxo de desenvolvimento herdado do listmonk (ambiente local, hot reload, testes), veja o [developer setup do listmonk](https://listmonk.app/docs/developer-setup) — ele continua válido para este fork, exceto pelas extensões descritas acima, que vivem em `internal/mailview/` e `cmd/mailview*.go`.

## Licença

MailView é licenciado sob AGPL v3, herdado do projeto [listmonk](https://listmonk.app) (© Zerodha Technology e contribuidores). Este fork acrescenta código próprio sob a mesma licença.
