# Ferramentas e cadeia de entrega do MailView v0.5.0

Este documento registra as ferramentas realmente usadas para desenvolver,
validar, empacotar, publicar e operar o MailView. Versões são as declaradas na
release; substituições devem passar pela mesma matriz de testes.

## Desenvolvimento e build

| Ferramenta | Versão/base | Escopo |
|---|---|---|
| Go | 1.26.5 | API, Control Plane, Data Plane, workers internos e binário `MailView` |
| Node.js | 22 | build da SPA e do editor de e-mail |
| Yarn | 1.22.22 | dependências, lint e build frontend |
| Vue | 2.7.14 | painel administrativo e portal tenant |
| Vite | 5.4 | bundling da SPA |
| PostgreSQL | 17 recomendado | persistência, constraints, transações e RLS |
| `stuffbin` | dependência Go do projeto | incorpora frontend, SQL, templates e i18n no executável |
| GNU Make | compatível com o Makefile | comandos reproduzíveis de build/test/release |

Comandos principais:

```sh
make build                 # gera ./MailView
make dist                  # gera ./MailView com assets embarcados
go test ./...
go vet ./...
yarn --cwd frontend lint
yarn --cwd frontend build
```

## Qualidade e segurança

| Ferramenta | Uso |
|---|---|
| Go test | testes unitários e integrações opt-in com PostgreSQL |
| ESLint | regras JavaScript/Vue |
| Vite build | verificação de compilação e assets frontend |
| govulncheck | vulnerabilidades alcançáveis nas dependências Go |
| Gitleaks | detecção de secrets no histórico e workspace |
| Docker Compose config | validação das topologias local e de produção |
| PostgreSQL `NOBYPASSRLS` | prova de que a aplicação não contorna as policies tenant |

Testes integrados usam `MAILVIEW_TEST_DSN` e devem apontar exclusivamente para
um banco descartável. Nunca execute a suíte de migrations contra produção.

## Empacotamento e publicação

GoReleaser v2 lê `.goreleaser.yml`, gera `MailView`/`MailView.exe`, arquivos
`MailView_0.5.0_<os>_<arch>.tar.gz`, checksum
`MailView_0.5.0_checksums.txt` e a release `MailView v0.5.0`. Buildx/QEMU monta
imagens Linux multi-arquitetura. O caminho OCI permanece minúsculo por regra do
registry: `ghcr.io/jr1machado/mailview:v0.5.0`.

GitHub Actions executa:

- qualidade Go, frontend e configuração em `master` e `feature/**`;
- Gitleaks e govulncheck;
- release estável em tags `v*`;
- nightly versionada e imagens multi-arquitetura.

## Runtime e operação

| Ferramenta/componente | Papel | Comunicação |
|---|---|---|
| `MailView` | HTTP, SPA, campanhas, imports, bounces e APIs | escuta TCP 9000 |
| Caddy 2.10 | TLS 1.2/1.3, HSTS, ACME e reverse proxy | 80/443 → 9000 |
| PostgreSQL 17 | banco transacional e RLS | TCP 5432 privado |
| Docker/Compose v2 | isolamento e topologia de referência | redes `edge` e `data` |
| SMTP relay | entrega de mensagens | saída 25/465/587 |
| filesystem ou S3 | mídia/imports | volume local ou HTTPS 443 |
| DNS resolver | verificação/revalidação de domínios | saída UDP/TCP 53 |
| OIDC/captcha/postback | integrações opcionais | HTTPS 443 |

O container final usa Alpine 3.23, UID/GID 10001, filesystem read-only na
topologia de produção, capabilities removidas e secrets montados por arquivo.

## Compatibilidade herdada deliberada

O módulo Go `github.com/knadh/listmonk`, o prefixo de configuração
`LISTMONK_*` e alguns nomes de schema/database são mantidos para evitar quebra
de API, migrações e volumes existentes. Eles não identificam produto, binário,
pacote de release ou imagem publicada: esses artefatos pertencem ao MailView.
