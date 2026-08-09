# Fase 2 — Isolamento por tenant

## Entrega inicial

Esta primeira entrega estabelece o contrato obrigatório que será usado na migração do Data Plane:

- `tenant.Context` carrega `tenant_id`, `user_id` e `request_id` somente no contexto do servidor.
- `tenant.Begin` usa `set_config(..., true)` dentro de uma transação. É o equivalente parametrizado de `SET LOCAL`; o estado desaparece no commit/rollback e não vaza entre conexões do pool.
- `mv_tenant_settings` é a primeira tabela sob `FORCE ROW LEVEL SECURITY`, com política baseada em `app.tenant_id`.
- O teste de integração cria duas tenants e comprova que uma role sem `BYPASSRLS` não lê nem insere dados da outra.

## Rollout seguro do Data Plane legado

As tabelas atuais do Listmonk não receberam RLS nesta entrega. Elas usam statements preparados e acessos diretos ao pool; habilitar RLS antes de migrá-las para `tenant.Begin` bloquearia requisições legítimas ou incentivaria um bypass inseguro.

A sequência obrigatória para cada agregado é:

1. acrescentar `tenant_id` e índices compostos;
2. preservar integridade com FKs compostas entre entidades relacionadas;
3. mover todas as queries e jobs do agregado para a transação tenant-scoped;
4. adicionar testes de leitura, escrita, exportação e job cross-tenant;
5. então ativar `ENABLE` e `FORCE ROW LEVEL SECURITY` para aquele agregado.

O primeiro agregado a migrar será **subscribers + lists + subscriber_lists**, pois é a fronteira de maior risco de vazamento. Campanhas, analytics, mídia e SMTP só entram após essa migração estar protegida.

## Segundo incremento

O schema desse agregado agora possui `tenant_id`, índices por tenant, unicidade de e-mail por tenant e FKs compostas que impedem um vínculo `subscriber_lists` entre tenants. A API nova fica em `/api/mailview/tenants/:tenantID/data/{lists,subscribers}` e sempre usa o contrato transacional.

## Terceiro incremento — RLS e autorização granular

`subscribers`, `lists` e `subscriber_lists` agora usam `ENABLE` e `FORCE ROW LEVEL SECURITY`. Requisições SaaS continuam definindo o tenant com `SET LOCAL`; conexões sem contexto são confinadas ao tenant especial `legacy-workspace`, preservando compatibilidade operacional com jobs upstream sem permitir que uma query esquecida leia tenants SaaS.

As rotas autenticadas de contatos e listas resolvidas por host exigem permissões MailView antes de abrir o Data Plane:

- `subscriber.read.tenant` para leitura;
- `subscriber.manage.tenant` para criação, alteração, bloqueio, associação e exclusão;
- `subscriber.export.tenant` para CSV;
- `subscriber.import.tenant` para importação;
- `list.read.tenant` e `list.manage.tenant` para listas.

Operações em lote por IDs foram migradas para transações tenant-scoped e validam todos os IDs antes de alterar dados. Expressões SQL arbitrárias permanecem desabilitadas no Data Plane SaaS. Activity, bounces, opt-in e o privacy export completo respondem explicitamente como indisponíveis para hosts tenant até que seus agregados também sejam migrados; não há fallback silencioso para queries globais.

## Quarto incremento — agregados completos e interface tenant-scoped

O isolamento foi estendido a `templates`, `campaigns`, `campaign_lists`, `campaign_views`, `media`, `campaign_media`, `links`, `link_clicks` e `bounces`. Todas essas tabelas possuem `tenant_id` obrigatório, índices por tenant, FKs compostas entre agregados e `ENABLE` + `FORCE ROW LEVEL SECURITY`. Defaults derivados do contexto transacional mantêm compatibilidade com os statements upstream sem aceitar acesso global.

As APIs usadas pela interface principal para campanhas, templates, mídia, analytics e bounces agora exigem permissões MailView específicas e executam o core do Listmonk dentro da mesma transação tenant-scoped. O perfil autenticado publica somente as permissões efetivas da membership atual; a navegação Vue usa essas permissões e os endpoints administrativos globais são bloqueados em hosts de tenant.

A tela de importação detecta o host tenant e usa os jobs CSV isolados, incluindo polling, progresso e cancelamento efetivo entre lotes. Opções exclusivas do importador ZIP upstream são ocultadas nesse modo; o workspace legado mantém o comportamento original.

Os fluxos públicos resolvem o tenant exclusivamente pelo hostname reconhecido pelo servidor — subdomínio `{slug}.{tenant_base_domain}` ou domínio personalizado previamente verificado — antes de abrir a transação. Isso cobre formulários e páginas de inscrição, confirmação, unsubscribe, export/wipe, arquivos públicos, feeds e tracking de visualizações e links.

O worker de campanhas resolve o tenant da campanha antes de buscar destinatários, anexos ou criar links, e o gravador de bounces faz resolução inequívoca antes de escrever. Arquivos do provider local são armazenados sob `<tenant_id>/...`, incluindo thumbnails; a leitura normaliza URLs e rejeita traversal de diretório.

O teste PostgreSQL com duas tenants verifica o isolamento dos doze conjuntos protegidos (`subscribers`, `lists`, `subscriber_lists` e os nove agregados adicionados neste incremento), inclusive sob uma role sem `BYPASSRLS`.

## Fechamento da sprint — 2026-08-09

A auditoria final do código e a execução das suítes contra um PostgreSQL limpo fecharam as seguintes lacunas que ainda existiam entre o contrato acima e a implementação:

- activity, bounces, exclusão de bounces e export individual de subscriber não reaplicam permissões globais depois que o middleware já autorizou e abriu a transação tenant-scoped;
- activity exige `analytics.read.tenant`, enquanto leitura e exclusão de bounces exigem respectivamente `bounce.read.tenant` e `bounce.manage.tenant`;
- IDs de listas, mídia e templates recebidos na criação/alteração de campanhas são validados dentro da transação RLS, rejeitando referências inexistentes ou cross-tenant em vez de omiti-las silenciosamente;
- domínios personalizados verificados são resolvidos por consultas explicitamente tenant-scoped, compatíveis com a role real sem `BYPASSRLS`;
- tenants suspensas não atendem fluxos públicos;
- mídia filesystem e S3 servida pela própria aplicação valida o primeiro prefixo UUID do tenant, preserva subdiretórios no S3 e rejeita leitura cross-tenant/traversal;
- URLs de mídia hospedadas pelo próprio MailView permanecem relativas em hosts tenant, e a exclusão remove o thumbnail físico exato;
- helpers `SECURITY DEFINER` de descoberta de tenant, já sem consumidores, são removidos pela migration 10;
- o teste RLS agora cria e verifica uma linha isolada em cada um dos doze conjuntos, incluindo `subscriber_lists` e `campaign_lists`;
- o teste de importação prepara e consulta fixtures exclusivamente por `tenant.InTransaction`, comprovando o comportamento com `FORCE RLS` e a role `listmonk_app`.

Validação executada:

```text
go test ./...
MAILVIEW_TEST_DSN=<superuser-test-dsn> go test -count=1 -v ./internal/mailview/migrations ./internal/mailview/tenant
MAILVIEW_TEST_DSN=<restricted-app-test-dsn> go test -count=1 -v ./internal/mailview/control
MAILVIEW_TEST_DSN=<restricted-app-test-dsn> go test -count=1 -v ./internal/mailview/dataplane
MAILVIEW_TEST_DSN=<restricted-app-test-dsn> go test -count=1 -v ./internal/mailview/importjob
npm run lint
npm run build
```

Todas as validações acima passaram. O build do frontend mantém apenas os avisos preexistentes de depreciação do Sass e tamanho de chunks.
