# MailView — Issues conhecidos (mailview-v0.3.0)

Lista de limitações conhecidas nesta versão, para não serem confundidas com bugs não identificados. Itens de roadmap com prazo/priorização vivem em [`INFO/pendencias.md`](INFO/pendencias.md); aqui documentamos **o que já existe mas tem uma limitação conhecida**, para uso operacional e devido diligence.

## Isolamento multi-tenant

- **RLS ainda não ativado nas tabelas legadas.** `subscribers`, `lists` e `subscriber_lists` já têm `tenant_id`, índices e políticas RLS criadas, mas `ENABLE`/`FORCE ROW LEVEL SECURITY` só serão ligados depois que todas as rotas legadas globais (lote, activity, bounces, opt-in, subscriptions públicas) migrarem para o contexto transacional. Até lá, o isolamento dessas três tabelas depende de toda query nova passar por `tenant.InTransaction` — não há ainda uma segunda camada de defesa no banco.
- **Campanhas, templates, mídia, analytics e bounces ainda não são tenant-scoped.** Só contatos, listas e importação de contatos passaram pela migração de isolamento até esta release.
- **UI Vue administrativa não foi migrada para os endpoints `/api/mailview/*`.** Ela continua operando sobre as rotas globais do listmonk; os endpoints tenant-scoped desta release só têm cliente via API direta.
- **Ponte de Super Admin é temporária.** Todos os endpoints `/api/mailview/*` (Control Plane e Data Plane) exigem hoje o Super Admin global do listmonk (`requirePlatformAdmin` em `cmd/mailview.go`), não um papel de plataforma dedicado — é uma decisão deliberada da Fase 1, mas significa que não há hoje um usuário "apenas operador de plataforma" sem acesso de Super Admin do core.

## Importação de contatos

- **Só CSV com colunas `email`/`name`.** ZIP e o mapeamento de colunas avançado do importador legado (`internal/subimporter`: atributos customizados, pré-confirmação, delimitador configurável) não foram portados para o worker tenant-scoped.
- **Sem retomada de job parcialmente processado.** Se o processo reiniciar no meio de um job `processing`, o job fica travado nesse status — não há um mecanismo de retomada nem de detecção de job órfão nesta release.
- **Sem trilha de auditoria por job de import.** Diferente do Control Plane (que grava toda ação em `mv_audit_events`), a criação/cancelamento de job de importação não gera evento de auditoria ainda.
- **Sem rate limit dedicado no endpoint de upload.** O limite de 64 MB por arquivo existe (`io.LimitReader` em `internal/mailview/importjob`), mas não há limite de jobs simultâneos por tenant nem throttling de upload.
- **Assinatura HMAC usa uma única chave estática por instalação.** Não há rotação de chave nem versionamento de chave para `mailview.import_signing_key` (o MFA já tem `key_version` na tabela; o import ainda não).

## Escalabilidade / topologia

- **Não existe binário/imagem de worker separado.** `deploy/compose.production.yml` declara um serviço `worker` como placeholder de topologia da Fase 0; hoje o mesmo binário `api` processa campanhas e importações. Rodar réplicas com `--passive` é uma decisão operacional manual, não automática.
- **Redis está na topologia de produção mas não é usado por nenhum código do MailView.** Está reservado para uma futura camada de cache/sessão/fila; hoje é um serviço ocioso se subido.
- **Sem scheduler de capacidade.** Adicionar/remover réplicas `api` é manual; não há autoscaling nem métricas de fila expostas para orientar essa decisão.

## CI / build

- **CI não builda nem testa o frontend Vue.** O workflow `mailview-foundation.yml` roda `go vet`, `go test` e `make build` (que empacota o frontend via `stuffbin`, mas assume `frontend/dist` já existente) — não há job de lint/test do lado Vue nesta release.
- **Testes de integração do MailView são opt-in e não rodam em CI.** Todos os arquivos `*_integration_test.go` sob `internal/mailview/` são pulados sem a variável de ambiente `MAILVIEW_TEST_DSN` — nenhum workflow atual a define, então a suíte de integração roda apenas manualmente (validada localmente para esta release contra `postgres:16-alpine`).

## Documentação / branding

- **Módulo Go e nome do binário permanecem `listmonk`.** `go.mod` (`github.com/knadh/listmonk`) e o binário gerado pelo `Makefile` (`listmonk`) não foram renomeados para MailView nesta release — renomear o import path do módulo tocaria centenas de arquivos e não estava no escopo desta entrega. O branding MailView está aplicado em: nome do repositório, tag de release, imagens/serviços Docker (`deploy/compose.production.yml`, usuário runtime `mailview`), documentação e API (`/api/mailview/*`).
