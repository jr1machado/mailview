# Fase 0 — Fundação

## Entregas deste sprint

- Branch de trabalho `feature/sprint01`.
- ADRs aceitos para isolamento, MFA, domínios, API e imagens.
- Threat model e invariantes de segurança versionados.
- Imagem de runtime fixada e executada por usuário não-root.
- Esqueleto de deploy de produção com redes segregadas, secrets por arquivo e imagens explicitamente versionadas.
- Pipeline para teste, vet, build, secret scan e análise de vulnerabilidades Go.
- Convenção de extensões MailView para manter o fork sincronizável.

## Convenção de extensão do fork

1. Código novo do produto vive em `internal/mailview/` e `cmd/mailview/` quando possível.
2. Migrations MailView têm registro próprio em `internal/mailview/migrations/`; não entram no `migList` sem análise explícita de compatibilidade com upstream.
3. Tabelas do Control Plane usam prefixo `mv_` até a separação física do banco estar concluída.
4. Mudanças no core upstream exigem um teste de regressão e uma nota de compatibilidade em pull request.
5. O repositório acompanha `upstream/master` via uma branch de sincronização; a branch de produto nunca recebe merge direto sem CI completo.

## Limites deliberados

Esta fase não cria tenants, não altera tabelas existentes e não habilita RLS. A Fase 1 iniciou o Control Plane em tabelas `mv_*`; `tenant_id` no Data Plane e RLS continuam dependentes da camada transacional da Fase 2.
