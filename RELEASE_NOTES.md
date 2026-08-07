# MailView — Release Notes

## mailview-v0.3.0 — 2026-08-07

Primeira tag versionada do fork **MailView**. Consolida as Fases 0, 1 e a primeira metade da Fase 2 do roadmap SaaS multi-tenant descrito em [`INFO/Biblia-Projeto.md`](INFO/Biblia-Projeto.md), incluindo a entrega mais recente: **importação tenant-scoped de contatos via CSV**.

### Novidades desta release

**Importação tenant-scoped (CSV)**
- `mv_import_jobs` e `mv_import_files`, sob `FORCE ROW LEVEL SECURITY`, com idempotency key por tenant e assinatura HMAC do arquivo enviado.
- Upload isolado por tenant (`import_storage_dir/<tenant_id>/<job_id>.csv`).
- Worker CSV em lotes de 500 linhas por transação tenant-scoped (`tenant.InTransaction`), com revalidação de ownership de listas e da assinatura do arquivo antes de processar.
- Endpoints `POST/GET /api/mailview/tenants/:tenantID/data/import-jobs`, `GET .../import-jobs/:jobID`, `POST .../import-jobs/:jobID/cancel`.
- Nova configuração obrigatória `mailview.import_signing_key` (base64 de 32 bytes) — sem ela, criação de job é recusada em vez de assinar com chave vazia.
- Teste de integração cobrindo import concorrente entre duas tenants, rejeição de lista de outra tenant e replay de idempotency key.

**Base entregue nas fases anteriores, incluída nesta tag**
- Control Plane multi-tenant: tenants, memberships, 7 papéis padrão por tenant, permissões granulares, auditoria append-only.
- MFA TOTP com segredo cifrado (AES-256-GCM) e recovery codes de uso único (bcrypt).
- Contexto transacional de tenant (`tenant.Context` / `set_config(..., true)`) como única forma permitida de acessar dado tenant-scoped.
- `tenant_id`, índices e FKs compostas em `subscribers`, `lists` e `subscriber_lists`; Data Plane tenant-scoped para CRUD de contatos e listas.
- Resolução opcional de tenant por subdomínio, validando membership ativa.
- Topologia de produção de referência com redes segregadas, secrets por arquivo, imagem runtime não-root.
- Pipeline de CI com `go vet`, `go test`, build, scan de segredos e de vulnerabilidades Go.

### Mudanças de configuração

Duas novas chaves em `config.toml` / variáveis `LISTMONK_mailview__*`:

```toml
[mailview]
import_storage_dir = "./mailview-imports"   # diretório raiz dos uploads de import, um subdiretório por tenant
import_signing_key = ""                      # base64 de 32 bytes — obrigatório para habilitar import de contatos
```

`import_signing_key` vazio **desabilita** a criação de jobs de importação (mesmo comportamento defensivo já usado por `mfa_encryption_key`). Gere com:

```sh
openssl rand -base64 32
```

### Compatibilidade

- Nenhuma tabela legada do listmonk (`subscribers`, `lists`, `campaigns`, etc.) teve seu contrato de leitura alterado por esta release fora da adição de `tenant_id` já entregue na Fase 2. RLS **não** foi ativado em nenhuma tabela legada nesta release.
- Migrations do MailView (`internal/mailview/migrations`) rodam em ledger próprio (`mv_schema_migrations`), fora do `migList` do upstream listmonk — atualizações do binário continuam aplicando `--upgrade` normalmente.

### Como atualizar

```sh
./listmonk --upgrade --yes --config config.toml
```

As migrations do MailView (versões 1 a 4 do ledger `mv_schema_migrations`) são aplicadas automaticamente no boot do processo, além do `--upgrade` explícito.

Veja limitações conhecidas desta versão em [`ISSUES_CONHECIDOS.md`](ISSUES_CONHECIDOS.md).
