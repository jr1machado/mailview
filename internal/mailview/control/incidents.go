package control

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
)

type PlatformIncident struct {
	ID         uuid.UUID  `db:"id" json:"id"`
	TenantID   *uuid.UUID `db:"tenant_id" json:"tenant_id,omitempty"`
	Title      string     `db:"title" json:"title"`
	Severity   string     `db:"severity" json:"severity"`
	Status     string     `db:"status" json:"status"`
	Details    string     `db:"details" json:"details"`
	OpenedBy   *int       `db:"opened_by" json:"opened_by,omitempty"`
	ResolvedBy *int       `db:"resolved_by" json:"resolved_by,omitempty"`
	ResolvedAt *time.Time `db:"resolved_at" json:"resolved_at,omitempty"`
	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at" json:"updated_at"`
}

type CreateIncidentInput struct {
	TenantID *uuid.UUID `json:"tenant_id,omitempty"`
	Title    string     `json:"title"`
	Severity string     `json:"severity"`
	Details  string     `json:"details"`
}

func (s *Service) CreateIncident(ctx context.Context, in CreateIncidentInput, actor Actor) (PlatformIncident, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Severity = strings.ToLower(strings.TrimSpace(in.Severity))
	in.Details = strings.TrimSpace(in.Details)
	if len(in.Title) < 3 || len(in.Title) > 200 || (in.Severity != "low" && in.Severity != "medium" && in.Severity != "high" && in.Severity != "critical") {
		return PlatformIncident{}, ErrInvalid
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return PlatformIncident{}, err
	}
	defer tx.Rollback()
	var out PlatformIncident
	if err := tx.GetContext(ctx, &out, `INSERT INTO mv_platform_incidents(id,tenant_id,title,severity,details,opened_by)
	 VALUES($1,$2,$3,$4,$5,NULLIF($6,0)) RETURNING id,tenant_id,title,severity,status,details,opened_by,resolved_by,resolved_at,created_at,updated_at`,
		uuid.Must(uuid.NewV4()), in.TenantID, in.Title, in.Severity, in.Details, actor.UserID); err != nil {
		return PlatformIncident{}, err
	}
	if err := appendAudit(ctx, tx, in.TenantID, actor, "incident.create", "incident", out.ID.String(), "success", in.Details, map[string]any{"severity": in.Severity}); err != nil {
		return PlatformIncident{}, err
	}
	return out, tx.Commit()
}

func (s *Service) ListIncidents(ctx context.Context) ([]PlatformIncident, error) {
	out := []PlatformIncident{}
	err := s.db.SelectContext(ctx, &out, `SELECT id,tenant_id,title,severity,status,details,opened_by,resolved_by,resolved_at,created_at,updated_at
	 FROM mv_platform_incidents ORDER BY CASE severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END,created_at DESC LIMIT 200`)
	return out, err
}

func (s *Service) ResolveIncident(ctx context.Context, id uuid.UUID, actor Actor) (PlatformIncident, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return PlatformIncident{}, err
	}
	defer tx.Rollback()
	var out PlatformIncident
	if err := tx.GetContext(ctx, &out, `UPDATE mv_platform_incidents SET status='resolved',resolved_by=NULLIF($2,0),resolved_at=now(),updated_at=now()
	 WHERE id=$1 AND status<>'resolved' RETURNING id,tenant_id,title,severity,status,details,opened_by,resolved_by,resolved_at,created_at,updated_at`, id, actor.UserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PlatformIncident{}, ErrNotFound
		}
		return PlatformIncident{}, err
	}
	if err := appendAudit(ctx, tx, out.TenantID, actor, "incident.resolve", "incident", out.ID.String(), "success", "", nil); err != nil {
		return PlatformIncident{}, err
	}
	return out, tx.Commit()
}
