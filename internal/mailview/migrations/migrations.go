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
	{
		version: 5,
		name:    "platform_rbac",
		sql: `
INSERT INTO mv_permissions (code, description) VALUES
    ('tenant.manage.platform', 'Create, edit and offboard tenants'),
    ('tenant.suspend.platform', 'Suspend and reactivate tenants'),
    ('membership.manage.platform', 'Manage memberships and roles across tenants'),
    ('billing.manage.platform', 'Manage plans, quotas and billing across tenants'),
    ('audit.read.platform', 'Read audit events across tenants'),
    ('support.impersonate.platform', 'Impersonate a tenant member for support'),
    ('security.manage.platform', 'Manage platform security configuration'),
    ('platform.roles.manage', 'Assign and revoke platform roles')
ON CONFLICT (code) DO NOTHING;

INSERT INTO mv_roles (id, tenant_id, scope, name, is_system) VALUES
    ('00000000-0000-0000-0000-0000000000f1', NULL, 'platform', 'Platform Super Admin', true),
    ('00000000-0000-0000-0000-0000000000f2', NULL, 'platform', 'Platform Operations', true),
    ('00000000-0000-0000-0000-0000000000f3', NULL, 'platform', 'Platform Support', true),
    ('00000000-0000-0000-0000-0000000000f4', NULL, 'platform', 'Platform Security', true),
    ('00000000-0000-0000-0000-0000000000f5', NULL, 'platform', 'Platform Billing', true),
    ('00000000-0000-0000-0000-0000000000f6', NULL, 'platform', 'Platform Auditor', true)
ON CONFLICT DO NOTHING;

INSERT INTO mv_role_permissions (role_id, permission_code) VALUES
    ('00000000-0000-0000-0000-0000000000f1', 'tenant.manage.platform'),
    ('00000000-0000-0000-0000-0000000000f1', 'tenant.suspend.platform'),
    ('00000000-0000-0000-0000-0000000000f1', 'membership.manage.platform'),
    ('00000000-0000-0000-0000-0000000000f1', 'billing.manage.platform'),
    ('00000000-0000-0000-0000-0000000000f1', 'audit.read.platform'),
    ('00000000-0000-0000-0000-0000000000f1', 'support.impersonate.platform'),
    ('00000000-0000-0000-0000-0000000000f1', 'security.manage.platform'),
    ('00000000-0000-0000-0000-0000000000f1', 'platform.roles.manage'),
    ('00000000-0000-0000-0000-0000000000f2', 'tenant.manage.platform'),
    ('00000000-0000-0000-0000-0000000000f2', 'tenant.suspend.platform'),
    ('00000000-0000-0000-0000-0000000000f2', 'membership.manage.platform'),
    ('00000000-0000-0000-0000-0000000000f3', 'support.impersonate.platform'),
    ('00000000-0000-0000-0000-0000000000f3', 'membership.manage.platform'),
    ('00000000-0000-0000-0000-0000000000f4', 'security.manage.platform'),
    ('00000000-0000-0000-0000-0000000000f4', 'audit.read.platform'),
    ('00000000-0000-0000-0000-0000000000f5', 'billing.manage.platform'),
    ('00000000-0000-0000-0000-0000000000f6', 'audit.read.platform')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS mv_platform_role_assignments (
    id UUID PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES mv_roles(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, role_id)
);
CREATE INDEX IF NOT EXISTS mv_platform_role_assignments_user_idx ON mv_platform_role_assignments (user_id);
`,
	},
	{
		version: 6,
		name:    "tenant_domains_plans_quotas",
		sql: `
CREATE TABLE IF NOT EXISTS mv_tenant_domains (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
    hostname TEXT NOT NULL CHECK (hostname = lower(hostname) AND length(hostname) BETWEEN 3 AND 255),
    purpose TEXT NOT NULL CHECK (purpose IN ('portal', 'tracking', 'sending', 'return_path', 'public_forms')),
    verification_method TEXT NOT NULL DEFAULT 'txt' CHECK (verification_method IN ('cname', 'txt')),
    verification_token TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'verified', 'failed', 'revoked')),
    tls_status TEXT NOT NULL DEFAULT 'none' CHECK (tls_status IN ('none', 'pending', 'issued', 'failed')),
    last_verified_at TIMESTAMPTZ,
    created_by INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Global uniqueness on hostname prevents the same host being claimed by two tenants (takeover protection).
CREATE UNIQUE INDEX IF NOT EXISTS mv_tenant_domains_hostname_idx ON mv_tenant_domains (hostname);
CREATE INDEX IF NOT EXISTS mv_tenant_domains_tenant_idx ON mv_tenant_domains (tenant_id, purpose);

ALTER TABLE mv_tenant_domains ENABLE ROW LEVEL SECURITY;
ALTER TABLE mv_tenant_domains FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS mv_tenant_domains_isolation ON mv_tenant_domains;
CREATE POLICY mv_tenant_domains_isolation ON mv_tenant_domains
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

CREATE TABLE IF NOT EXISTS mv_tenant_plans (
    code TEXT PRIMARY KEY CHECK (code ~ '^[a-z0-9_]{2,40}$'),
    name TEXT NOT NULL,
    max_subscribers INTEGER,
    max_emails_month INTEGER,
    max_domains INTEGER,
    max_seats INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO mv_tenant_plans (code, name, max_subscribers, max_emails_month, max_domains, max_seats) VALUES
    ('starter', 'Starter', 2000, 10000, 1, 3),
    ('growth', 'Growth', 25000, 150000, 3, 10),
    ('enterprise', 'Enterprise', NULL, NULL, NULL, NULL)
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS mv_tenant_quotas (
    tenant_id UUID PRIMARY KEY REFERENCES mv_tenants(id) ON DELETE CASCADE,
    plan_code TEXT NOT NULL REFERENCES mv_tenant_plans(code) ON DELETE RESTRICT DEFAULT 'starter',
    max_subscribers INTEGER,
    max_emails_month INTEGER,
    max_domains INTEGER,
    max_seats INTEGER,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mv_tenant_usage (
    tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
    period_start DATE NOT NULL,
    emails_sent BIGINT NOT NULL DEFAULT 0,
    subscribers_count INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, period_start)
);

-- Schema-only preparation: nullable, unenforced tenant_id columns for the
-- aggregates that have not migrated to tenant.Begin yet (campaigns,
-- templates, media). Wiring these into queries and activating RLS is a
-- separate rollout step, same sequencing already used for subscribers/lists.
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES mv_tenants(id) ON DELETE RESTRICT;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES mv_tenants(id) ON DELETE RESTRICT;
ALTER TABLE media ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES mv_tenants(id) ON DELETE RESTRICT;
CREATE INDEX IF NOT EXISTS mv_campaigns_tenant_idx ON campaigns (tenant_id, status);
CREATE INDEX IF NOT EXISTS mv_templates_tenant_idx ON templates (tenant_id);
CREATE INDEX IF NOT EXISTS mv_media_tenant_idx ON media (tenant_id);
`,
	},
	{
		version: 7,
		name:    "rbac_refinements_and_admin_ops",
		sql: `
-- Explicit denial (Fase-4.md 10.4: "negação explícita deve prevalecer").
-- A permission_code present here for a role always wins over the same code
-- granted through mv_role_permissions, regardless of insertion order.
CREATE TABLE IF NOT EXISTS mv_role_permission_denials (
    role_id UUID NOT NULL REFERENCES mv_roles(id) ON DELETE CASCADE,
    permission_code TEXT NOT NULL REFERENCES mv_permissions(code) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, permission_code)
);

-- Placeholder flag for the Enterprise "move to dedicated" workflow
-- (Fase-3.md 6.6 / Fase-4.md 11.2). Actually provisioning a dedicated
-- database/worker/SMTP/namespace is an infrastructure operation outside
-- this codebase; this table only records the requested mode so the Control
-- Plane and support tooling have a place to read it from.
CREATE TABLE IF NOT EXISTS mv_tenant_infrastructure (
    tenant_id UUID PRIMARY KEY REFERENCES mv_tenants(id) ON DELETE CASCADE,
    mode TEXT NOT NULL DEFAULT 'shared' CHECK (mode IN ('shared', 'dedicated_requested', 'dedicated')),
    requested_by INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Impersonation grants (Fase-4.md 11.3). A grant only widens access to the
-- MailView tenant data plane (subscribers/lists) for its ttl; it never
-- carries platform or billing permissions and is always time-boxed.
CREATE TABLE IF NOT EXISTS mv_impersonation_grants (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
    actor_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    target_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reason TEXT NOT NULL CHECK (length(trim(reason)) >= 10),
    approved_by INTEGER REFERENCES users(id) ON DELETE RESTRICT,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);
CREATE INDEX IF NOT EXISTS mv_impersonation_grants_actor_idx ON mv_impersonation_grants (actor_user_id, expires_at);
CREATE INDEX IF NOT EXISTS mv_impersonation_grants_tenant_idx ON mv_impersonation_grants (tenant_id, created_at);
`,
	},
	{
		version: 8,
		name:    "contacts_lists_rls_and_permissions",
		sql: `
-- Granular Data Plane permissions. Existing tenants are upgraded according
-- to their system-role semantics; custom roles receive no implicit grants.
INSERT INTO mv_permissions (code, description) VALUES
    ('subscriber.read.tenant', 'Read subscribers'),
    ('subscriber.import.tenant', 'Import subscribers'),
    ('list.read.tenant', 'Read lists'),
    ('list.manage.tenant', 'Manage lists')
ON CONFLICT (code) DO NOTHING;

INSERT INTO mv_role_permissions (role_id, permission_code)
SELECT r.id, p.code
FROM mv_roles r
CROSS JOIN (VALUES
    ('subscriber.read.tenant'), ('subscriber.manage.tenant'),
    ('subscriber.import.tenant'), ('subscriber.export.tenant'),
    ('list.read.tenant'), ('list.manage.tenant')
) AS p(code)
WHERE r.scope = 'tenant' AND r.is_system AND r.name IN ('Tenant Owner', 'Tenant Admin')
ON CONFLICT DO NOTHING;

INSERT INTO mv_role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM mv_roles r
CROSS JOIN (VALUES ('subscriber.read.tenant'), ('subscriber.manage.tenant'), ('subscriber.import.tenant'), ('list.read.tenant'), ('list.manage.tenant')) AS p(code)
WHERE r.scope = 'tenant' AND r.is_system AND r.name = 'Operator'
ON CONFLICT DO NOTHING;

INSERT INTO mv_role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM mv_roles r
CROSS JOIN (VALUES ('subscriber.read.tenant'), ('subscriber.export.tenant'), ('list.read.tenant')) AS p(code)
WHERE r.scope = 'tenant' AND r.is_system AND r.name = 'Analyst'
ON CONFLICT DO NOTHING;

INSERT INTO mv_role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM mv_roles r
CROSS JOIN (VALUES ('subscriber.read.tenant'), ('list.read.tenant')) AS p(code)
WHERE r.scope = 'tenant' AND r.is_system AND r.name = 'Viewer'
ON CONFLICT DO NOTHING;

-- Missing transaction context is deliberately restricted to the imported
-- legacy workspace. This preserves upstream jobs during the staged migration
-- without allowing a forgotten SET LOCAL to see any SaaS tenant.
DROP POLICY IF EXISTS mv_subscribers_isolation ON subscribers;
DROP POLICY IF EXISTS mv_lists_isolation ON lists;
DROP POLICY IF EXISTS mv_subscriber_lists_isolation ON subscriber_lists;
CREATE POLICY mv_subscribers_isolation ON subscribers
    USING (tenant_id = COALESCE(NULLIF(current_setting('app.tenant_id', true), '')::uuid, '00000000-0000-0000-0000-000000000001'::uuid))
    WITH CHECK (tenant_id = COALESCE(NULLIF(current_setting('app.tenant_id', true), '')::uuid, '00000000-0000-0000-0000-000000000001'::uuid));
CREATE POLICY mv_lists_isolation ON lists
    USING (tenant_id = COALESCE(NULLIF(current_setting('app.tenant_id', true), '')::uuid, '00000000-0000-0000-0000-000000000001'::uuid))
    WITH CHECK (tenant_id = COALESCE(NULLIF(current_setting('app.tenant_id', true), '')::uuid, '00000000-0000-0000-0000-000000000001'::uuid));
CREATE POLICY mv_subscriber_lists_isolation ON subscriber_lists
    USING (tenant_id = COALESCE(NULLIF(current_setting('app.tenant_id', true), '')::uuid, '00000000-0000-0000-0000-000000000001'::uuid))
    WITH CHECK (tenant_id = COALESCE(NULLIF(current_setting('app.tenant_id', true), '')::uuid, '00000000-0000-0000-0000-000000000001'::uuid));

ALTER TABLE subscribers ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscribers FORCE ROW LEVEL SECURITY;
ALTER TABLE lists ENABLE ROW LEVEL SECURITY;
ALTER TABLE lists FORCE ROW LEVEL SECURITY;
ALTER TABLE subscriber_lists ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscriber_lists FORCE ROW LEVEL SECURITY;
`,
	},
	{
		version: 9,
		name:    "mailing_aggregates_tenant_isolation",
		sql: `
INSERT INTO mv_permissions (code, description) VALUES
 ('campaign.manage.tenant','Manage campaigns'), ('campaign.send.tenant','Send campaigns'),
 ('analytics.read.tenant','Read analytics'), ('template.read.tenant','Read templates'),
 ('media.read.tenant','Read media'), ('media.manage.tenant','Manage media'),
 ('bounce.read.tenant','Read bounces'), ('bounce.manage.tenant','Manage bounces')
ON CONFLICT (code) DO NOTHING;

-- Upgrade existing system roles. Custom roles remain explicit.
INSERT INTO mv_role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM mv_roles r CROSS JOIN (VALUES
 ('campaign.manage.tenant'),('campaign.send.tenant'),('analytics.read.tenant'),('template.read.tenant'),
 ('media.read.tenant'),('media.manage.tenant'),('bounce.read.tenant'),('bounce.manage.tenant')) p(code)
WHERE r.scope='tenant' AND r.is_system AND r.name IN ('Tenant Owner','Tenant Admin') ON CONFLICT DO NOTHING;
INSERT INTO mv_role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM mv_roles r CROSS JOIN (VALUES
 ('campaign.manage.tenant'),('campaign.send.tenant'),('analytics.read.tenant'),('template.read.tenant'),
 ('media.read.tenant'),('media.manage.tenant'),('bounce.read.tenant')) p(code)
WHERE r.scope='tenant' AND r.is_system AND r.name='Campaign Manager' ON CONFLICT DO NOTHING;
INSERT INTO mv_role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM mv_roles r CROSS JOIN (VALUES
 ('campaign.manage.tenant'),('template.read.tenant'),('media.read.tenant'),('media.manage.tenant')) p(code)
WHERE r.scope='tenant' AND r.is_system AND r.name='Operator' ON CONFLICT DO NOTHING;
INSERT INTO mv_role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM mv_roles r CROSS JOIN (VALUES
 ('analytics.read.tenant'),('template.read.tenant'),('media.read.tenant'),('bounce.read.tenant')) p(code)
WHERE r.scope='tenant' AND r.is_system AND r.name IN ('Analyst','Viewer') ON CONFLICT DO NOTHING;

-- Upstream INSERT statements omit tenant_id; the transaction-local default
-- keeps those statements compatible while RLS validates the resulting row.
ALTER TABLE subscribers ALTER COLUMN tenant_id SET DEFAULT COALESCE(NULLIF(current_setting('app.tenant_id',true),'')::uuid,'00000000-0000-0000-0000-000000000001');
ALTER TABLE lists ALTER COLUMN tenant_id SET DEFAULT COALESCE(NULLIF(current_setting('app.tenant_id',true),'')::uuid,'00000000-0000-0000-0000-000000000001');
ALTER TABLE subscriber_lists ALTER COLUMN tenant_id SET DEFAULT COALESCE(NULLIF(current_setting('app.tenant_id',true),'')::uuid,'00000000-0000-0000-0000-000000000001');

ALTER TABLE templates ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE campaign_lists ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE campaign_views ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE media ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE campaign_media ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE links ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE link_clicks ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE bounces ADD COLUMN IF NOT EXISTS tenant_id UUID;

UPDATE templates SET tenant_id='00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE campaigns SET tenant_id='00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE campaign_lists cl SET tenant_id=c.tenant_id FROM campaigns c WHERE cl.campaign_id=c.id AND cl.tenant_id IS NULL;
UPDATE campaign_views cv SET tenant_id=c.tenant_id FROM campaigns c WHERE cv.campaign_id=c.id AND cv.tenant_id IS NULL;
UPDATE media SET tenant_id='00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE campaign_media cm SET tenant_id=c.tenant_id FROM campaigns c WHERE cm.campaign_id=c.id AND cm.tenant_id IS NULL;
UPDATE links SET tenant_id='00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE link_clicks SET tenant_id='00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;
UPDATE bounces b SET tenant_id=s.tenant_id FROM subscribers s WHERE b.subscriber_id=s.id AND b.tenant_id IS NULL;

ALTER TABLE templates ALTER COLUMN tenant_id SET DEFAULT COALESCE(NULLIF(current_setting('app.tenant_id',true),'')::uuid,'00000000-0000-0000-0000-000000000001');
ALTER TABLE campaigns ALTER COLUMN tenant_id SET DEFAULT COALESCE(NULLIF(current_setting('app.tenant_id',true),'')::uuid,'00000000-0000-0000-0000-000000000001');
ALTER TABLE campaign_lists ALTER COLUMN tenant_id SET DEFAULT COALESCE(NULLIF(current_setting('app.tenant_id',true),'')::uuid,'00000000-0000-0000-0000-000000000001');
ALTER TABLE campaign_views ALTER COLUMN tenant_id SET DEFAULT COALESCE(NULLIF(current_setting('app.tenant_id',true),'')::uuid,'00000000-0000-0000-0000-000000000001');
ALTER TABLE media ALTER COLUMN tenant_id SET DEFAULT COALESCE(NULLIF(current_setting('app.tenant_id',true),'')::uuid,'00000000-0000-0000-0000-000000000001');
ALTER TABLE campaign_media ALTER COLUMN tenant_id SET DEFAULT COALESCE(NULLIF(current_setting('app.tenant_id',true),'')::uuid,'00000000-0000-0000-0000-000000000001');
ALTER TABLE links ALTER COLUMN tenant_id SET DEFAULT COALESCE(NULLIF(current_setting('app.tenant_id',true),'')::uuid,'00000000-0000-0000-0000-000000000001');
ALTER TABLE link_clicks ALTER COLUMN tenant_id SET DEFAULT COALESCE(NULLIF(current_setting('app.tenant_id',true),'')::uuid,'00000000-0000-0000-0000-000000000001');
ALTER TABLE bounces ALTER COLUMN tenant_id SET DEFAULT COALESCE(NULLIF(current_setting('app.tenant_id',true),'')::uuid,'00000000-0000-0000-0000-000000000001');

ALTER TABLE templates ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE campaigns ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE campaign_lists ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE campaign_views ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE media ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE campaign_media ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE links ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE link_clicks ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE bounces ALTER COLUMN tenant_id SET NOT NULL;

ALTER TABLE templates ADD CONSTRAINT mv_templates_tenant_fk FOREIGN KEY(tenant_id) REFERENCES mv_tenants(id) ON DELETE RESTRICT;
ALTER TABLE campaigns ADD CONSTRAINT mv_campaigns_tenant_fk FOREIGN KEY(tenant_id) REFERENCES mv_tenants(id) ON DELETE RESTRICT;
ALTER TABLE campaign_lists ADD CONSTRAINT mv_campaign_lists_tenant_fk FOREIGN KEY(tenant_id) REFERENCES mv_tenants(id) ON DELETE RESTRICT;
ALTER TABLE campaign_views ADD CONSTRAINT mv_campaign_views_tenant_fk FOREIGN KEY(tenant_id) REFERENCES mv_tenants(id) ON DELETE RESTRICT;
ALTER TABLE media ADD CONSTRAINT mv_media_tenant_fk FOREIGN KEY(tenant_id) REFERENCES mv_tenants(id) ON DELETE RESTRICT;
ALTER TABLE campaign_media ADD CONSTRAINT mv_campaign_media_tenant_fk FOREIGN KEY(tenant_id) REFERENCES mv_tenants(id) ON DELETE RESTRICT;
ALTER TABLE links ADD CONSTRAINT mv_links_tenant_fk FOREIGN KEY(tenant_id) REFERENCES mv_tenants(id) ON DELETE RESTRICT;
ALTER TABLE link_clicks ADD CONSTRAINT mv_link_clicks_tenant_fk FOREIGN KEY(tenant_id) REFERENCES mv_tenants(id) ON DELETE RESTRICT;
ALTER TABLE bounces ADD CONSTRAINT mv_bounces_tenant_fk FOREIGN KEY(tenant_id) REFERENCES mv_tenants(id) ON DELETE RESTRICT;

CREATE UNIQUE INDEX IF NOT EXISTS mv_templates_tenant_id_idx ON templates(tenant_id,id);
CREATE UNIQUE INDEX IF NOT EXISTS mv_campaigns_tenant_id_idx ON campaigns(tenant_id,id);
CREATE UNIQUE INDEX IF NOT EXISTS mv_media_tenant_id_idx ON media(tenant_id,id);
CREATE UNIQUE INDEX IF NOT EXISTS mv_links_tenant_id_idx ON links(tenant_id,id);
ALTER TABLE campaigns DROP CONSTRAINT IF EXISTS campaigns_archive_slug_key;
CREATE UNIQUE INDEX IF NOT EXISTS mv_campaigns_tenant_archive_slug_idx ON campaigns(tenant_id,archive_slug) WHERE archive_slug IS NOT NULL;
DROP INDEX IF EXISTS templates_is_default_idx;
CREATE UNIQUE INDEX IF NOT EXISTS mv_templates_tenant_default_idx ON templates(tenant_id,is_default) WHERE is_default;
ALTER TABLE links DROP CONSTRAINT IF EXISTS links_url_key;
CREATE UNIQUE INDEX IF NOT EXISTS mv_links_tenant_url_idx ON links(tenant_id,url);

ALTER TABLE campaigns DROP CONSTRAINT IF EXISTS campaigns_template_id_fkey;
ALTER TABLE campaigns DROP CONSTRAINT IF EXISTS campaigns_archive_template_id_fkey;
ALTER TABLE campaigns ADD CONSTRAINT mv_campaign_template_tenant_fk FOREIGN KEY(tenant_id,template_id) REFERENCES templates(tenant_id,id) ON DELETE SET NULL (template_id);
ALTER TABLE campaigns ADD CONSTRAINT mv_campaign_archive_template_tenant_fk FOREIGN KEY(tenant_id,archive_template_id) REFERENCES templates(tenant_id,id) ON DELETE SET NULL (archive_template_id);
ALTER TABLE campaign_lists DROP CONSTRAINT IF EXISTS campaign_lists_campaign_id_fkey;
ALTER TABLE campaign_lists DROP CONSTRAINT IF EXISTS campaign_lists_list_id_fkey;
ALTER TABLE campaign_lists ADD CONSTRAINT mv_campaign_lists_campaign_tenant_fk FOREIGN KEY(tenant_id,campaign_id) REFERENCES campaigns(tenant_id,id) ON DELETE CASCADE;
ALTER TABLE campaign_lists ADD CONSTRAINT mv_campaign_lists_list_tenant_fk FOREIGN KEY(tenant_id,list_id) REFERENCES lists(tenant_id,id) ON DELETE SET NULL (list_id);
ALTER TABLE campaign_views DROP CONSTRAINT IF EXISTS campaign_views_campaign_id_fkey;
ALTER TABLE campaign_views DROP CONSTRAINT IF EXISTS campaign_views_subscriber_id_fkey;
ALTER TABLE campaign_views ADD CONSTRAINT mv_campaign_views_campaign_tenant_fk FOREIGN KEY(tenant_id,campaign_id) REFERENCES campaigns(tenant_id,id) ON DELETE CASCADE;
ALTER TABLE campaign_views ADD CONSTRAINT mv_campaign_views_subscriber_tenant_fk FOREIGN KEY(tenant_id,subscriber_id) REFERENCES subscribers(tenant_id,id) ON DELETE SET NULL (subscriber_id);
ALTER TABLE campaign_media DROP CONSTRAINT IF EXISTS campaign_media_campaign_id_fkey;
ALTER TABLE campaign_media DROP CONSTRAINT IF EXISTS campaign_media_media_id_fkey;
ALTER TABLE campaign_media ADD CONSTRAINT mv_campaign_media_campaign_tenant_fk FOREIGN KEY(tenant_id,campaign_id) REFERENCES campaigns(tenant_id,id) ON DELETE CASCADE;
ALTER TABLE campaign_media ADD CONSTRAINT mv_campaign_media_media_tenant_fk FOREIGN KEY(tenant_id,media_id) REFERENCES media(tenant_id,id) ON DELETE SET NULL (media_id);
ALTER TABLE link_clicks DROP CONSTRAINT IF EXISTS link_clicks_campaign_id_fkey;
ALTER TABLE link_clicks DROP CONSTRAINT IF EXISTS link_clicks_link_id_fkey;
ALTER TABLE link_clicks DROP CONSTRAINT IF EXISTS link_clicks_subscriber_id_fkey;
ALTER TABLE link_clicks ADD CONSTRAINT mv_click_campaign_tenant_fk FOREIGN KEY(tenant_id,campaign_id) REFERENCES campaigns(tenant_id,id) ON DELETE CASCADE;
ALTER TABLE link_clicks ADD CONSTRAINT mv_click_link_tenant_fk FOREIGN KEY(tenant_id,link_id) REFERENCES links(tenant_id,id) ON DELETE CASCADE;
ALTER TABLE link_clicks ADD CONSTRAINT mv_click_subscriber_tenant_fk FOREIGN KEY(tenant_id,subscriber_id) REFERENCES subscribers(tenant_id,id) ON DELETE SET NULL (subscriber_id);
ALTER TABLE bounces DROP CONSTRAINT IF EXISTS bounces_subscriber_id_fkey;
ALTER TABLE bounces DROP CONSTRAINT IF EXISTS bounces_campaign_id_fkey;
ALTER TABLE bounces ADD CONSTRAINT mv_bounce_subscriber_tenant_fk FOREIGN KEY(tenant_id,subscriber_id) REFERENCES subscribers(tenant_id,id) ON DELETE CASCADE;
ALTER TABLE bounces ADD CONSTRAINT mv_bounce_campaign_tenant_fk FOREIGN KEY(tenant_id,campaign_id) REFERENCES campaigns(tenant_id,id) ON DELETE SET NULL (campaign_id);

DO $mv$
DECLARE t text;
BEGIN
 FOREACH t IN ARRAY ARRAY['templates','campaigns','campaign_lists','campaign_views','media','campaign_media','links','link_clicks','bounces'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('DROP POLICY IF EXISTS mv_%s_isolation ON %I',t,t);
  EXECUTE format('CREATE POLICY mv_%s_isolation ON %I USING (tenant_id=COALESCE(NULLIF(current_setting(''app.tenant_id'',true),'''')::uuid,''00000000-0000-0000-0000-000000000001''::uuid)) WITH CHECK (tenant_id=COALESCE(NULLIF(current_setting(''app.tenant_id'',true),'''')::uuid,''00000000-0000-0000-0000-000000000001''::uuid))',t,t);
 END LOOP;
END $mv$;

CREATE OR REPLACE FUNCTION mv_campaign_tenant(p_id integer, p_uuid uuid) RETURNS uuid
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=public,pg_catalog AS $$
 SELECT tenant_id FROM campaigns WHERE (p_id>0 AND id=p_id) OR (p_uuid IS NOT NULL AND uuid=p_uuid) LIMIT 1
$$;
CREATE OR REPLACE FUNCTION mv_media_tenant(p_id integer) RETURNS uuid
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=public,pg_catalog AS $$ SELECT tenant_id FROM media WHERE id=p_id $$;
CREATE OR REPLACE FUNCTION mv_subscriber_tenant(p_id bigint) RETURNS uuid
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=public,pg_catalog AS $$ SELECT tenant_id FROM subscribers WHERE id=p_id $$;
CREATE OR REPLACE FUNCTION mv_bounce_tenant(p_sub_uuid uuid, p_email text, p_campaign_uuid uuid) RETURNS uuid
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=public,pg_catalog AS $$
 SELECT CASE WHEN count(DISTINCT tenant_id)=1 THEN min(tenant_id::text)::uuid ELSE NULL END FROM (
  SELECT tenant_id FROM campaigns WHERE p_campaign_uuid IS NOT NULL AND uuid=p_campaign_uuid
  UNION ALL SELECT tenant_id FROM subscribers WHERE p_sub_uuid IS NOT NULL AND uuid=p_sub_uuid
  UNION ALL SELECT tenant_id FROM subscribers WHERE p_sub_uuid IS NULL AND p_campaign_uuid IS NULL AND lower(email)=lower(p_email)
 ) resolved
$$;
`,
	},
	{
		version: 10,
		name:    "remove_rls_bypass_helpers",
		sql: `
-- These SECURITY DEFINER lookup helpers were introduced during the worker
-- migration but are no longer used. Tenant discovery now iterates explicit
-- RLS-scoped transactions, so retaining publicly executable owner-context
-- functions would create an unnecessary bypass primitive.
DROP FUNCTION IF EXISTS mv_campaign_tenant(integer, uuid);
DROP FUNCTION IF EXISTS mv_media_tenant(integer);
DROP FUNCTION IF EXISTS mv_subscriber_tenant(bigint);
DROP FUNCTION IF EXISTS mv_bounce_tenant(uuid, text, uuid);
`,
	},
	{
		version: 11,
		name:    "phase3_complete_tenant_model",
		sql: `
-- Fase 3: complete the MailView-owned Control/Data Plane catalogue. Native
-- MailView tables use strict tenant context: unlike migrated upstream tables,
-- there is no legacy-workspace fallback when app.tenant_id is absent.
CREATE TABLE IF NOT EXISTS mv_tenant_branding (
 tenant_id UUID PRIMARY KEY REFERENCES mv_tenants(id) ON DELETE CASCADE,
 logo_url TEXT NOT NULL DEFAULT '', primary_color TEXT NOT NULL DEFAULT '',
 product_name TEXT NOT NULL DEFAULT 'MailView', custom_css TEXT NOT NULL DEFAULT '',
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS mv_tenant_sessions (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
 request_id TEXT NOT NULL DEFAULT '', expires_at TIMESTAMPTZ NOT NULL,
 revoked_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (tenant_id, session_id)
);
CREATE INDEX IF NOT EXISTS mv_tenant_sessions_tenant_user_idx ON mv_tenant_sessions(tenant_id,user_id,expires_at);
CREATE TABLE IF NOT EXISTS mv_api_keys (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 name TEXT NOT NULL, key_prefix TEXT NOT NULL, secret_hash TEXT NOT NULL,
 permissions TEXT[] NOT NULL DEFAULT '{}', last_used_at TIMESTAMPTZ,
 expires_at TIMESTAMPTZ, revoked_at TIMESTAMPTZ, created_by INTEGER NOT NULL REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,name), UNIQUE(tenant_id,key_prefix)
);
CREATE INDEX IF NOT EXISTS mv_api_keys_tenant_active_idx ON mv_api_keys(tenant_id,revoked_at,expires_at);
CREATE TABLE IF NOT EXISTS mv_billing_accounts (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL UNIQUE REFERENCES mv_tenants(id) ON DELETE CASCADE,
 provider TEXT NOT NULL DEFAULT 'manual', external_customer_id TEXT NOT NULL DEFAULT '',
 billing_email TEXT NOT NULL DEFAULT '', tax_id TEXT NOT NULL DEFAULT '',
 metadata JSONB NOT NULL DEFAULT '{}', updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS mv_billing_accounts_tenant_id_idx ON mv_billing_accounts(tenant_id,id);
CREATE INDEX IF NOT EXISTS mv_billing_accounts_tenant_provider_idx ON mv_billing_accounts(tenant_id,provider);
CREATE TABLE IF NOT EXISTS mv_subscriptions (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 billing_account_id UUID NOT NULL, plan_code TEXT NOT NULL REFERENCES mv_tenant_plans(code),
 status TEXT NOT NULL CHECK(status IN ('trialing','active','past_due','cancelled','ended')),
 provider_subscription_id TEXT NOT NULL DEFAULT '', current_period_start TIMESTAMPTZ,
 current_period_end TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,billing_account_id) REFERENCES mv_billing_accounts(tenant_id,id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS mv_subscriptions_tenant_id_idx ON mv_subscriptions(tenant_id,id);
CREATE INDEX IF NOT EXISTS mv_subscriptions_tenant_status_idx ON mv_subscriptions(tenant_id,status,current_period_end);
CREATE TABLE IF NOT EXISTS mv_invoices (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 subscription_id UUID, provider_invoice_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL,
 currency CHAR(3) NOT NULL DEFAULT 'BRL', amount_cents BIGINT NOT NULL DEFAULT 0 CHECK(amount_cents>=0),
 due_at TIMESTAMPTZ, paid_at TIMESTAMPTZ, document_url TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,subscription_id) REFERENCES mv_subscriptions(tenant_id,id) ON DELETE SET NULL (subscription_id)
);
CREATE INDEX IF NOT EXISTS mv_invoices_tenant_status_idx ON mv_invoices(tenant_id,status,due_at);
CREATE TABLE IF NOT EXISTS mv_feature_flags (
 tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 key TEXT NOT NULL CHECK(key ~ '^[a-z][a-z0-9_.-]{1,127}$'), enabled BOOLEAN NOT NULL DEFAULT false,
 value JSONB NOT NULL DEFAULT '{}', updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(tenant_id,key)
);

CREATE TABLE IF NOT EXISTS mv_smtp_profiles (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 name TEXT NOT NULL, host TEXT NOT NULL, port INTEGER NOT NULL CHECK(port BETWEEN 1 AND 65535),
 username TEXT NOT NULL DEFAULT '', secret_ref TEXT NOT NULL, tls_mode TEXT NOT NULL CHECK(tls_mode IN ('none','starttls','tls')),
 is_dedicated BOOLEAN NOT NULL DEFAULT false, status TEXT NOT NULL DEFAULT 'active',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,name)
);
CREATE UNIQUE INDEX IF NOT EXISTS mv_smtp_profiles_tenant_id_idx ON mv_smtp_profiles(tenant_id,id);
CREATE INDEX IF NOT EXISTS mv_smtp_profiles_tenant_status_idx ON mv_smtp_profiles(tenant_id,status,id);
CREATE TABLE IF NOT EXISTS mv_sender_identities (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 smtp_profile_id UUID, email TEXT NOT NULL, name TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending',
 verified_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,smtp_profile_id) REFERENCES mv_smtp_profiles(tenant_id,id) ON DELETE SET NULL (smtp_profile_id),
 UNIQUE(tenant_id,email)
);
CREATE UNIQUE INDEX IF NOT EXISTS mv_sender_identities_tenant_id_idx ON mv_sender_identities(tenant_id,id);
CREATE INDEX IF NOT EXISTS mv_sender_identities_tenant_status_idx ON mv_sender_identities(tenant_id,status,id);
CREATE TABLE IF NOT EXISTS mv_sending_domains (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 hostname TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending', dkim_selector TEXT NOT NULL DEFAULT '',
 verified_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,hostname)
);
CREATE UNIQUE INDEX IF NOT EXISTS mv_sending_domains_tenant_id_idx ON mv_sending_domains(tenant_id,id);
CREATE TABLE IF NOT EXISTS mv_domain_dns_records (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 domain_id UUID NOT NULL, type TEXT NOT NULL CHECK(type IN ('TXT','CNAME','MX')),
 name TEXT NOT NULL, expected_value TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
 last_checked_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,domain_id) REFERENCES mv_sending_domains(tenant_id,id) ON DELETE CASCADE,
 UNIQUE(tenant_id,domain_id,type,name)
);
CREATE INDEX IF NOT EXISTS mv_domain_dns_records_tenant_status_idx ON mv_domain_dns_records(tenant_id,status,domain_id);
CREATE TABLE IF NOT EXISTS mv_complaints (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 subscriber_id BIGINT, campaign_id INTEGER, source TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '',
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,subscriber_id) REFERENCES subscribers(tenant_id,id) ON DELETE SET NULL (subscriber_id),
 FOREIGN KEY(tenant_id,campaign_id) REFERENCES campaigns(tenant_id,id) ON DELETE SET NULL (campaign_id)
);
CREATE INDEX IF NOT EXISTS mv_complaints_tenant_time_idx ON mv_complaints(tenant_id,occurred_at DESC,id);
CREATE TABLE IF NOT EXISTS mv_campaign_events (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 campaign_id INTEGER, subscriber_id BIGINT, type TEXT NOT NULL, occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 metadata JSONB NOT NULL DEFAULT '{}',
 FOREIGN KEY(tenant_id,campaign_id) REFERENCES campaigns(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,subscriber_id) REFERENCES subscribers(tenant_id,id) ON DELETE SET NULL (subscriber_id)
);
CREATE INDEX IF NOT EXISTS mv_campaign_events_tenant_campaign_idx ON mv_campaign_events(tenant_id,campaign_id,occurred_at DESC);
CREATE TABLE IF NOT EXISTS mv_exports (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 actor_id INTEGER NOT NULL REFERENCES users(id), type TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
 storage_key TEXT NOT NULL DEFAULT '', signature TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mv_exports_tenant_status_idx ON mv_exports(tenant_id,status,created_at DESC);
CREATE TABLE IF NOT EXISTS mv_webhooks (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 name TEXT NOT NULL, url TEXT NOT NULL, event_types TEXT[] NOT NULL DEFAULT '{}', secret_ref TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'active', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,name)
);
CREATE UNIQUE INDEX IF NOT EXISTS mv_webhooks_tenant_id_idx ON mv_webhooks(tenant_id,id);
CREATE TABLE IF NOT EXISTS mv_webhook_deliveries (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 webhook_id UUID NOT NULL, event_type TEXT NOT NULL, payload JSONB NOT NULL, signature TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending', attempts INTEGER NOT NULL DEFAULT 0, next_attempt_at TIMESTAMPTZ,
 response_status INTEGER, response_body TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,webhook_id) REFERENCES mv_webhooks(tenant_id,id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS mv_webhook_deliveries_tenant_queue_idx ON mv_webhook_deliveries(tenant_id,status,next_attempt_at,id);
CREATE TABLE IF NOT EXISTS mv_transactional_messages (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 idempotency_key TEXT NOT NULL, sender_identity_id UUID, recipient TEXT NOT NULL,
 subject TEXT NOT NULL, body_html TEXT NOT NULL DEFAULT '', body_text TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL DEFAULT 'pending', provider_message_id TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '',
 scheduled_at TIMESTAMPTZ, sent_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,sender_identity_id) REFERENCES mv_sender_identities(tenant_id,id) ON DELETE SET NULL (sender_identity_id),
 UNIQUE(tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS mv_transactional_messages_tenant_queue_idx ON mv_transactional_messages(tenant_id,status,scheduled_at,id);

DO $mv$
DECLARE t text;
BEGIN
 FOREACH t IN ARRAY ARRAY[
  'mv_tenant_branding','mv_tenant_sessions','mv_api_keys','mv_billing_accounts','mv_subscriptions','mv_invoices','mv_feature_flags',
  'mv_smtp_profiles','mv_sender_identities','mv_sending_domains','mv_domain_dns_records','mv_complaints','mv_campaign_events',
  'mv_exports','mv_webhooks','mv_webhook_deliveries','mv_transactional_messages'
 ] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('DROP POLICY IF EXISTS %I ON %I','phase3_'||t||'_isolation',t);
  EXECUTE format('CREATE POLICY %I ON %I USING (tenant_id=NULLIF(current_setting(''app.tenant_id'',true),'''')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting(''app.tenant_id'',true),'''')::uuid)','phase3_'||t||'_isolation',t);
 END LOOP;
END $mv$;
`,
	},
	{
		version: 12,
		name:    "phase3_domain_and_dedicated_routing",
		sql: `
CREATE TABLE IF NOT EXISTS mv_reserved_slugs (
 slug TEXT PRIMARY KEY CHECK(slug ~ '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$'), reason TEXT NOT NULL
);
INSERT INTO mv_reserved_slugs(slug,reason) VALUES
 ('admin','platform route'),('api','platform route'),('app','platform route'),('assets','static route'),
 ('auth','authentication route'),('billing','billing route'),('help','support route'),('mail','mail route'),
 ('status','status route'),('support','support route'),('www','public website') ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS mv_tenant_slug_history (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 old_slug TEXT NOT NULL UNIQUE, new_slug TEXT NOT NULL, redirect_until TIMESTAMPTZ NOT NULL,
 changed_by INTEGER NOT NULL REFERENCES users(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mv_tenant_slug_history_tenant_time_idx ON mv_tenant_slug_history(tenant_id,created_at DESC);
CREATE OR REPLACE FUNCTION mv_require_slug_change_workflow() RETURNS trigger AS $$
BEGIN
	IF EXISTS(SELECT 1 FROM mv_reserved_slugs WHERE slug=NEW.slug) THEN
	 RAISE EXCEPTION 'tenant slug is reserved';
	END IF;
 IF TG_OP='UPDATE' AND OLD.slug IS DISTINCT FROM NEW.slug AND current_setting('app.slug_change_workflow',true) IS DISTINCT FROM 'true' THEN
  RAISE EXCEPTION 'tenant slug changes must use the MailView workflow';
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS mv_tenants_slug_workflow ON mv_tenants;
CREATE TRIGGER mv_tenants_slug_workflow BEFORE INSERT OR UPDATE OF slug ON mv_tenants
 FOR EACH ROW EXECUTE FUNCTION mv_require_slug_change_workflow();

ALTER TABLE mv_tenant_domains ADD COLUMN IF NOT EXISTS verification_name TEXT NOT NULL DEFAULT '';
ALTER TABLE mv_tenant_domains ADD COLUMN IF NOT EXISTS verification_value TEXT NOT NULL DEFAULT '';
ALTER TABLE mv_tenant_domains ADD COLUMN IF NOT EXISTS last_checked_at TIMESTAMPTZ;
ALTER TABLE mv_tenant_domains ADD COLUMN IF NOT EXISTS next_check_at TIMESTAMPTZ;
ALTER TABLE mv_tenant_domains ADD COLUMN IF NOT EXISTS failure_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE mv_tenant_domains ADD COLUMN IF NOT EXISTS certificate_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE mv_tenant_domains ADD COLUMN IF NOT EXISTS certificate_revoked_at TIMESTAMPTZ;
ALTER TABLE mv_tenant_domains ADD COLUMN IF NOT EXISTS owner_user_id INTEGER;
UPDATE mv_tenant_domains d SET owner_user_id=COALESCE((
 SELECT m.user_id FROM mv_memberships m
 JOIN mv_membership_roles mr ON mr.membership_id=m.id
 JOIN mv_roles r ON r.id=mr.role_id
 WHERE m.tenant_id=d.tenant_id AND m.status='active' AND r.name='Tenant Owner'
 ORDER BY m.created_at LIMIT 1
),d.created_by) WHERE owner_user_id IS NULL;
ALTER TABLE mv_tenant_domains ALTER COLUMN owner_user_id SET NOT NULL;
ALTER TABLE mv_tenant_domains ADD CONSTRAINT mv_tenant_domains_owner_fk FOREIGN KEY(owner_user_id) REFERENCES users(id) ON DELETE RESTRICT;
UPDATE mv_tenant_domains SET
 verification_name=CASE WHEN verification_method='txt' THEN '_mailview-verification.'||hostname ELSE hostname END,
 verification_value=CASE WHEN verification_method='txt' THEN verification_token ELSE replace(verification_token,'mailview-verify-','')||'.verify.mailview.com.br' END
WHERE verification_name='';
ALTER TABLE mv_tenant_domains DROP CONSTRAINT IF EXISTS mv_tenant_domains_tls_status_check;
ALTER TABLE mv_tenant_domains ADD CONSTRAINT mv_tenant_domains_tls_status_check CHECK(tls_status IN ('none','pending','issued','failed','revoked'));
CREATE INDEX IF NOT EXISTS mv_tenant_domains_tenant_recheck_idx ON mv_tenant_domains(tenant_id,next_check_at,status);

ALTER TABLE mv_tenant_infrastructure ADD COLUMN IF NOT EXISTS database_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE mv_tenant_infrastructure ADD COLUMN IF NOT EXISTS worker_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE mv_tenant_infrastructure ADD COLUMN IF NOT EXISTS smtp_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE mv_tenant_infrastructure ADD COLUMN IF NOT EXISTS storage_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE mv_tenant_infrastructure ADD COLUMN IF NOT EXISTS encryption_key_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE mv_tenant_infrastructure ADD COLUMN IF NOT EXISTS docker_namespace TEXT NOT NULL DEFAULT '';
ALTER TABLE mv_tenant_infrastructure ADD COLUMN IF NOT EXISTS routing_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE mv_tenant_infrastructure ADD COLUMN IF NOT EXISTS activated_at TIMESTAMPTZ;
`,
	},
	{
		version: 13,
		name:    "phase4_rbac_portals_and_data_security",
		sql: `
-- Fase 4: granular capabilities used by the campaign workflow and the
-- tenant security portal. Tenant roles can only reference this fixed
-- catalogue, so a tenant can never manufacture a platform capability.
INSERT INTO mv_permissions(code,description) VALUES
 ('campaign.review.tenant','Submit campaigns for review'),
 ('campaign.schedule.tenant','Schedule approved campaigns'),
 ('campaign.cancel.tenant','Safely cancel scheduled or sending campaigns'),
 ('campaign.test.tenant','Send campaign tests'),
 ('apikey.manage.tenant','Manage tenant API keys'),
 ('security.read.tenant','Read tenant security configuration'),
 ('incident.manage.platform','Manage platform incidents')
ON CONFLICT(code) DO NOTHING;

INSERT INTO mv_role_permissions(role_id,permission_code)
SELECT id,'incident.manage.platform' FROM mv_roles
WHERE scope='platform' AND name IN ('Platform Super Admin','Platform Operations','Platform Security')
ON CONFLICT DO NOTHING;

-- Upgrade the seven system roles of existing tenants. Newly-created tenants
-- receive the same grants from seedTenantRoles.
INSERT INTO mv_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM mv_roles r CROSS JOIN (VALUES
 ('campaign.review.tenant'),('campaign.schedule.tenant'),('campaign.cancel.tenant'),
 ('campaign.test.tenant'),('apikey.manage.tenant'),('security.read.tenant')
) p(code) WHERE r.scope='tenant' AND r.is_system AND r.name='Tenant Owner'
ON CONFLICT DO NOTHING;
INSERT INTO mv_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM mv_roles r CROSS JOIN (VALUES
 ('campaign.review.tenant'),('campaign.schedule.tenant'),('campaign.cancel.tenant'),
 ('campaign.test.tenant'),('apikey.manage.tenant'),('security.read.tenant')
) p(code) WHERE r.scope='tenant' AND r.is_system AND r.name='Tenant Admin'
ON CONFLICT DO NOTHING;
INSERT INTO mv_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM mv_roles r CROSS JOIN (VALUES
 ('campaign.review.tenant'),('campaign.schedule.tenant'),('campaign.cancel.tenant'),('campaign.test.tenant')
) p(code) WHERE r.scope='tenant' AND r.is_system AND r.name='Campaign Manager'
ON CONFLICT DO NOTHING;
INSERT INTO mv_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM mv_roles r CROSS JOIN (VALUES ('campaign.review.tenant'),('campaign.test.tenant')) p(code)
WHERE r.scope='tenant' AND r.is_system AND r.name='Operator' ON CONFLICT DO NOTHING;
INSERT INTO mv_role_permissions(role_id,permission_code)
SELECT r.id,'security.read.tenant' FROM mv_roles r
WHERE r.scope='tenant' AND r.is_system AND r.name IN ('Analyst','Viewer') ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS mv_campaign_workflows (
 tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 campaign_id INTEGER NOT NULL,
 state TEXT NOT NULL DEFAULT 'draft' CHECK(state IN
  ('draft','review','approved','scheduled','sending','completed','rejected','cancelled')),
 revision INTEGER NOT NULL DEFAULT 1 CHECK(revision>0),
 submitted_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
 approved_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
 rejected_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
 scheduled_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
 scheduled_at TIMESTAMPTZ,
 cancellation_requested_at TIMESTAMPTZ,
 completed_at TIMESTAMPTZ,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,campaign_id),
 FOREIGN KEY(tenant_id,campaign_id) REFERENCES campaigns(tenant_id,id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS mv_campaign_workflows_tenant_state_idx
 ON mv_campaign_workflows(tenant_id,state,scheduled_at,campaign_id);
CREATE TABLE IF NOT EXISTS mv_campaign_workflow_events (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES mv_tenants(id) ON DELETE CASCADE,
 campaign_id INTEGER NOT NULL, from_state TEXT NOT NULL, to_state TEXT NOT NULL,
 actor_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
 reason TEXT NOT NULL DEFAULT '', idempotency_key TEXT NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,campaign_id) REFERENCES mv_campaign_workflows(tenant_id,campaign_id) ON DELETE CASCADE,
 UNIQUE(tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS mv_campaign_workflow_events_tenant_campaign_idx
 ON mv_campaign_workflow_events(tenant_id,campaign_id,occurred_at DESC);

-- API secrets remain one-way hashes. SMTP credentials are ciphertext with a
-- key version; plaintext and master keys are never persisted in PostgreSQL.
ALTER TABLE mv_smtp_profiles ADD COLUMN IF NOT EXISTS secret_ciphertext BYTEA;
ALTER TABLE mv_smtp_profiles ADD COLUMN IF NOT EXISTS key_version SMALLINT;
ALTER TABLE mv_smtp_profiles ADD CONSTRAINT mv_smtp_profiles_secret_storage_check
 CHECK(secret_ref<>'' OR (secret_ciphertext IS NOT NULL AND key_version IS NOT NULL));
CREATE TABLE IF NOT EXISTS mv_encryption_key_versions (
 version SMALLINT PRIMARY KEY CHECK(version>0), key_ref TEXT NOT NULL UNIQUE,
 status TEXT NOT NULL CHECK(status IN ('active','decrypt_only','retired')),
 activated_at TIMESTAMPTZ NOT NULL DEFAULT now(), retired_at TIMESTAMPTZ
);

-- Approval is optional per grant, but when requested it is an independent
-- action. Clients can no longer self-assert approved_by while creating it.
ALTER TABLE mv_impersonation_grants ADD COLUMN IF NOT EXISTS approval_required BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mv_impersonation_grants ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS mv_platform_incidents (
 id UUID PRIMARY KEY, tenant_id UUID REFERENCES mv_tenants(id) ON DELETE SET NULL,
 title TEXT NOT NULL CHECK(length(trim(title)) BETWEEN 3 AND 200),
 severity TEXT NOT NULL CHECK(severity IN ('low','medium','high','critical')),
 status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','monitoring','resolved')),
 details TEXT NOT NULL DEFAULT '', opened_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
 resolved_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
 resolved_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mv_platform_incidents_status_idx ON mv_platform_incidents(status,severity,created_at DESC);

-- Minimum contact governance fields. Existing attributes JSONB remains the
-- custom-field store and tenant-first uniqueness continues to deduplicate.
ALTER TABLE subscribers ADD COLUMN IF NOT EXISTS mv_tags TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE subscribers ADD COLUMN IF NOT EXISTS mv_consent_status TEXT NOT NULL DEFAULT 'unknown'
 CHECK(mv_consent_status IN ('unknown','granted','withdrawn'));
ALTER TABLE subscribers ADD COLUMN IF NOT EXISTS mv_consent_source TEXT NOT NULL DEFAULT '';
ALTER TABLE subscribers ADD COLUMN IF NOT EXISTS mv_consent_at TIMESTAMPTZ;
ALTER TABLE subscribers ADD COLUMN IF NOT EXISTS mv_suppressed_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS mv_subscribers_tenant_consent_idx
 ON subscribers(tenant_id,mv_consent_status,id);
-- tenant_id already leads the subscriber lookup indexes. The array-specific
-- GIN index accelerates tag containment after the RLS tenant filter.
CREATE INDEX IF NOT EXISTS mv_subscribers_tags_idx ON subscribers USING GIN(mv_tags);

DO $mv$
DECLARE t text;
BEGIN
 FOREACH t IN ARRAY ARRAY['mv_campaign_workflows','mv_campaign_workflow_events'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
  EXECUTE format('CREATE POLICY %I ON %I USING (tenant_id=NULLIF(current_setting(''app.tenant_id'',true),'''')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting(''app.tenant_id'',true),'''')::uuid)','phase4_'||t||'_isolation',t);
 END LOOP;
END $mv$;
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
