# Integrações do MailView v0.6.0

| Integração | Comunicação | Detalhes e limite atual |
|---|---|---|
| PostgreSQL | TCP 5432 | obrigatório, PostgreSQL 17 recomendado, `pgcrypto`, pool e TLS configuráveis; role sem `BYPASSRLS` |
| SMTP | saída 25/465/587 | múltiplos relays, auth/TLS/STARTTLS, SMTP nomeado/grupo; configuração ainda global |
| HTTP postback | saída HTTP(S) | messenger externo para campanhas/transacionais; use HTTPS/auth |
| filesystem | volume local | mídia sob prefixo UUID do tenant; volume compartilhado necessário em cluster |
| S3-compatible | HTTPS 443 | endpoint/região/bucket/IAM ou credencial, URL pública/presigned e prefixo tenant |
| OIDC | HTTPS 443 + `/auth/oidc` | Keycloak, Authentik, Google e providers compatíveis; membership MailView continua explícita |
| ALTCHA/hCaptcha | HTTP(S) | proteção configurável de páginas públicas |
| DNS recursivo | UDP/TCP 53 | TXT/CNAME de ownership e revalidação periódica; timeouts não ativam domínio |
| ACME/Caddy | HTTP/HTTPS 80/443 | certificado e redirect HTTPS no edge; chaves privadas não entram no banco MailView |

## Contratos e responsabilidade

| Integração | MailView fornece | Operador/provedor fornece |
|---|---|---|
| PostgreSQL | schema, migrations, RLS, pool e health | instância, TLS, backup, HA, IOPS e retenção |
| SMTP | cliente, autenticação, TLS, seleção e tracking | relay, reputação, limites, SPF/DKIM/DMARC e feedback loops |
| DNS/domínios | desafio, consulta e estado de ownership | registros autoritativos, delegação e propagação |
| Caddy/ACME | hostname verificado e estado TLS por API | emissão, renovação, chave privada e roteamento edge |
| S3/filesystem | prefixo e validação de tenant | bucket/volume, IAM, criptografia, lifecycle e backup |
| OIDC | authorization code flow e mapeamento de usuário | IdP, client, redirect URI, grupos e lifecycle de membership |
| captcha | endpoints/configuração de validação | conta, site key, secret e disponibilidade do provedor |
| postback | payload HTTP do messenger | endpoint, autenticação, retry/idempotência e observabilidade |

Credenciais devem entrar por configuração protegida ou arquivos de secret; o
contrato de infraestrutura dedicada persiste referências, não o segredo de
banco, SMTP, storage ou KMS.

## Bounces

- mailbox POP com TLS e intervalo configurável;
- webhook genérico autenticado em `/webhooks/bounce`;
- Amazon SES/SNS, Azure ACS/Event Grid, SendGrid, Postmark, ForwardEmail e Lettermint em `/webhooks/service/:service`;
- validações específicas incluem assinaturas/secrets/credentials conforme provedor;
- tenant é inferido de subscriber/campanha; evento ambíguo não deve escrever cross-tenant.

## Domínios e DNS

O Control Plane guarda hostname, finalidade (`portal`, `tracking`, `sending`, `return_path`, `public_forms`), desafio TXT/CNAME e estado DNS/TLS. A aplicação consulta o resolver configurado antes de ativar e revalida a cada 24h por padrão; perda de propriedade remove a rota e revoga o estado TLS. Caddy ou outro controller continua responsável por ACME e reporta `pending|issued|failed|revoked` pela API, usando apenas uma referência de certificado.

## APIs e proxy

REST usa HTTP(S)/JSON e sessão, Basic/API token conforme endpoint. Preserve `Host` e `X-Forwarded-Proto`. Webhooks entram em 443; S3, OIDC, postbacks e provedores saem em 443.

O proxy deve manter o hostname original porque subdomínio, domínio verificado,
alias de slug e portal público dependem dele. Em topologias com múltiplos nós,
somente instâncias designadas devem executar o scanner ativo; réplicas
`--passive` continuam atendendo HTTP. Todos os nós precisam alcançar o mesmo
PostgreSQL e storage compartilhado/S3.

## Ambientes dedicados

O modo Enterprise integra-se a um provisionador externo por referências:
`database_ref`, `worker_ref`, `smtp_ref`, `storage_ref`,
`encryption_key_ref` e `docker_namespace`. O MailView valida o conjunto,
versiona a decisão de roteamento e o apresenta no mapa de ambientes. Ele não
cria VPC, database, queue, bucket, secret ou namespace nesta release.

Não existem gateway de pagamento, CRM nativo, Redis/Kafka/RabbitMQ, provider DNS automático, OpenTelemetry/Prometheus dedicado ou SIEM export estruturado. Billing, webhooks e SMTP tenant possuem modelo persistente, mas os dispatchers/gateways novos continuam separados do fluxo herdado nesta release.
