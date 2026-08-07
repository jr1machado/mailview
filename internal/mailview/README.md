# Extensões MailView

Este diretório é reservado a módulos que pertencem ao produto MailView, e não ao Listmonk upstream.

Módulos novos devem depender de interfaces do core quando possível. Alterações transversais no Listmonk precisam ser pequenas, documentadas e cobertas por testes de compatibilidade e isolamento.

As migrations do produto serão introduzidas em `internal/mailview/migrations/` junto com o executor próprio, antes da primeira mudança de schema SaaS.
