package control

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
)

type ChangeTenantSlugInput struct {
	Slug         string `json:"slug"`
	RedirectDays int    `json:"redirect_days"`
}

type TenantSlugChange struct {
	Tenant        Tenant    `json:"tenant"`
	PreviousSlug  string    `json:"previous_slug"`
	RedirectUntil time.Time `json:"redirect_until"`
}

// ChangeTenantSlug performs the only supported slug mutation. It preserves a
// time-boxed alias so links do not break abruptly and records an audit event.
func (s *Service) ChangeTenantSlug(ctx context.Context, tenantID uuid.UUID, in ChangeTenantSlugInput, actor Actor) (TenantSlugChange, error) {
	slug, err := NormalizeSlug(in.Slug)
	if err != nil {
		return TenantSlugChange{}, err
	}
	days := in.RedirectDays
	if days == 0 {
		days = 30
	}
	if days < 1 || days > 365 {
		return TenantSlugChange{}, ErrInvalid
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return TenantSlugChange{}, err
	}
	defer tx.Rollback()

	var current Tenant
	if err := tx.GetContext(ctx, &current, `SELECT id,slug,name,status,created_at,updated_at FROM mv_tenants WHERE id=$1 FOR UPDATE`, tenantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TenantSlugChange{}, ErrNotFound
		}
		return TenantSlugChange{}, err
	}
	if current.Slug == slug {
		return TenantSlugChange{}, ErrConflict
	}
	var reserved bool
	if err := tx.GetContext(ctx, &reserved, `SELECT EXISTS(SELECT 1 FROM mv_reserved_slugs WHERE slug=$1)`, slug); err != nil {
		return TenantSlugChange{}, err
	}
	if reserved {
		return TenantSlugChange{}, ErrInvalid
	}
	var occupied bool
	if err := tx.GetContext(ctx, &occupied, `SELECT EXISTS(
		SELECT 1 FROM mv_tenants WHERE slug=$1
		UNION ALL SELECT 1 FROM mv_tenant_slug_history WHERE old_slug=$1
	)`, slug); err != nil {
		return TenantSlugChange{}, err
	}
	if occupied {
		return TenantSlugChange{}, ErrConflict
	}

	redirectUntil := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
	if _, err := tx.ExecContext(ctx, `INSERT INTO mv_tenant_slug_history(id,tenant_id,old_slug,new_slug,redirect_until,changed_by)
		VALUES($1,$2,$3,$4,$5,$6)`, uuid.Must(uuid.NewV4()), tenantID, current.Slug, slug, redirectUntil, actor.UserID); err != nil {
		if isUniqueViolation(err) {
			return TenantSlugChange{}, ErrConflict
		}
		return TenantSlugChange{}, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.slug_change_workflow','true',true)`); err != nil {
		return TenantSlugChange{}, err
	}
	var updated Tenant
	if err := tx.GetContext(ctx, &updated, `UPDATE mv_tenants SET slug=$2,updated_at=now() WHERE id=$1
		RETURNING id,slug,name,status,created_at,updated_at`, tenantID, slug); err != nil {
		if isUniqueViolation(err) {
			return TenantSlugChange{}, ErrConflict
		}
		return TenantSlugChange{}, err
	}
	if err := appendAudit(ctx, tx, &tenantID, actor, "tenant.slug.change", "tenant", tenantID.String(), "success", "", map[string]any{
		"old_slug": current.Slug, "new_slug": slug, "redirect_until": redirectUntil,
	}); err != nil {
		return TenantSlugChange{}, err
	}
	return TenantSlugChange{Tenant: updated, PreviousSlug: current.Slug, RedirectUntil: redirectUntil}, tx.Commit()
}

// ResolveTenantSlug accepts the canonical slug or a non-expired historical
// alias. The returned canonical flag lets HTTP gateways issue a 308 redirect.
func (s *Service) ResolveTenantSlug(ctx context.Context, value string) (tenant Tenant, canonical bool, err error) {
	slug := strings.ToLower(strings.TrimSpace(value))
	err = s.db.GetContext(ctx, &tenant, `SELECT id,slug,name,status,created_at,updated_at FROM mv_tenants WHERE slug=$1`, slug)
	if err == nil {
		return tenant, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Tenant{}, false, err
	}
	err = s.db.GetContext(ctx, &tenant, `
		SELECT t.id,t.slug,t.name,t.status,t.created_at,t.updated_at
		FROM mv_tenant_slug_history h JOIN mv_tenants t ON t.id=h.tenant_id
		WHERE h.old_slug=$1 AND h.redirect_until>now()`, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return Tenant{}, false, ErrNotFound
	}
	return tenant, false, err
}
