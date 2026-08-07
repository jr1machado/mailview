# ADRs do MailView

**Status:** aceitos para a Fase 0  
**Data:** 2026-08-06

Este documento transforma as decisões da [Bíblia de Arquitetura](Biblia-Projeto.md) em regras que orientam implementação e revisão.

## ADR-001 — Isolamento padrão por banco compartilhado e RLS

O plano padrão usará PostgreSQL compartilhado, `tenant_id` obrigatório e Row-Level Security. A aplicação nunca se conectará com uma role que tenha `BYPASSRLS`.

Consequência: antes de migrar tabelas de negócio, toda operação deve passar por uma transação com contexto de tenant. Não são aceitas queries que recebam o tenant como parâmetro controlável pelo cliente.

## ADR-002 — Isolamento dedicado para Enterprise

O Control Plane deverá ser capaz de rotear um tenant para banco, worker, SMTP, storage e chave dedicados.

Consequência: identificadores externos e a resolução de tenant não podem pressupor uma única conexão de banco.

## ADR-003 — MFA inicial com TOTP

TOTP RFC 6238 será o primeiro segundo fator. WebAuthn entra posteriormente.

Consequência: os segredos devem ser criptografados em repouso; recovery codes, prevenção a replay, rate limit e step-up são obrigatórios antes do uso comercial.

## ADR-004 — Endereçamento por subdomínio e domínio verificado

O endereço padrão será `{tenant-slug}.mailview.com.br`; domínios personalizados só serão ativados após prova de posse DNS.

Consequência: o host deve ser normalizado e validado por middleware central. Nunca deve definir tenant diretamente a partir de um corpo de requisição.

## ADR-005 — Separar contrato de frontend e backend

O Listmonk atual entrega o frontend Vue embutido no binário. O MailView evoluirá para um frontend e uma API com contratos versionados.

Consequência: funcionalidades novas não devem depender de estado global do frontend atual; a migração pode ser incremental para preservar compatibilidade com o upstream.

## ADR-006 — Contexto transacional obrigatório

Todo acesso ao Data Plane deve executar na mesma transação que define `app.tenant_id`, `app.user_id` e `app.request_id` com `SET LOCAL`.

Consequência: a refatoração inicial é uma camada de acesso transacional. Adicionar apenas colunas `tenant_id` sem essa camada é proibido.

## ADR-007 — API keys com escopo e tenant imutável

Chaves de API serão armazenadas somente como hash, terão prefixo, escopos, expiração e um tenant imutável.

Consequência: o modelo atual de usuários de API globais do Listmonk não será reutilizado como contrato SaaS.

## ADR-008 — Imagens reproduzíveis e não-root

Imagens de produção usam versões fixadas, usuário não-root, filesystem somente leitura quando possível, secrets por arquivo e scans no pipeline.

Consequência: `latest`, credenciais padrão e segredos em Compose são proibidos fora do desenvolvimento local.
