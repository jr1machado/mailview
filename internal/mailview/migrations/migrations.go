// Package migrations owns schema changes introduced by MailView. It is kept
// separate from Listmonk's migration registry so upstream upgrades remain
// independently reviewable.
package migrations

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type migration struct {
	version int
	name    string
	sql     string
}

var all = []migration{
	{
		version: 1,
		name:    "control_plane",
		sql: `
CREATE TABLE IF NOT EXISTS mv_tenants (
    id UUID PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$'),
    name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 160),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'pending', 'offboarded')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mv_roles (
    id UUID PRIMARY KEY,
    tenant_id UUID NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
    scope TEXT NOT NULL CHECK (scope IN ('tenant', 'platform')),
    name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 100),
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((scope = 'platform' AND tenant_id IS NULL) OR (scope = 'tenant' AND tenant_id IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS mv_roles_tenant_name_idx
    ON mv_roles (tenant_id, lower(name)) WHERE tenant_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS mv_roles_platform_name_idx
    ON mv_roles (lower(name)) WHERE tenant_id IS NULL;

CREATE TABLE IF NOT EXISTS mv_permissions (
    code TEXT PRIMARY KEY CHECK (code ~ '^[a-z]+(\.[a-z_]+){2}$'),
    description TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS mv_role_permissions (
    role_id UUID NOT NULL REFERENCES mv_roles(id) ON DELETE CASCADE,
    permission_code TEXT NOT NULL REFERENCES mv_permissions(code) ON DELETE RESTRICT,
    PRIMARY KEY (role_id, permission_code)
);

CREATE TABLE IF NOT EXISTS mv_memberships (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE RESTRICT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'invited', 'suspended', 'removed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id)
);
CREATE INDEX IF NOT EXISTS mv_memberships_user_idx ON mv_memberships (user_id, status);

CREATE TABLE IF NOT EXISTS mv_membership_roles (
    membership_id UUID NOT NULL REFERENCES mv_memberships(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES mv_roles(id) ON DELETE RESTRICT,
    PRIMARY KEY (membership_id, role_id)
);

CREATE TABLE IF NOT EXISTS mv_mfa_methods (
    id UUID PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('totp')),
    secret_ciphertext BYTEA NOT NULL,
    key_version SMALLINT NOT NULL DEFAULT 1,
    enabled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ NULL,
    UNIQUE (user_id, type)
);

CREATE TABLE IF NOT EXISTS mv_mfa_recovery_codes (
    id UUID PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    used_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mv_mfa_recovery_codes_user_idx
    ON mv_mfa_recovery_codes (user_id) WHERE used_at IS NULL;

CREATE TABLE IF NOT EXISTS mv_audit_events (
    id UUID PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    tenant_id UUID NULL REFERENCES mv_tenants(id) ON DELETE RESTRICT,
    actor_user_id INTEGER NULL REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    source_ip INET NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    result TEXT NOT NULL CHECK (result IN ('success', 'failure')),
    reason TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS mv_audit_events_tenant_time_idx ON mv_audit_events (tenant_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS mv_audit_events_actor_time_idx ON mv_audit_events (actor_user_id, occurred_at DESC);

CREATE OR REPLACE FUNCTION mv_reject_audit_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'mv_audit_events is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS mv_audit_events_immutable ON mv_audit_events;
CREATE TRIGGER mv_audit_events_immutable
    BEFORE UPDATE OR DELETE ON mv_audit_events
    FOR EACH ROW EXECUTE FUNCTION mv_reject_audit_mutation();
`,
	},
	{
		version: 2,
		name:    "tenant_transaction_boundary",
		sql: `
CREATE TABLE IF NOT EXISTS mv_tenant_settings (
    tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
    key TEXT NOT NULL CHECK (key ~ '^[a-z][a-z0-9_.-]{0,127}$'),
    value JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, key)
);

ALTER TABLE mv_tenant_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE mv_tenant_settings FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS mv_tenant_settings_isolation ON mv_tenant_settings;
CREATE POLICY mv_tenant_settings_isolation ON mv_tenant_settings
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
`,
	},
	{
		version: 3,
		name:    "contacts_and_lists_tenant_keys",
		sql: `
INSERT INTO mv_tenants (id, slug, name, status)
VALUES ('00000000-0000-0000-0000-000000000001', 'legacy-workspace', 'Legacy workspace', 'pending')
ON CONFLICT (id) DO NOTHING;

ALTER TABLE subscribers ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE lists ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE subscriber_lists ADD COLUMN IF NOT EXISTS tenant_id UUID;
UPDATE subscribers SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE lists SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE subscriber_lists sl SET tenant_id = s.tenant_id FROM subscribers s WHERE sl.subscriber_id = s.id AND sl.tenant_id IS NULL;
ALTER TABLE subscribers ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE lists ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE subscriber_lists ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE subscribers ADD CONSTRAINT mv_subscribers_tenant_fk FOREIGN KEY (tenant_id) REFERENCES mv_tenants(id) ON DELETE RESTRICT;
ALTER TABLE lists ADD CONSTRAINT mv_lists_tenant_fk FOREIGN KEY (tenant_id) REFERENCES mv_tenants(id) ON DELETE RESTRICT;
ALTER TABLE subscriber_lists ADD CONSTRAINT mv_subscriber_lists_tenant_fk FOREIGN KEY (tenant_id) REFERENCES mv_tenants(id) ON DELETE RESTRICT;

ALTER TABLE subscribers DROP CONSTRAINT IF EXISTS subscribers_email_key;
DROP INDEX IF EXISTS idx_subs_email;
CREATE UNIQUE INDEX IF NOT EXISTS mv_subscribers_tenant_email_idx ON subscribers (tenant_id, lower(email));
CREATE UNIQUE INDEX IF NOT EXISTS mv_subscribers_tenant_id_idx ON subscribers (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS mv_lists_tenant_id_idx ON lists (tenant_id, id);
CREATE INDEX IF NOT EXISTS mv_subscribers_tenant_status_idx ON subscribers (tenant_id, status, id);
CREATE INDEX IF NOT EXISTS mv_lists_tenant_status_idx ON lists (tenant_id, status, id);
CREATE INDEX IF NOT EXISTS mv_subscriber_lists_tenant_list_idx ON subscriber_lists (tenant_id, list_id, subscriber_id);
ALTER TABLE subscriber_lists ADD CONSTRAINT mv_subscriber_lists_subscriber_tenant_fk FOREIGN KEY (tenant_id, subscriber_id) REFERENCES subscribers (tenant_id, id) ON DELETE CASCADE;
ALTER TABLE subscriber_lists ADD CONSTRAINT mv_subscriber_lists_list_tenant_fk FOREIGN KEY (tenant_id, list_id) REFERENCES lists (tenant_id, id) ON DELETE CASCADE;

DROP POLICY IF EXISTS mv_subscribers_isolation ON subscribers;
DROP POLICY IF EXISTS mv_lists_isolation ON lists;
DROP POLICY IF EXISTS mv_subscriber_lists_isolation ON subscriber_lists;
CREATE POLICY mv_subscribers_isolation ON subscribers USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
CREATE POLICY mv_lists_isolation ON lists USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
CREATE POLICY mv_subscriber_lists_isolation ON subscriber_lists USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
`,
	},
	{
		version: 4,
		name:    "tenant_scoped_import_jobs",
		sql: `
CREATE TABLE IF NOT EXISTS mv_import_jobs (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE RESTRICT,
    actor_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    idempotency_key TEXT NOT NULL CHECK (length(trim(idempotency_key)) BETWEEN 1 AND 200),
    list_ids INTEGER[] NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'cancelled')),
    total_rows INTEGER NOT NULL DEFAULT 0,
    processed_rows INTEGER NOT NULL DEFAULT 0,
    imported_rows INTEGER NOT NULL DEFAULT 0,
    error_rows INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, idempotency_key)
);
CREATE UNIQUE INDEX IF NOT EXISTS mv_import_jobs_tenant_id_idx ON mv_import_jobs (tenant_id, id);
CREATE INDEX IF NOT EXISTS mv_import_jobs_tenant_status_idx ON mv_import_jobs (tenant_id, status, id);

ALTER TABLE mv_import_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE mv_import_jobs FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS mv_import_jobs_isolation ON mv_import_jobs;
CREATE POLICY mv_import_jobs_isolation ON mv_import_jobs
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

CREATE TABLE IF NOT EXISTS mv_import_files (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    job_id UUID NOT NULL,
    storage_key TEXT NOT NULL,
    signature TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, job_id) REFERENCES mv_import_jobs (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS mv_import_files_job_idx ON mv_import_files (tenant_id, job_id);

ALTER TABLE mv_import_files ENABLE ROW LEVEL SECURITY;
ALTER TABLE mv_import_files FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS mv_import_files_isolation ON mv_import_files;
CREATE POLICY mv_import_files_isolation ON mv_import_files
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
`,
	},
}

// Upgrade applies pending MailView migrations atomically. It is safe to call
// on every start; an advisory lock prevents concurrent application.
func Upgrade(ctx context.Context, db *sqlx.DB) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(77420101)`); err != nil {
		return fmt.Errorf("lock mailview migrations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS mv_schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return fmt.Errorf("create mailview migration ledger: %w", err)
	}

	for _, m := range all {
		var applied bool
		if err := tx.GetContext(ctx, &applied, `SELECT EXISTS(SELECT 1 FROM mv_schema_migrations WHERE version = $1)`, m.version); err != nil {
			return fmt.Errorf("check mailview migration %d: %w", m.version, err)
		}
		if applied {
			continue
		}
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			return fmt.Errorf("apply mailview migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO mv_schema_migrations (version, name) VALUES ($1, $2)`, m.version, m.name); err != nil {
			return fmt.Errorf("record mailview migration %d: %w", m.version, err)
		}
	}

	return tx.Commit()
}
