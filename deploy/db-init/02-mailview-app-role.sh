#!/bin/sh
set -eu

app_password="$(cat "$MAILVIEW_APP_DB_PASSWORD_FILE")"

psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set=app_password="$app_password" <<-'EOSQL'
SELECT format('CREATE ROLE mailview_app LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE', :'app_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mailview_app') \gexec
SELECT format('ALTER ROLE mailview_app PASSWORD %L NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE', :'app_password') \gexec
GRANT CONNECT ON DATABASE mailview TO mailview_app;
GRANT USAGE, CREATE ON SCHEMA public TO mailview_app;
GRANT ALL ON ALL TABLES IN SCHEMA public TO mailview_app;
GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO mailview_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO mailview_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO mailview_app;
EOSQL
