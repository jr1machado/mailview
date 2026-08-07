# Fase 1 — Control Plane, identidade e auditoria

## Entregue

- Executor de migrations MailView independente do `migList` upstream, com ledger e advisory lock.
- `mv_tenants`, `mv_memberships`, papéis e permissões SaaS, métodos MFA, recovery codes e auditoria append-only.
- Criação de tenant cria os sete papéis padrão e associa o owner como `Tenant Owner` em uma única transação.
- API administrativa protegida pelo Super Admin atual do Listmonk, como ponte temporária para o papel de plataforma.
- Novos segredos TOTP cifrados com AES-256-GCM; recovery codes exibidos uma vez e guardados como bcrypt hashes.
- Testes unitários e de integração opt-in com PostgreSQL.

## API administrativa

Todos os endpoints requerem uma sessão do Super Admin atual:

| Método | Rota | Função |
|---|---|---|
| `GET` | `/api/mailview/tenants` | Lista tenants |
| `POST` | `/api/mailview/tenants` | Cria tenant e owner |
| `GET/PATCH` | `/api/mailview/tenants/:tenantID` | Consulta ou altera status |
| `GET` | `/api/mailview/tenants/:tenantID/roles` | Lista papéis padrão |
| `GET/POST` | `/api/mailview/tenants/:tenantID/memberships` | Lista ou cria membership |
| `PUT` | `/api/mailview/tenants/:tenantID/memberships/:membershipID/roles` | Substitui papéis |
| `GET` | `/api/mailview/tenants/:tenantID/audit-events` | Lê auditoria do tenant |
| `POST` | `/api/mailview/profile/mfa/recovery-codes` | Gera 10 recovery codes para o usuário autenticado com TOTP |

Exemplo de criação:

```json
{
  "slug": "acme-email",
  "name": "Acme Email",
  "owner_user_id": 1
}
```

## Operação de MFA

Defina uma chave aleatória de 32 bytes codificada em base64 antes de novas inscrições TOTP:

```text
LISTMONK_mailview__mfa_encryption_key_FILE=/run/secrets/mailview_mfa_key
```

O arquivo contém somente a chave em base64. Sem ela, novas inscrições TOTP são recusadas em vez de gravar o segredo em texto claro. Contas TOTP anteriores continuam legíveis durante a migração e devem ser reenroladas. Na tela de segundo fator, informe um código de recuperação no lugar do TOTP para consumi-lo uma única vez.

## Aplicação das migrations

O processo principal executa migrations MailView pendentes no boot. Em uma atualização operacional, rode também o upgrade normal:

```sh
./listmonk --upgrade --yes --config config.toml
```

## Limites intencionais

Ainda não há `tenant_id` nas tabelas de mailing nem RLS: isso é a Fase 2. Os papéis desta fase controlam o Control Plane e não concedem acesso às rotas globais existentes do Listmonk. O portal visual do cliente, domínios e SMTP por tenant entram nas fases posteriores.
