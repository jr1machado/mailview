# MailView

Plataforma self-hosted de campanhas, listas e comunicação por e-mail com isolamento multi-tenant, RBAC e operação de plataforma.

> **Fork independente.** MailView nasceu do código aberto do [listmonk](https://github.com/knadh/listmonk), sob AGPL-3.0, mas possui mantenedor, roadmap, releases, arquitetura SaaS e identidade próprios. Este repositório não é afiliado nem representa o projeto upstream. O caminho do módulo Go `github.com/knadh/listmonk` e o prefixo de configuração `LISTMONK_*` foram preservados exclusivamente para compatibilidade técnica; binários, pacotes, imagens, tags e releases deste fork usam **MailView**.

Release atual: **MailView v0.4.0** (`v0.4.0`) — 9 de agosto de 2026.

## Visão comercial

MailView transforma um motor maduro de campanhas em uma base de produto multiempresa operável por provedores SaaS, agências, grupos empresariais e times que precisam manter seus dados sob controle. Para C-Levels, a proposta combina quatro resultados:

- **redução de dependência e custo variável por contato:** a organização controla infraestrutura, armazenamento e provedor de entrega;
- **governança de dados:** contatos, campanhas, mídia, métricas e bounces são separados por tenant no PostgreSQL com RLS;
- **delegação segura:** papéis de tenant e de plataforma separam marketing, operação, suporte, segurança, auditoria e billing;
- **base para monetização:** planos, quotas, consumo, domínios e infraestrutura dedicada já possuem modelo administrativo, sem afirmar que cobrança ou provisionamento automático estejam prontos.

Casos de uso relevantes:

1. agência que opera campanhas de vários clientes sem misturar bases;
2. SaaS vertical que oferece comunicação white-label dentro do próprio produto;
3. grupo empresarial com unidades independentes e governança central;
4. empresa regulada que quer manter PII e histórico em infraestrutura própria;
5. equipe de suporte que precisa investigar um tenant com acesso temporário, MFA e auditoria.

As dores resolvidas incluem contas compartilhadas, falta de rastreabilidade, risco de consulta cross-tenant, importações sem isolamento, custos imprevisíveis de ESPs e ausência de separação entre administração da plataforma e operação do cliente.

## O que está implementado

O core herdado oferece campanhas regulares e transacionais, listas e segmentação, contatos e atributos, templates HTML/texto, editor visual, mídia, importação, analytics de abertura/clique, bounces, páginas públicas, múltiplos idiomas, SMTP e mensageiros HTTP.

O MailView acrescenta nesta release:

- tenants, memberships, status, owner e auditoria append-only;
- 7 papéis padrão de tenant, 6 papéis de plataforma, permissões granulares, papéis customizados e negação explícita;
- TOTP com segredo AES-256-GCM e recovery codes bcrypt de uso único;
- contexto de tenant por transação e `ENABLE` + `FORCE ROW LEVEL SECURITY` em todos os agregados tenant-scoped;
- isolamento de contatos, listas, campanhas, templates, mídia, links, tracking, bounces e relacionamentos;
- roteamento público por subdomínio e domínio personalizado verificado; tenant suspenso é bloqueado;
- importação CSV assíncrona, idempotente, assinada por HMAC, com progresso e cancelamento;
- storage local ou S3 com prefixo de tenant e proteção contra traversal;
- portal de administração da plataforma para tenants, memberships, domínios, planos/quotas, owner, infraestrutura, RBAC e impersonação;
- impersonação de suporte limitada a 30 minutos, com justificativa, TOTP recente, revogação e auditoria;
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
mailview — binário Go único
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

Veja [Arquitetura](docs/ARQUITETURA.md), [Integrações](docs/INTEGRACOES.md) e [Referência operacional](docs/REFERENCIA_OPERACIONAL.md).

## Portas

| Porta | Protocolo | Uso | Exposição recomendada |
|---|---|---|---|
| `80/tcp` | HTTP | redirect/ACME no proxy | pública |
| `443/tcp` | HTTPS | painel, API, páginas, tracking e webhooks | pública |
| `9000/tcp` | HTTP | processo `mailview` | somente proxy/rede privada |
| `5432/tcp` | PostgreSQL | aplicação → banco | somente rede de dados |
| `25`, `465`, `587/tcp` | SMTP/SMTPS/Submission | aplicação → relay externo | somente saída, conforme provedor |
| `443/tcp` | HTTPS | S3, OIDC e APIs externas | somente saída |

O Compose local publica `10443:9000` e, apenas em loopback, `15432:5432`. Portas de desenvolvimento adicionais: MailHog `1025/8025`, Adminer `8070` e Vite `8080`.

## Requisitos

Software de build: Go 1.26.5, Node.js 22, Yarn 1.22 e Docker/Compose v2. Runtime: Linux containerizado ou SO suportado pelo Go, PostgreSQL 17 recomendado e acesso a um relay SMTP ou mensageiro HTTP.

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
make build          # ./mailview
make dist           # ./mailview com SPA/SQL/i18n embarcados
./mailview --new-config
./mailview --install --idempotent --yes --config config.toml
./mailview --upgrade --yes --config config.toml
./mailview --config config.toml
```

Convenção desta release:

- binário: `mailview` (`mailview.exe` no Windows);
- tag Git SemVer: `v0.4.0`;
- arquivos: `MailView_0.4.0_<sistema>_<arquitetura>.tar.gz`;
- imagem: `ghcr.io/jr1machado/mailview:v0.4.0`;
- título da release: `MailView v0.4.0`.

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
- [Hardware, software, portas e operação](docs/REFERENCIA_OPERACIONAL.md)
- [Release notes v0.4.0](RELEASE_NOTES.md)
- [Issues conhecidos v0.4.0](ISSUES_CONHECIDOS.md)
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
