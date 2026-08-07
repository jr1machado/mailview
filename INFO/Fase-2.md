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

As políticas RLS foram instaladas, mas ainda não estão ativadas nas três tabelas: as rotas administrativas legadas do Listmonk continuam globais. A ativação ocorrerá somente junto com a substituição dessas rotas pelo Data Plane tenant-scoped; até lá, a API nova aplica escopo tanto na transação como nas queries.
