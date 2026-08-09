# Integrações do MailView v0.4.0

| Integração | Comunicação | Detalhes e limite atual |
|---|---|---|
| PostgreSQL | TCP 5432 | obrigatório, PostgreSQL 17 recomendado, `pgcrypto`, pool e TLS configuráveis; role sem `BYPASSRLS` |
| SMTP | saída 25/465/587 | múltiplos relays, auth/TLS/STARTTLS, SMTP nomeado/grupo; configuração ainda global |
| HTTP postback | saída HTTP(S) | messenger externo para campanhas/transacionais; use HTTPS/auth |
| filesystem | volume local | mídia sob prefixo UUID do tenant; volume compartilhado necessário em cluster |
| S3-compatible | HTTPS 443 | endpoint/região/bucket/IAM ou credencial, URL pública/presigned e prefixo tenant |
| OIDC | HTTPS 443 + `/auth/oidc` | Keycloak, Authentik, Google e providers compatíveis; membership MailView continua explícita |
| ALTCHA/hCaptcha | HTTP(S) | proteção configurável de páginas públicas |

## Bounces

- mailbox POP com TLS e intervalo configurável;
- webhook genérico autenticado em `/webhooks/bounce`;
- Amazon SES/SNS, Azure ACS/Event Grid, SendGrid, Postmark, ForwardEmail e Lettermint em `/webhooks/service/:service`;
- validações específicas incluem assinaturas/secrets/credentials conforme provedor;
- tenant é inferido de subscriber/campanha; evento ambíguo não deve escrever cross-tenant.

## Domínios e DNS

O Control Plane guarda hostname, finalidade (`portal`, `tracking`, `sending`, `return_path`, `public_forms`), método TXT/CNAME, token, status e TLS. A verificação nesta versão é marcada manualmente: não há consulta DNS nem emissão TLS automática por tenant. Caddy atende o host principal; domínios adicionais exigem operação no proxy/DNS.

## APIs e proxy

REST usa HTTP(S)/JSON e sessão, Basic/API token conforme endpoint. Preserve `Host` e `X-Forwarded-Proto`. Webhooks entram em 443; S3, OIDC, postbacks e provedores saem em 443.

Não existem gateway de pagamento, CRM nativo, Redis/Kafka/RabbitMQ, provider DNS automático, OpenTelemetry/Prometheus dedicado ou SIEM export estruturado.
