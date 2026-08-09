-- Runs only on first init of a fresh postgres data volume (docker-entrypoint-initdb.d).
-- POSTGRES_USER is the Postgres superuser used for installation/migrations;
-- MailView must never run with a superuser/BYPASSRLS connection (see
-- INFO/Fase-3.md 6.5), so the application connects as this restricted role
-- instead. It is not owner of any table, so FORCE ROW LEVEL SECURITY applies
-- to it normally.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'listmonk_app') THEN
        CREATE ROLE listmonk_app LOGIN PASSWORD 'listmonk_app' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
END
$$;

GRANT ALL PRIVILEGES ON DATABASE listmonk TO listmonk_app;
GRANT USAGE, CREATE ON SCHEMA public TO listmonk_app;
GRANT ALL ON ALL TABLES IN SCHEMA public TO listmonk_app;
GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO listmonk_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO listmonk_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO listmonk_app;
