package control

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/knadh/listmonk/internal/mailview/tenant"
	"github.com/lib/pq"
)

type ContactGovernance struct {
	SubscriberID  int            `db:"id" json:"subscriber_id"`
	Tags          pq.StringArray `db:"mv_tags" json:"tags"`
	ConsentStatus string         `db:"mv_consent_status" json:"consent_status"`
	ConsentSource string         `db:"mv_consent_source" json:"consent_source"`
	ConsentAt     *time.Time     `db:"mv_consent_at" json:"consent_at,omitempty"`
	SuppressedAt  *time.Time     `db:"mv_suppressed_at" json:"suppressed_at,omitempty"`
}

type UpdateContactGovernanceInput struct {
	Tags          []string   `json:"tags"`
	ConsentStatus string     `json:"consent_status"`
	ConsentSource string     `json:"consent_source"`
	ConsentAt     *time.Time `json:"consent_at,omitempty"`
	Suppressed    bool       `json:"suppressed"`
}

func (s *Service) GetContactGovernance(ctx context.Context, subscriberID int) (ContactGovernance, error) {
	var out ContactGovernance
	if subscriberID < 1 {
		return out, ErrInvalid
	}
	tx, scope, err := tenant.Begin(ctx, s.db)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err := tx.GetContext(ctx, &out, `SELECT id,mv_tags,mv_consent_status,mv_consent_source,mv_consent_at,mv_suppressed_at
	 FROM subscribers WHERE tenant_id=$1 AND id=$2`, scope.TenantID, subscriberID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ContactGovernance{}, ErrNotFound
		}
		return ContactGovernance{}, err
	}
	return out, tx.Commit()
}

func (s *Service) UpdateContactGovernance(ctx context.Context, subscriberID int, in UpdateContactGovernanceInput, actor Actor) (ContactGovernance, error) {
	var out ContactGovernance
	in.ConsentStatus = strings.ToLower(strings.TrimSpace(in.ConsentStatus))
	in.ConsentSource = strings.TrimSpace(in.ConsentSource)
	if subscriberID < 1 || (in.ConsentStatus != "unknown" && in.ConsentStatus != "granted" && in.ConsentStatus != "withdrawn") || len(in.ConsentSource) > 200 || len(in.Tags) > 100 {
		return out, ErrInvalid
	}
	tags := make([]string, 0, len(in.Tags))
	seen := map[string]struct{}{}
	for _, value := range in.Tags {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || len(value) > 64 {
			return ContactGovernance{}, ErrInvalid
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			tags = append(tags, value)
		}
	}
	tx, scope, err := tenant.Begin(ctx, s.db)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err := tx.GetContext(ctx, &out, `UPDATE subscribers SET mv_tags=$3,mv_consent_status=$4,
	 mv_consent_source=$5,mv_consent_at=$6,mv_suppressed_at=CASE WHEN $7 THEN COALESCE(mv_suppressed_at,now()) ELSE NULL END,
	 status=CASE WHEN $7 THEN 'blocklisted'::subscriber_status ELSE status END,updated_at=now()
	 WHERE tenant_id=$1 AND id=$2
	 RETURNING id,mv_tags,mv_consent_status,mv_consent_source,mv_consent_at,mv_suppressed_at`,
		scope.TenantID, subscriberID, pq.Array(tags), in.ConsentStatus, in.ConsentSource, in.ConsentAt, in.Suppressed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ContactGovernance{}, ErrNotFound
		}
		return ContactGovernance{}, err
	}
	if err := appendAudit(ctx, tx, &scope.TenantID, actor, "subscriber.governance.update", "subscriber", stringInt(subscriberID), "success", "", map[string]any{"consent_status": out.ConsentStatus, "suppressed": out.SuppressedAt != nil}); err != nil {
		return ContactGovernance{}, err
	}
	return out, tx.Commit()
}
