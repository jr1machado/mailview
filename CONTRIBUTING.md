# Contribuindo com o MailView

O MailView é desenvolvido e versionado como fork independente. Issues, pull
requests, decisões de arquitetura e releases deste repositório não são geridos
pelo projeto de origem.

## Antes de alterar

1. Pesquise issues existentes e descreva o problema, o resultado esperado e o
   impacto em tenants, segurança e compatibilidade.
2. Para mudanças de schema, RBAC, autenticação, filas ou integrações, registre a
   estratégia de migração e rollback.
3. Mantenha o escopo da pull request pequeno e não misture refatorações sem
   relação com a funcionalidade.

## Ambiente e validação

Use as versões e comandos documentados em `docs/FERRAMENTAS.md`. Como baseline,
execute:

```sh
go test ./...
go vet ./...
yarn --cwd frontend lint
yarn --cwd frontend build
make build
```

Novas funções devem incluir testes proporcionais ao risco. Mudanças de interface
devem preservar acessibilidade e isolamento por tenant; mudanças de dados devem
ser verificadas com uma role PostgreSQL `NOBYPASSRLS`.

## Convenções do projeto

- Produto, binário, pacote distribuível e título de release: **MailView**.
- Identificadores que exigem minúsculas, como módulos Go, pacotes npm e imagens
  OCI, podem usar `mailview`; isso não altera o nome comercial.
- Preserve compatibilidade legada apenas quando ela estiver explicitamente
  documentada.
- Atualize README, arquitetura, referência operacional, release notes e issues
  conhecidos quando uma mudança afetar o contrato publicado.

Ao contribuir, mantenha as atribuições e licenças dos componentes herdados e dos
novos componentes. Interações devem ser técnicas, respeitosas e reproduzíveis.
