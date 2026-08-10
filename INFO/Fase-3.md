# Fase 3 — implementação

Status em `feature/sprint03`: implementada. Esta fase acrescenta às fundações da Fase 2:

- migrations 11/12 com o catálogo completo de entidades `mv_*`, `tenant_id`, índices tenant-first, FKs compostas e `FORCE RLS` estrito nas 17 entidades nativas tenant-owned;
- envelope HMAC versionado que assina `tenant_id`, `job_id`, tipo, validade e hash do payload; o worker de import só inicia por esse envelope;
- logs de requisição com `tenant_id`, `user_id` e `request_id`, sem confiar em campos editáveis do corpo;
- workflow de slug com nomes reservados, unicidade global, histórico auditado e redirecionamento HTTP 308 por 1–365 dias;
- hostname validado sem IP, wildcard, `.local` ou labels inválidos; unicidade global evita takeover;
- desafio TXT/CNAME gerado pelo servidor, consulta DNS real, ativação somente após confirmação e revalidação automática (24h por padrão);
- perda de propriedade DNS retira o host do roteamento e marca o certificado como revogado; o controlador TLS possui endpoint próprio;
- contrato Enterprise dedicado completo no Control Plane: referências independentes de database, worker, SMTP, storage, chave e namespace Docker, com versão de roteamento e ativação atômica;
- migrations e isolamento validados em PostgreSQL 17 com role `NOBYPASSRLS` e dois tenants.

As credenciais dos recursos dedicados não são persistidas: somente referências para o secret manager/provisionador. O provisionamento físico continua sendo responsabilidade da infraestrutura, enquanto o Control Plane é a fonte autoritativa de roteamento.

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
