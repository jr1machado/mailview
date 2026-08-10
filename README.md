# MailView

Plataforma self-hosted de campanhas, listas e comunicação por e-mail com isolamento multi-tenant, RBAC e operação de plataforma.

> **Fork independente.** MailView nasceu do código aberto do [listmonk](https://github.com/knadh/listmonk), sob AGPL-3.0, mas possui mantenedor, roadmap, releases, arquitetura SaaS e identidade próprios. Este repositório não é afiliado nem representa o projeto upstream. O caminho do módulo Go `github.com/knadh/listmonk` e o prefixo de configuração `LISTMONK_*` foram preservados exclusivamente para compatibilidade técnica; binários, pacotes, imagens, tags e releases deste fork usam **MailView**.

Release atual: **MailView v0.6.0** (`v0.6.0`) — 10 de agosto de 2026.

## Visão comercial

MailView transforma um motor maduro de campanhas em uma base de produto multiempresa operável por provedores SaaS, agências, grupos empresariais e times que precisam manter seus dados sob controle. Para C-Levels, a proposta combina quatro resultados:

- **redução de dependência e custo variável por contato:** a organização controla infraestrutura, armazenamento e provedor de entrega;
- **governança de dados:** contatos, campanhas, mídia, métricas e bounces são separados por tenant no PostgreSQL com RLS;
- **delegação segura:** papéis de tenant e de plataforma separam marketing, operação, suporte, segurança, auditoria e billing;
- **base para monetização:** planos, quotas, consumo, billing/subscriptions/invoices e roteamento dedicado possuem modelo tenant-aware, sem afirmar que gateway ou provisionamento físico estejam prontos.

Casos de uso relevantes:

1. agência que opera campanhas de vários clientes sem misturar bases;
2. SaaS vertical que oferece comunicação white-label dentro do próprio produto;
3. grupo empresarial com unidades independentes e governança central;
4. empresa regulada que quer manter PII e histórico em infraestrutura própria;
5. equipe de suporte que precisa investigar um tenant com acesso temporário, MFA e auditoria.

As dores resolvidas incluem contas compartilhadas, falta de rastreabilidade, risco de consulta cross-tenant, importações sem isolamento, custos imprevisíveis de ESPs e ausência de separação entre administração da plataforma e operação do cliente.

| Objetivo executivo | Como o MailView contribui |
|---|---|
| consolidar comunicação de várias marcas | um Control Plane governa tenants isolados, domínios e papéis |
| reduzir risco operacional e regulatório | RLS, auditoria, MFA, consentimento, supressão e acesso temporário |
| controlar unit economics | infraestrutura e relay próprios, catálogo de planos, quotas e consumo |
| oferecer comunicação como produto | APIs, branding, domínios e roteamento dedicado formam uma base white-label |
| acelerar resposta a incidentes | dashboard global, incidentes, logs correlacionados e impersonation auditada |

O público-alvo primário é composto por SaaS B2B, agências, holdings, equipes de
plataforma e organizações reguladas com capacidade de operar PostgreSQL,
entregabilidade e infraestrutura. MailView v0.6.0 é uma base self-hosted; não
substitui, por si só, um gateway de cobrança, um serviço gerenciado de
entregabilidade ou uma plataforma HA pronta.

## Escopo da release e fronteiras

Esta release cobre o produto executável, sua interface administrativa e de
tenant, APIs, migrations, segurança multi-tenant, build, containers, referência
de deploy e documentação operacional. O suporte oficial desta release parte da
topologia Caddy + MailView + PostgreSQL descrita neste repositório.

Estão dentro do escopo: administração de tenants, RBAC, campanhas e contatos,
importação, conteúdo, envio via SMTP/messenger, tracking, domínios, auditoria,
MFA, suporte controlado, catálogo de planos e contrato de infraestrutura
dedicada. São integrações externas necessárias ou opcionais: relay SMTP,
DNS/ACME, storage S3, OIDC, POP3 de bounces, captcha e postbacks HTTP.

Não estão incluídos como serviço pronto nesta versão: gateway financeiro,
provisionamento automático de recursos dedicados, HA do banco, fila distribuída,
autoscaling, observabilidade Prometheus/OpenTelemetry nativa ou operação
gerenciada. Modelos persistentes sem dispatcher/enforcement são identificados
explicitamente na matriz funcional e em `ISSUES_CONHECIDOS.md`.

## O que está implementado

O core herdado oferece campanhas regulares e transacionais, listas e segmentação, contatos e atributos, templates HTML/texto, editor visual, mídia, importação, analytics de abertura/clique, bounces, páginas públicas, múltiplos idiomas, SMTP e mensageiros HTTP.

O MailView acrescenta nesta release:

- tenants, memberships, status, owner e auditoria append-only;
- 7 papéis padrão de tenant, 6 papéis de plataforma, permissões granulares, papéis customizados e negação explícita;
- TOTP com segredo AES-256-GCM e recovery codes bcrypt de uso único;
- contexto de tenant por transação e `ENABLE` + `FORCE ROW LEVEL SECURITY` em todos os agregados tenant-scoped;
- isolamento de contatos, listas, campanhas, templates, mídia, links, tracking, bounces e relacionamentos;
- roteamento público por subdomínio e domínio personalizado verificado em DNS; troca de slug gera redirect 308 temporário e tenant suspenso é bloqueado;
- importação CSV assíncrona, idempotente, com arquivo e envelope tenant/job assinados por HMAC, progresso e cancelamento;
- storage local ou S3 com prefixo de tenant e proteção contra traversal;
- portal de administração da plataforma para tenants, memberships, slugs, DNS/domínios, planos/quotas, owner, roteamento Enterprise, RBAC e impersonação;
- mapa visual de ambientes que distribui cada cliente entre infraestrutura compartilhada, em provisionamento ou dedicada, exibindo a fronteira e os recursos de banco, fila, SMTP e storage;
- faixa persistente no portal do cliente com hostname e identificador abreviado do tenant ativo, reduzindo erro operacional entre ambientes;
- revalidação DNS periódica com retirada automática do host e revogação do estado TLS quando a propriedade é perdida;
- catálogo Fase 3 para branding, sessões/API keys, billing, SMTP/senders, DNS, complaints/events, exports, webhooks e transacionais, protegido por RLS;
- workflow Fase 4 de campanhas com revisão, aprovação, rejeição, agendamento e cancelamento idempotentes;
- home tenant e dashboard global com consumo, billing, filas, bounces, complaints e falhas de webhooks;
- API keys tenant com escopo não elevável, token de exibição única e persistência somente do hash;
- contatos com tags, consentimento e supressão, mantendo atributos customizados e deduplicação tenant-first;
- impersonação de suporte limitada a 30 minutos, com justificativa, TOTP recente, aprovação independente opcional, banner, revogação e auditoria;
- planos Starter, Growth e Enterprise e registro de uso; enforcement de quota e billing automático ainda não existem;
- imagem não-root, Compose de produção executável, CI de backend/frontend, testes de RLS e build de release nomeado MailView.

A matriz detalhada, incluindo limites, está em [Funcionalidades](docs/FUNCIONALIDADES.md). As rotas estão em [API MailView](docs/API_MAILVIEW.md) e as limitações em [Issues conhecidos](ISSUES_CONHECIDOS.md).

## Arquitetura resumida

```text
Internet
  │ HTTP/HTTPS 80/443
  ▼
Caddy / proxy TLS
  │ HTTP 9000 (rede interna)
  ▼
MailView — binário Go único
  ├─ API REST e páginas públicas
  ├─ SPA Vue 2.7 embarcada
  ├─ manager de campanhas e mensagens transacionais
  ├─ workers internos de importação e bounce
  └─ Control Plane + Data Plane tenant-scoped
  │ PostgreSQL 5432 (rede interna)
  ▼
PostgreSQL 17 — schema core + mv_* + RLS

Saídas opcionais: SMTP 25/465/587, S3 HTTPS 443, OIDC HTTPS 443 e postbacks HTTP(S).
Entradas opcionais: webhooks de bounce e fluxos públicos, sempre pelo proxy.
```

O frontend não é um serviço separado e não existe binário de worker independente nesta release. Uma instância ativa executa HTTP e jobs; réplicas com `--passive` atendem HTTP sem iniciar o scanner de campanhas. Redis não integra a arquitetura atual.

### Organização do código

| Caminho | Conteúdo |
|---|---|
| `cmd/` | processo HTTP, CLI, handlers do core e handlers MailView |
| `internal/mailview/` | Control Plane, Data Plane, tenant context/RLS, migrations, jobs e segurança próprias |
| `internal/core/`, `internal/manager/` | campanhas, listas, templates, usuários, entrega e workers herdados e adaptados |
| `models/`, `queries/`, `schema.sql` | modelos e SQL do core compatível |
| `frontend/src/` | SPA Vue, portal do cliente e administração da plataforma |
| `static/`, `i18n/` | páginas públicas, templates e idiomas embarcados |
| `deploy/` | Caddy, Compose de produção, role restrita e secrets |
| `docs/` | arquitetura, API, ferramentas, integrações e operação da release |

### Fluxo entre componentes

```text
Cliente/operador
  └─ HTTPS :443 → Caddy
       └─ HTTP :9000 → MailView
            ├─ SQL :5432 → PostgreSQL (role sem BYPASSRLS)
            ├─ SMTP :25/:465/:587 → relay de entrega
            ├─ POP3 :110/:995 → mailbox de bounces (opcional)
            ├─ HTTPS :443 → S3/OIDC/captcha/postbacks (opcional)
            └─ DNS :53 → verificação/revalidação de domínio
```

Veja [Arquitetura](docs/ARQUITETURA.md), [Integrações](docs/INTEGRACOES.md) e [Referência operacional](docs/REFERENCIA_OPERACIONAL.md).

## Portas

| Porta | Protocolo | Uso | Exposição recomendada |
|---|---|---|---|
| `80/tcp` | HTTP | redirect/ACME no proxy | pública |
| `443/tcp` | HTTPS | painel, API, páginas, tracking e webhooks | pública |
| `9000/tcp` | HTTP | processo `MailView` | somente proxy/rede privada |
| `5432/tcp` | PostgreSQL | aplicação → banco | somente rede de dados |
| `25`, `465`, `587/tcp` | SMTP/SMTPS/Submission | aplicação → relay externo | somente saída, conforme provedor |
| `110`, `995/tcp` | POP3/POP3S | aplicação → mailbox de bounces | somente saída, opcional |
| `53/udp`, `53/tcp` | DNS | verificação e revalidação de domínios | somente saída |
| `80`, `443/tcp` | HTTP(S) externo | ACME, S3, OIDC, captcha e APIs | somente saída |

O Compose local publica `10443:9000` e, apenas em loopback, `15432:5432`. Portas de desenvolvimento adicionais: MailHog `1025/8025`, Adminer `8070` e Vite `8080`.

## Requisitos

Software de build: Go 1.26.5, Node.js 22, Yarn 1.22, Vite 5/6, Vue 2.7,
GoReleaser 2 e Docker/Compose v2. Runtime: Linux containerizado ou SO suportado
pelo Go, PostgreSQL 17 recomendado, Caddy 2.10 na topologia de referência e
acesso a um relay SMTP ou mensageiro HTTP. O navegador deve suportar JavaScript
moderno, cookies/sessão e TLS 1.2 ou superior.

Referência inicial de capacidade — não é benchmark nem SLA:

| Perfil | Aplicação | PostgreSQL | Disco |
|---|---|---|---|
| desenvolvimento/piloto | 2 vCPU, 2–4 GiB RAM | compartilhado no host | 20 GiB SSD |
| produção inicial | 2–4 vCPU, 4–8 GiB RAM | 4 vCPU, 8 GiB RAM | 50+ GiB SSD |
| alto volume | réplicas HTTP `--passive` e nó ativo dimensionado por throughput | serviço dedicado, IOPS e retenção medidos | sizing por contatos, mídia e eventos |

Faça teste de carga com o template, tamanho de lista, concorrência e relay reais. A vazão costuma ser limitada pelo SMTP e pelo banco, não apenas pela CPU.

## Instalação com Docker Compose

Para avaliação local:

```sh
export MAILVIEW_MFA_ENCRYPTION_KEY="$(openssl rand -base64 32)"
export MAILVIEW_IMPORT_SIGNING_KEY="$(openssl rand -base64 32)"
docker compose up -d --build
```

Acesse `http://localhost:10443`. O banco local fica em `127.0.0.1:15432`. O Compose conserva os nomes de database/role `listmonk` e `listmonk_app` por compatibilidade com volumes existentes; isso não altera o nome do produto ou dos artefatos.

Para produção:

```sh
cd deploy
cp .env.example .env
mkdir -p secrets && chmod 700 secrets
printf '%s' 'mailview_admin' > secrets/postgres-user
openssl rand -base64 36 > secrets/postgres-password
openssl rand -base64 36 > secrets/app-db-password
openssl rand -base64 32 > secrets/mfa-encryption-key
openssl rand -base64 32 > secrets/import-signing-key
chmod 600 secrets/*
docker compose --env-file .env -f compose.production.yml config
docker compose --env-file .env -f compose.production.yml up -d
```

Defina `MAILVIEW_IMAGE` com tag imutável e configure DNS para `MAILVIEW_PUBLIC_HOST`. Leia [Deploy de produção](deploy/README.md) antes de expor o serviço.

## Binário e release

```sh
make build          # ./MailView
make dist           # ./MailView com SPA/SQL/i18n embarcados
./MailView --new-config
./MailView --install --idempotent --yes --config config.toml
./MailView --upgrade --yes --config config.toml
./MailView --config config.toml
```

Convenção desta release:

- binário: `MailView` (`MailView.exe` no Windows);
- tag Git SemVer: `v0.6.0`;
- arquivos: `MailView_0.6.0_<sistema>_<arquitetura>.tar.gz`;
- checksum: `MailView_0.6.0_checksums.txt`;
- imagem: `ghcr.io/jr1machado/mailview:v0.6.0` (OCI exige nome minúsculo);
- pacote interno do editor: `@mailview/email-builder` (npm exige minúsculas);
- título da release: `MailView v0.6.0`.

## Configuração e compatibilidade

As chaves de bootstrap ficam em `config.toml`; configurações funcionais são persistidas no PostgreSQL e editadas no painel. Por compatibilidade com o loader herdado, variáveis seguem `LISTMONK_secao__chave`, inclusive `_FILE` para secrets. Exemplos:

```text
LISTMONK_app__address=0.0.0.0:9000
LISTMONK_db__host=postgres
LISTMONK_db__password_FILE=/run/secrets/app_db_password
LISTMONK_mailview__mfa_encryption_key_FILE=/run/secrets/mfa_encryption_key
LISTMONK_mailview__import_signing_key_FILE=/run/secrets/import_signing_key
```

O módulo Go também conserva `github.com/knadh/listmonk` para evitar uma reescrita incompatível de centenas de imports. Novas extensões ficam em `internal/mailview`, tabelas usam prefixo `mv_` e APIs próprias usam `/api/mailview`.

## Documentação

- [Arquitetura completa](docs/ARQUITETURA.md)
- [Funções e recursos](docs/FUNCIONALIDADES.md)
- [API e permissões](docs/API_MAILVIEW.md)
- [Integrações](docs/INTEGRACOES.md)
- [Ferramentas e cadeia de entrega](docs/FERRAMENTAS.md)
- [Hardware, software, portas e operação](docs/REFERENCIA_OPERACIONAL.md)
- [Release notes v0.6.0](RELEASE_NOTES.md)
- [Issues conhecidos v0.6.0](ISSUES_CONHECIDOS.md)
- [Roadmap e decisões](INFO/Biblia-Projeto.md)
- [Licença AGPL-3.0](LICENSE)

## Desenvolvimento e validação

```sh
go test ./...
make build
cd frontend && yarn lint && yarn build
docker compose config --quiet
docker compose --env-file deploy/.env.example -f deploy/compose.production.yml config --quiet
```

Testes PostgreSQL de isolamento usam `MAILVIEW_TEST_DSN`; consulte [Referência operacional](docs/REFERENCIA_OPERACIONAL.md). Contribuições devem preservar a fronteira do fork, o contexto transacional e os testes cross-tenant.

## Licença

MailView é distribuído sob [GNU Affero General Public License v3.0](LICENSE). As atribuições do código derivado permanecem preservadas.
