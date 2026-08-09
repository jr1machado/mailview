# Referência operacional do MailView v0.4.0

## Stack e capacidade

Build: Go 1.26.5, Node 22, Yarn 1.22, Vue 2.7.14 e Vite 5.4. Runtime: Alpine 3.23, PostgreSQL 17 e Docker Compose v2. Targets estão na configuração GoReleaser.

Não há benchmark/SLA. Baseline: piloto 2 vCPU/2–4 GiB/20 GiB SSD; produção inicial app 2–4 vCPU/4–8 GiB e PostgreSQL 4 vCPU/8 GiB/50+ GiB SSD. Dimensione por contatos, tamanho de templates, concorrência, tracking, retenção, IOPS e latência/limites do SMTP.

## Portas

| Contexto | Porta | Origem → destino |
|---|---|---|
| produção | 80/443 | Internet → Caddy |
| interna | 9000 | Caddy → MailView |
| interna | 5432 | MailView → PostgreSQL |
| compose raiz | 10443 | host → MailView 9000 |
| compose raiz | 127.0.0.1:15432 | host → PostgreSQL |
| dev | 9000/8080 | backend/Vite |
| dev | 1025/8025/8070 | MailHog SMTP/UI e Adminer |
| saída | 25/465/587 | MailView → SMTP |
| saída | 443 | MailView → S3/OIDC/postback |

Não exponha 5432, 9000 nem portas dev à Internet.

## Bootstrap

`config.toml.sample` contém app, MailView e DB. SMTP, bounces, storage, OIDC, captcha, aparência e performance ficam em `settings` e no painel.

| Chave `[mailview]` | Uso |
|---|---|
| `mfa_encryption_key` | base64 de 32 bytes, AES-256-GCM |
| `import_signing_key` | base64 de 32 bytes, HMAC |
| `import_storage_dir` | raiz persistente dos CSVs |
| `tenant_routing_enabled` | resolução por hostname |
| `tenant_base_domain` | sufixo de `{slug}.dominio` |

O prefixo env compatível é `LISTMONK_`, `__` representa seção e `_FILE` carrega secret.

## Upgrade, backup e observabilidade

Antes de upgrade, faça backup consistente de PostgreSQL, mídia, imports necessários, secrets/config e dados ACME. Rode `mailview --upgrade --yes`, inicie, valide `/health`, logs, login, host tenant e campanha de teste. Migrations podem não ser reversíveis; restore deve ser testado fora de produção.

Centralize stdout/stderr e monitore reinícios, latência/5xx, pool/IOPS/disco do PostgreSQL, campanhas, bounces e imports. Auditoria está em `mv_audit_events`. Não há endpoint Prometheus próprio.

## Segurança operacional

Use UID 10001, filesystem read-only, TLS, secrets `0600`, role `NOSUPERUSER NOBYPASSRLS`, limites de body/rate no proxy, SPF/DKIM/DMARC e scans contínuos. PostgreSQL fora da rede local deve usar TLS.

## Validação

```sh
go vet ./...
go test ./...
make build && ./mailview --version
(cd frontend && yarn lint && yarn build)
docker compose config --quiet
docker compose --env-file deploy/.env.example -f deploy/compose.production.yml config --quiet
```

Testes integrados usam um `MAILVIEW_TEST_DSN` administrativo para migrations/RLS e depois um DSN de aplicação restrita para `control`, `dataplane` e `importjob`. Nunca use banco de produção.

Artefatos: `mailview`, `MailView_<versão>_<os>_<arch>.tar.gz`, imagem `ghcr.io/jr1machado/mailview` e tag `mailview-v<semver>`.
