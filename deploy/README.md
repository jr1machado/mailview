# Deploy de produção do MailView v0.5.0

A topologia fornecida executa Caddy (80/443), o binário monolítico MailView (9000 interno) e PostgreSQL (5432 interno). Frontend e workers estão no binário; não há Redis.

```sh
cp .env.example .env
mkdir -p secrets && chmod 700 secrets
printf '%s' 'mailview_admin' > secrets/postgres-user
openssl rand -base64 36 > secrets/postgres-password
openssl rand -base64 36 > secrets/app-db-password
openssl rand -base64 32 > secrets/mfa-encryption-key
openssl rand -base64 32 > secrets/import-signing-key
chmod 600 secrets/*
docker compose --env-file .env -f compose.production.yml config
docker compose --env-file .env -f compose.production.yml up -d
```

Edite `.env`: `MAILVIEW_IMAGE` deve ser imagem MailView com tag imutável e `MAILVIEW_PUBLIC_HOST` deve possuir DNS apontando para o proxy. O Caddy emite TLS do host principal. Domínios de tenant adicionais exigem configuração operacional de DNS/proxy; o MailView automatiza a consulta DNS de propriedade, mas o provisionamento do registro e do certificado no proxy continua externo.

Na primeira criação do volume, `db-init/02-mailview-app-role.sh` cria `mailview_app` como `NOBYPASSRLS`. Alterar secrets depois não executa novamente o init; rotacione a senha também no PostgreSQL.

Configure em Settings o provider de mídia como filesystem `/mailview/uploads` ou S3. Faça backup dos volumes `postgres-data`, `media-data`, `import-data` e `caddy-data`. O Compose é baseline de segurança, não HA.

O prefixo `LISTMONK_*` dentro do Compose é compatibilidade do loader, não nome de pacote ou release.
