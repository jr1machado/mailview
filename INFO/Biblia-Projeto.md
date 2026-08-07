# MailView
## Bíblia de Arquitetura SaaS Multi-Tenant baseada em Listmonk

**Versão:** 1.0  
**Data:** 06/08/2026  
**Público:** arquitetura, backend, frontend, DevOps, segurança, QA, produto e operações  
**Objetivo:** orientar o desenvolvimento de um produto SaaS premium para gerenciamento de mailing, campanhas, domínios e clientes, utilizando o Listmonk como base técnica e preservando isolamento forte entre tenants.

---

# 1. Sumário executivo

O MailView deve ser construído como uma plataforma SaaS multi-tenant com dois planos arquiteturais separados:

1. **Control Plane**: tenants, usuários, autenticação, MFA, RBAC, billing, planos, quotas, domínios, provisionamento, auditoria e operação global.
2. **Data Plane**: contatos, listas, campanhas, templates, métricas, SMTP, bounces, webhooks e entregabilidade.

O Listmonk deve ser tratado como um **motor de mailing incorporado ao produto**, e não como o produto inteiro. A customização deve evitar espalhar lógica de tenant de forma improvisada pelo código. O isolamento precisa existir no banco, na aplicação, no armazenamento, nas filas, nos logs, nas métricas, nos backups e nas integrações.

A arquitetura recomendada é híbrida:

- **Plano padrão:** aplicação compartilhada com banco PostgreSQL compartilhado, `tenant_id` obrigatório e Row-Level Security.
- **Plano Enterprise:** tenant dedicado com banco, SMTP, domínio de tracking e workers dedicados.
- **Control Plane único:** administra os dois modelos.

O produto deverá suportar URLs como:

```text
clientea.mailview.com.br
clienteb.mailview.com.br
```

E também domínios personalizados:

```text
mail.cliente.com.br
news.cliente.com.br
```

A autenticação deve usar senha forte, MFA TOTP compatível com Microsoft Authenticator e Google Authenticator, códigos de recuperação, gestão de sessões e trilha de auditoria.

---

# 2. Princípios arquiteturais

## 2.1 Princípios obrigatórios

1. **Tenant isolation by design**: nenhuma entidade de negócio pode existir sem vínculo explícito ao tenant.
2. **Deny by default**: todo acesso é negado até que uma política o permita.
3. **Defense in depth**: filtros na aplicação e RLS no banco.
4. **Zero trust entre serviços**: cada serviço valida identidade, tenant e autorização.
5. **Immutable audit trail**: eventos administrativos e sensíveis devem ser auditáveis.
6. **No shared secrets in code**: segredos em secret manager ou Docker secrets.
7. **Idempotência**: webhooks, imports, provisionamento e jobs devem tolerar repetição.
8. **Observabilidade por tenant**: logs, métricas e tracing devem carregar `tenant_id`.
9. **Separação de domínio**: autenticação, mailing, billing e entregabilidade não devem ficar acoplados em um único módulo.
10. **Compatibilidade de upgrade**: manter o fork próximo do upstream e isolar extensões.

## 2.2 Restrições arquiteturais

- Todos os componentes executados em containers Docker.
- PostgreSQL como banco principal.
- Proxy reverso com TLS automático.
- Filas assíncronas para envios, imports, webhooks e geração de relatórios.
- MFA TOTP obrigatório para perfis administrativos.
- Domínios e remetentes verificados antes de uso.
- Nenhum token de API pode atravessar tenants.

---

# 3. Contexto do Listmonk

O Listmonk é uma aplicação self-hosted de mailing e newsletter, escrita em Go, com frontend Vue e PostgreSQL. Sua distribuição oficial pode ser executada como binário único ou via Docker. A base é licenciada sob AGPLv3.

A plataforma possui bom desempenho, listas, campanhas, templates, mensagens transacionais, métricas e múltiplos SMTPs. Também possui suporte a múltiplos usuários e permissões. Contudo, permissões de usuário não equivalem a isolamento SaaS por tenant.

## 3.1 Decisão

O MailView utilizará o Listmonk como **foundation layer**, mas adicionará:

- contexto de tenant;
- RLS;
- RBAC SaaS;
- MFA;
- control plane;
- billing;
- quotas;
- domínios personalizados;
- gestão de DNS;
- branding por tenant;
- auditoria ampliada;
- políticas de retenção;
- isolamento de mídia;
- segregação de SMTP;
- administração global;
- portal do cliente.

## 3.2 Estratégia de fork

Criar um fork corporativo com três trilhas:

```text
upstream/master
mailview/upstream-sync
mailview/main
```

Regras:

- commits de customização devem ser pequenos e temáticos;
- evitar editar diretamente módulos upstream quando uma extensão for possível;
- todo merge de upstream deve executar suíte completa de isolamento;
- migrations próprias devem usar faixa de versionamento distinta;
- mudanças de segurança upstream devem ter SLA de incorporação.

---

# 4. Arquitetura lógica

```text
                         Internet
                            |
                      DNS / CDN / WAF
                            |
                    Traefik ou Nginx
                            |
          +-----------------+------------------+
          |                                    |
   clientea.mailview.com.br             admin.mailview.com.br
          |                                    |
          +-----------------+------------------+
                            |
                       Web Frontend
                            |
                       API Gateway
                            |
       +--------------------+--------------------+
       |                    |                    |
 Identity Service     Control Plane API     Mailing API
       |                    |                    |
       |              Tenant/Billing/RBAC       |
       |                    |                    |
       +--------------------+--------------------+
                            |
                     PostgreSQL Cluster
                            |
                    Row-Level Security
                            |
       +--------------------+--------------------+
       |                    |                    |
   Queue/Workers       Object Storage       Audit Store
       |                    |                    |
  SMTP Providers       Tenant prefixes      Append-only
```

---

# 5. Componentes

## 5.1 Frontend Portal do Cliente

Responsabilidades:

- dashboard do tenant;
- assinantes e listas;
- campanhas;
- templates;
- remetentes;
- domínios;
- usuários;
- MFA;
- auditoria visível ao cliente;
- relatórios;
- plano e consumo;
- API keys;
- webhooks;
- preferências de branding.

## 5.2 Frontend Portal Administrativo

Responsabilidades:

- visão global de tenants;
- provisionamento;
- suspensão e reativação;
- quotas;
- planos;
- billing;
- saúde do ambiente;
- incidentes;
- impersonation controlada;
- suporte;
- gestão de domínios;
- métricas globais;
- auditoria global.

## 5.3 Identity Service

Responsabilidades:

- login;
- MFA TOTP;
- recuperação de conta;
- sessões;
- política de senha;
- lockout;
- device/session management;
- OIDC futuro;
- rotação de refresh tokens;
- emissão de tokens internos.

## 5.4 Control Plane API

Responsabilidades:

- tenants;
- memberships;
- RBAC;
- planos;
- quotas;
- billing;
- domínios;
- configuração de tenant;
- provisionamento dedicado;
- políticas;
- feature flags.

## 5.5 Mailing API

Responsabilidades:

- assinantes;
- listas;
- campanhas;
- templates;
- imports;
- métricas;
- bounces;
- mensagens transacionais;
- webhooks;
- SMTP e rate limits.

## 5.6 Worker Service

Responsabilidades:

- envio de e-mails;
- importação CSV;
- exportações;
- processamento de bounce;
- webhooks;
- relatórios;
- verificação DNS;
- limpeza e retenção;
- reprocessamento de jobs.

---

# 6. Modelo de multi-tenancy

## 6.1 Estratégia principal

Banco compartilhado com `tenant_id` em todas as entidades e RLS no PostgreSQL.

Exemplo:

```sql
CREATE TABLE subscribers (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    email CITEXT NOT NULL,
    name TEXT,
    status TEXT NOT NULL,
    attributes JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, email)
);
```

## 6.2 Regras de tenant

- `tenant_id` nunca vem diretamente de um campo editável pelo usuário.
- O tenant é resolvido por host, sessão ou token.
- Toda transação abre contexto de tenant.
- Toda query deve estar dentro de transação com `SET LOCAL`.
- Jobs assíncronos carregam `tenant_id` assinado no payload.
- Logs recebem `tenant_id`, `user_id` e `request_id`.

## 6.3 Row-Level Security

```sql
ALTER TABLE subscribers ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscribers FORCE ROW LEVEL SECURITY;

CREATE POLICY subscribers_tenant_policy
ON subscribers
USING (
  tenant_id = current_setting('app.tenant_id', true)::uuid
)
WITH CHECK (
  tenant_id = current_setting('app.tenant_id', true)::uuid
);
```

## 6.4 Contexto por transação

```sql
BEGIN;
SET LOCAL app.tenant_id = '00000000-0000-0000-0000-000000000001';
SET LOCAL app.user_id = '123';
-- queries
COMMIT;
```

## 6.5 Proibição

Não utilizar conexão PostgreSQL com usuário que possua `BYPASSRLS` na aplicação.

## 6.6 Plano Enterprise dedicado

O sistema deverá permitir mover um tenant para:

- banco dedicado;
- worker dedicado;
- SMTP dedicado;
- storage dedicado;
- chave criptográfica dedicada;
- namespace Docker dedicado.

O roteamento será feito pelo Control Plane.

---

# 7. Modelo de dados principal

## 7.1 Entidades do Control Plane

- `tenants`
- `tenant_settings`
- `tenant_branding`
- `tenant_domains`
- `tenant_plans`
- `tenant_quotas`
- `tenant_usage`
- `users`
- `memberships`
- `roles`
- `permissions`
- `role_permissions`
- `membership_roles`
- `mfa_methods`
- `recovery_codes`
- `sessions`
- `api_keys`
- `audit_events`
- `billing_accounts`
- `subscriptions`
- `invoices`
- `feature_flags`

## 7.2 Entidades do Data Plane

- `subscribers`
- `lists`
- `subscriber_lists`
- `campaigns`
- `campaign_lists`
- `templates`
- `media`
- `smtp_profiles`
- `sender_identities`
- `domains`
- `domain_dns_records`
- `bounces`
- `complaints`
- `campaign_events`
- `link_clicks`
- `campaign_views`
- `imports`
- `exports`
- `webhooks`
- `webhook_deliveries`
- `transactional_messages`

## 7.3 Índices obrigatórios

Todo índice funcional deve começar por `tenant_id` quando aplicável:

```sql
CREATE INDEX idx_campaigns_tenant_status
ON campaigns (tenant_id, status, scheduled_at);
```

---

# 8. Resolução de tenant e domínios

## 8.1 Subdomínios padrão

```text
{tenant-slug}.mailview.com.br
```

Regras de slug:

- 3 a 63 caracteres;
- letras minúsculas, números e hífen;
- único globalmente;
- nomes reservados bloqueados;
- alteração exige workflow e redirecionamento controlado.

## 8.2 Domínios personalizados

Um tenant pode cadastrar múltiplos domínios:

```text
mail.cliente.com.br
news.cliente.com.br
comunicados.cliente.com.br
```

Cada domínio precisa de:

- token de verificação;
- CNAME ou TXT;
- status de validação;
- certificado TLS;
- data de última verificação;
- tenant owner;
- finalidade.

## 8.3 Tipos de domínio

1. **Portal**: acesso ao painel.
2. **Tracking**: links e pixels.
3. **Sending**: domínio remetente.
4. **Return-Path**: bounces.
5. **Public forms**: inscrição e descadastro.

## 8.4 Proteção contra domain takeover

- nunca ativar domínio antes de validação DNS;
- revogar certificado quando domínio for removido;
- revalidar periodicamente;
- impedir que o mesmo hostname seja usado por dois tenants;
- bloquear wildcard inseguro;
- auditar mudança de domínio.

---

# 9. Autenticação e MFA

## 9.1 Métodos

- e-mail e senha;
- TOTP RFC 6238;
- códigos de recuperação;
- OIDC/SAML em fase posterior;
- WebAuthn como evolução premium.

## 9.2 Compatibilidade

O QR Code TOTP deve funcionar com:

- Microsoft Authenticator;
- Google Authenticator;
- Authy;
- 1Password;
- Bitwarden.

## 9.3 Enrolamento MFA

Fluxo:

1. usuário informa senha atual;
2. servidor gera segredo aleatório;
3. segredo é armazenado criptografado;
4. QR Code é exibido uma única vez;
5. usuário confirma dois códigos consecutivos;
6. sistema ativa MFA;
7. sistema gera 10 códigos de recuperação;
8. evento é auditado.

## 9.4 Requisitos TOTP

- SHA-1 por compatibilidade;
- 6 dígitos;
- período de 30 segundos;
- tolerância máxima de 1 janela anterior/posterior;
- proteção contra replay;
- rate limit por usuário e IP;
- segredo com no mínimo 160 bits;
- issuer configurado como `MailView` ou marca do tenant.

## 9.5 Políticas

- Super Admin: MFA obrigatório.
- Tenant Admin: MFA obrigatório.
- Operador: configurável, recomendado obrigatório.
- Visualizador: configurável.
- API users: sem login interativo, apenas chaves e escopos.

## 9.6 Sessões

- access token curto;
- refresh token rotativo;
- revogação por dispositivo;
- cookie HttpOnly, Secure, SameSite;
- session binding por tenant;
- step-up authentication para ações críticas.

## 9.7 Ações críticas com step-up

- alterar MFA;
- criar API key;
- alterar SMTP;
- adicionar domínio;
- exportar base;
- excluir tenant;
- impersonar usuário;
- alterar billing;
- conceder papel administrativo.

---

# 10. RBAC

## 10.1 Papéis padrão do tenant

### Tenant Owner

- controle total do tenant;
- billing;
- domínios;
- usuários;
- exclusão;
- exportações.

### Tenant Admin

- usuários;
- campanhas;
- listas;
- templates;
- remetentes;
- relatórios;
- sem excluir tenant ou alterar ownership.

### Campaign Manager

- criar, editar, aprovar e agendar campanhas;
- administrar templates;
- visualizar métricas.

### Operator

- criar rascunhos;
- importar contatos;
- executar tarefas operacionais;
- sem publicar campanha sem aprovação.

### Analyst

- visualizar relatórios;
- exportar métricas autorizadas;
- sem alterar campanhas.

### Viewer

- somente leitura.

### Billing Manager

- plano;
- faturas;
- consumo;
- sem acesso a contatos.

## 10.2 Papéis da plataforma

- Platform Super Admin;
- Platform Operations;
- Platform Support;
- Platform Security;
- Platform Billing;
- Platform Auditor.

## 10.3 Permissões granulares

Formato:

```text
resource.action.scope
```

Exemplos:

```text
campaign.create.tenant
campaign.approve.tenant
subscriber.export.tenant
domain.manage.tenant
user.invite.tenant
audit.read.tenant
smtp.manage.tenant
```

## 10.4 Regras

- permissões são aditivas;
- negação explícita deve prevalecer;
- papéis customizados por tenant em plano premium;
- nenhuma permissão global pode ser criada por tenant;
- mudanças de papel invalidam sessões sensíveis.

---

# 11. Portal administrativo

## 11.1 Dashboard global

- tenants ativos;
- MRR/ARR;
- e-mails enviados;
- taxa de erro;
- filas;
- bounces;
- reclamações;
- incidentes;
- consumo de infraestrutura;
- reputação de domínios;
- falhas de webhooks.

## 11.2 Gestão de tenants

- criar;
- editar;
- suspender;
- reativar;
- migrar para dedicado;
- aplicar quota;
- ver auditoria;
- redefinir owner;
- iniciar offboarding.

## 11.3 Impersonation

Permitida apenas para suporte autorizado.

Requisitos:

- justificativa obrigatória;
- expiração curta;
- banner visível;
- MFA recente;
- aprovação opcional;
- log imutável;
- proibição de visualizar segredos;
- proibição de alterar billing sem privilégio específico.

---

# 12. Portal do cliente

## 12.1 Home

- volume enviado;
- campanhas ativas;
- contatos;
- bounces;
- consumo do plano;
- domínios pendentes;
- alertas de entregabilidade.

## 12.2 Campanhas

Estados:

```text
draft -> review -> approved -> scheduled -> sending -> completed
                   \-> rejected
```

Controles:

- teste de envio;
- preview desktop/mobile;
- spam-check opcional;
- validação de links;
- aprovação de campanha;
- agendamento;
- cancelamento seguro;
- idempotência.

## 12.3 Contatos

- importação CSV;
- deduplicação por tenant;
- campos customizados;
- consentimento;
- tags;
- listas;
- supressão;
- exportação controlada.

## 12.4 Domínios

Wizard com:

- domínio;
- finalidade;
- registros DNS;
- verificação;
- SPF;
- DKIM;
- DMARC;
- CNAME de tracking;
- teste final.

---

# 13. Segurança de dados

## 13.1 Criptografia

Em trânsito:

- TLS 1.2 mínimo;
- TLS 1.3 preferencial;
- HSTS;
- mTLS entre serviços críticos em ambientes maduros.

Em repouso:

- criptografia de disco;
- segredos TOTP criptografados em nível de campo;
- chaves de API armazenadas apenas como hash;
- credenciais SMTP criptografadas;
- backups criptografados.

## 13.2 Chaves

- envelope encryption;
- chave mestra fora do banco;
- rotação programada;
- chave por tenant no plano Enterprise;
- versionamento de chave.

## 13.3 Dados sensíveis

Classificação mínima:

- PII;
- credenciais;
- segredos;
- conteúdo de campanha;
- dados de billing;
- logs de auditoria.

---

# 14. Proteções de aplicação

## 14.1 OWASP

Controles obrigatórios para:

- Broken Access Control;
- IDOR;
- Injection;
- XSS;
- CSRF;
- SSRF;
- mass assignment;
- upload inseguro;
- open redirect;
- secrets exposure;
- deserialização insegura.

## 14.2 Validação

- validação server-side;
- DTOs explícitos;
- allowlist;
- sanitização de HTML;
- bloqueio de JavaScript em templates;
- validação de URL;
- limite de tamanho;
- MIME sniffing desabilitado.

## 14.3 Rate limiting

Dimensões:

- IP;
- usuário;
- tenant;
- endpoint;
- API key;
- campanha;
- provedor SMTP.

---

# 15. E-mail e entregabilidade

## 15.1 Perfis SMTP

Cada tenant pode ter múltiplos perfis:

- SMTP próprio;
- Amazon SES;
- Mailgun;
- SendGrid;
- Postmark;
- relay da plataforma.

## 15.2 Seleção de SMTP

Critérios:

- domínio remetente;
- tipo de campanha;
- plano;
- volume;
- região;
- reputação;
- failover.

## 15.3 Controles

- SPF;
- DKIM;
- DMARC;
- Return-Path;
- feedback loops;
- suppression list;
- hard bounce;
- soft bounce;
- complaint handling;
- warming;
- throttling;
- domínio de tracking.

## 15.4 Isolamento de reputação

Planos:

- shared pool;
- premium pool;
- dedicated IP;
- dedicated domain;
- dedicated SMTP.

---

# 16. Filas e jobs

## 16.1 Categorias

- campaign_send;
- transactional_send;
- import;
- export;
- dns_verify;
- webhook_delivery;
- bounce_process;
- report_generate;
- retention_cleanup.

## 16.2 Payload

Todo job contém:

```json
{
  "job_id": "uuid",
  "tenant_id": "uuid",
  "actor_id": "uuid",
  "type": "campaign_send",
  "resource_id": "uuid",
  "attempt": 1,
  "created_at": "timestamp",
  "signature": "hmac"
}
```

## 16.3 Regras

- idempotency key;
- dead-letter queue;
- retries exponenciais;
- limite por tenant;
- prioridade por plano;
- cancelamento;
- tracing distribuído.

---

# 17. Storage e mídia

## 17.1 Estrutura

```text
/{tenant_id}/templates/
/{tenant_id}/campaigns/
/{tenant_id}/imports/
/{tenant_id}/exports/
/{tenant_id}/branding/
```

## 17.2 Controles

- URLs assinadas;
- expiração;
- antivírus;
- validação MIME;
- limite por arquivo;
- quota por tenant;
- sem bucket público;
- retenção por categoria.

---

# 18. Auditoria

## 18.1 Eventos obrigatórios

- login e falha;
- MFA;
- recuperação;
- criação e exclusão de usuário;
- alteração de papel;
- criação de API key;
- exportação;
- importação;
- publicação de campanha;
- alteração de SMTP;
- alteração de domínio;
- impersonation;
- alteração de billing;
- exclusão de dados.

## 18.2 Campos

- event_id;
- timestamp;
- tenant_id;
- actor_id;
- actor_type;
- action;
- resource_type;
- resource_id;
- source_ip;
- user_agent;
- request_id;
- result;
- before_hash;
- after_hash;
- reason.

## 18.3 Integridade

- append-only;
- hash encadeado opcional;
- exportação para SIEM;
- retenção configurável;
- acesso restrito.

---

# 19. LGPD e governança

## 19.1 Capacidades

- registro de origem do contato;
- finalidade;
- base legal;
- data de consentimento;
- versão do termo;
- opt-out;
- supressão;
- retenção;
- exportação;
- exclusão;
- anonimização;
- trilha de auditoria.

## 19.2 Categorias de comunicação

- contratual;
- operacional;
- institucional;
- marketing;
- eventos;
- newsletter.

O opt-out de marketing não deve impedir comunicações contratuais legítimas.

---

# 20. API

## 20.1 Padrões

- REST versionada;
- OpenAPI;
- UUIDs externos;
- paginação cursor-based;
- idempotency keys;
- scopes;
- rate limit;
- request IDs;
- webhooks assinados.

## 20.2 API keys

- hash irreversível;
- prefixo identificável;
- scopes;
- expiração;
- rotação;
- último uso;
- IP allowlist opcional;
- tenant fixo.

## 20.3 Webhooks

Eventos:

- campaign.sent;
- campaign.completed;
- subscriber.created;
- subscriber.unsubscribed;
- bounce.received;
- complaint.received;
- domain.verified;

Segurança:

- HMAC;
- timestamp;
- proteção anti-replay;
- retries;
- DLQ;
- logs por tenant.

---

# 21. Docker

## 21.1 Containers mínimos

```text
mailview-proxy
mailview-frontend
mailview-api
mailview-worker
mailview-postgres
mailview-redis
mailview-minio
mailview-observability
```

## 21.2 Ambientes

- dev;
- test;
- staging;
- production.

## 21.3 Regras de imagem

- multi-stage build;
- imagem mínima;
- usuário não-root;
- filesystem read-only;
- healthcheck;
- SBOM;
- assinatura de imagem;
- scan de vulnerabilidade;
- pin de versão;
- sem `latest` em produção.

## 21.4 Exemplo conceitual de Compose

```yaml
services:
  proxy:
    image: traefik:v3.5
    read_only: true
    ports:
      - "80:80"
      - "443:443"
    networks: [edge, app]

  frontend:
    image: registry.local/mailview/frontend:${VERSION}
    read_only: true
    networks: [app]

  api:
    image: registry.local/mailview/api:${VERSION}
    read_only: true
    user: "10001:10001"
    environment:
      DATABASE_URL_FILE: /run/secrets/database_url
    secrets:
      - database_url
    networks: [app, data]

  worker:
    image: registry.local/mailview/worker:${VERSION}
    read_only: true
    user: "10001:10001"
    networks: [app, data]

  postgres:
    image: postgres:17.5
    volumes:
      - pgdata:/var/lib/postgresql/data
    networks: [data]

  redis:
    image: redis:8.0
    command: ["redis-server", "--appendonly", "yes"]
    networks: [data]

networks:
  edge:
  app:
  data:
    internal: true

secrets:
  database_url:
    file: ./secrets/database_url

volumes:
  pgdata:
```

---

# 22. CI/CD

## 22.1 Pipeline

1. lint;
2. unit tests;
3. tenant isolation tests;
4. SAST;
5. dependency scan;
6. secret scan;
7. build;
8. SBOM;
9. container scan;
10. DAST em staging;
11. migrations dry-run;
12. deploy canary;
13. smoke tests;
14. progressive rollout.

## 22.2 Gates

Bloquear release se houver:

- vulnerabilidade crítica;
- falha de RLS;
- regressão de tenant isolation;
- migration irreversível sem plano;
- quebra de backup restore;
- secrets detectados.

---

# 23. Testes

## 23.1 Testes de isolamento

Obrigatórios:

- Tenant A não lê assinantes do Tenant B.
- Tenant A não altera campanhas do Tenant B.
- Tenant A não acessa mídia do Tenant B.
- Tenant A não usa SMTP do Tenant B.
- Tenant A não consulta métricas do Tenant B.
- API key do Tenant A não opera no Tenant B.
- Job do Tenant A não processa recurso do Tenant B.
- Exportação do Tenant A não contém dados do Tenant B.
- domínio do Tenant A não pode ser reivindicado pelo Tenant B.

## 23.2 Testes de segurança

- IDOR;
- bypass de host header;
- JWT confusion;
- replay TOTP;
- brute force;
- CSRF;
- XSS em templates;
- SSRF em webhooks;
- upload malicioso;
- SQL injection;
- mass assignment;
- privilege escalation;
- session fixation.

## 23.3 Performance

- 1 milhão de contatos por tenant;
- imports concorrentes;
- campanhas concorrentes;
- picos de webhooks;
- fila degradada;
- failover SMTP;
- restore de banco.

---

# 24. Observabilidade

## 24.1 Logs

JSON estruturado com:

- timestamp;
- level;
- service;
- environment;
- tenant_id;
- user_id;
- request_id;
- trace_id;
- action;
- result.

## 24.2 Métricas

- requests;
- latency;
- errors;
- queue depth;
- send rate;
- bounce rate;
- complaint rate;
- DB pool;
- RLS violations;
- auth failures;
- MFA failures;
- tenant quota usage.

## 24.3 Alertas

- bounce rate anormal;
- complaint spike;
- fila presa;
- falha de DNS;
- certificado próximo do vencimento;
- erro de backup;
- falha de RLS;
- aumento de 401/403;
- SMTP bloqueado.

---

# 25. Backup e continuidade

## 25.1 Requisitos

- backup diário completo;
- WAL contínuo;
- PITR;
- backup de storage;
- criptografia;
- teste de restore mensal;
- retenção por plano;
- cópia off-site.

## 25.2 Metas iniciais

- RPO padrão: 15 minutos;
- RTO padrão: 4 horas;
- Enterprise: RPO 5 minutos, RTO 1 hora.

---

# 26. Billing e quotas

## 26.1 Dimensões

- contatos ativos;
- e-mails enviados;
- domínios;
- usuários;
- armazenamento;
- retenção;
- SMTP dedicado;
- IP dedicado;
- suporte.

## 26.2 Enforcement

Quotas devem ser aplicadas no backend, nunca apenas na interface.

Estados:

- normal;
- warning;
- soft limit;
- hard limit;
- suspended.

---

# 27. Planos sugeridos

## Starter

- 10 mil contatos;
- 100 mil envios/mês;
- 3 usuários;
- 1 domínio;
- pool compartilhado.

## Professional

- 100 mil contatos;
- 1 milhão de envios/mês;
- 15 usuários;
- 5 domínios;
- RBAC customizado;
- webhooks.

## Business

- 500 mil contatos;
- 5 milhões de envios/mês;
- 50 usuários;
- domínios ilimitados sob política;
- aprovação de campanhas;
- retenção ampliada.

## Enterprise

- infraestrutura dedicada;
- SSO;
- chaves dedicadas;
- SLA;
- IP dedicado;
- banco dedicado;
- suporte premium.

---

# 28. Threat model resumido

## Ameaças prioritárias

1. vazamento cross-tenant;
2. takeover de domínio;
3. privilege escalation;
4. abuso de SMTP;
5. comprometimento de conta administrativa;
6. exportação indevida;
7. injeção em templates;
8. SSRF por webhooks;
9. vazamento de credenciais SMTP;
10. supply-chain compromise.

## Controles principais

- RLS;
- MFA;
- RBAC;
- rate limiting;
- audit log;
- assinatura de imagens;
- secrets manager;
- DNS verification;
- content sanitization;
- egress control.

---

# 29. Roadmap de desenvolvimento

## Fase 0: Fundação

- fork;
- ADRs;
- CI/CD;
- Docker;
- threat model;
- baseline de segurança.

## Fase 1: Control Plane

- tenants;
- usuários;
- memberships;
- RBAC;
- login;
- MFA;
- auditoria.

## Fase 2: Tenant isolation

- `tenant_id`;
- migrations;
- RLS;
- middleware;
- testes cross-tenant.

## Fase 3: Portal do cliente

- dashboard;
- listas;
- contatos;
- campanhas;
- templates.

## Fase 4: Domínios e SMTP

- múltiplos domínios;
- DNS verification;
- certificados;
- tracking;
- perfis SMTP.

## Fase 5: SaaS comercial

- billing;
- quotas;
- planos;
- onboarding;
- trial;
- suspensão.

## Fase 6: Enterprise

- SSO;
- tenant dedicado;
- IP dedicado;
- SLA;
- exportação SIEM;
- chaves dedicadas.

---

# 30. Definition of Done

Uma funcionalidade só está pronta quando:

- possui testes unitários;
- possui teste de autorização;
- possui teste cross-tenant;
- possui logs;
- possui métricas;
- possui auditoria quando aplicável;
- possui documentação API;
- possui migration segura;
- não contém segredo;
- passou em SAST e container scan;
- possui rollback;
- foi validada em staging.

---

# 31. ADRs iniciais

## ADR-001

**Decisão:** PostgreSQL compartilhado com RLS para plano padrão.

## ADR-002

**Decisão:** opção de tenant dedicado no plano Enterprise.

## ADR-003

**Decisão:** TOTP como MFA inicial, WebAuthn em roadmap.

## ADR-004

**Decisão:** subdomínio padrão mais domínio personalizado verificado.

## ADR-005

**Decisão:** frontend separado do backend, mesmo que o Listmonk original use assets embutidos.

## ADR-006

**Decisão:** nenhum acesso ao banco fora do contexto de tenant.

## ADR-007

**Decisão:** API keys com scopes e tenant fixo.

## ADR-008

**Decisão:** imagens Docker assinadas e sem tag `latest` em produção.

---

# 32. Riscos estratégicos

## 32.1 Divergência do upstream

Quanto mais o core for alterado, maior o custo de atualização.

Mitigação:

- modularização;
- sincronização frequente;
- patch set pequeno;
- testes automatizados.

## 32.2 AGPLv3

Modificações no software coberto e oferecido em rede podem criar obrigação de disponibilização do código-fonte correspondente. A estrutura societária, comercial e de licenciamento deve ser revisada por especialista jurídico.

## 32.3 Entregabilidade

A experiência do cliente pode degradar mesmo com aplicação saudável caso domínio, IP ou SMTP tenham reputação ruim.

## 32.4 Complexidade operacional

Multi-tenancy, domínios customizados, SMTPs e billing criam uma plataforma maior do que o Listmonk original.

---

# 33. Decisão recomendada

O MailView não deve ser implementado como uma coleção de condicionais `if tenant` inseridas no Listmonk. A solução deve ter:

- contexto de tenant central;
- enforcement no banco;
- control plane;
- RBAC próprio;
- identity layer;
- MFA;
- domínios verificados;
- filas isoladas logicamente;
- observabilidade por tenant;
- plano dedicado para clientes de maior risco.

A primeira entrega comercial deve priorizar isolamento, autenticação, domínios e confiabilidade. Recursos sofisticados de marketing podem vir depois. Um SaaS de mailing pode sobreviver com menos automações; não sobrevive a um único vazamento entre clientes.

---

# 34. Referências técnicas

- Repositório oficial Listmonk: https://github.com/knadh/listmonk
- Documentação: https://listmonk.app/docs/
- Instalação: https://listmonk.app/docs/installation/
- Developer setup: https://listmonk.app/docs/developer-setup/
- Releases: https://github.com/knadh/listmonk/releases
- Licença AGPLv3: https://www.gnu.org/licenses/agpl-3.0.html
- RFC 6238 TOTP: https://www.rfc-editor.org/rfc/rfc6238
- OWASP ASVS: https://owasp.org/www-project-application-security-verification-standard/
- OWASP SaaS Security: https://owasp.org/


