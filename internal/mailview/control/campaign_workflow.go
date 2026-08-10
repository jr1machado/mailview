package control

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/internal/mailview/tenant"
)

var ErrInvalidCampaignTransition = errors.New("invalid campaign workflow transition")

const (
	CampaignStateDraft     = "draft"
	CampaignStateReview    = "review"
	CampaignStateApproved  = "approved"
	CampaignStateScheduled = "scheduled"
	CampaignStateSending   = "sending"
	CampaignStateCompleted = "completed"
	CampaignStateRejected  = "rejected"
	CampaignStateCancelled = "cancelled"
)

type CampaignWorkflow struct {
	TenantID                uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	CampaignID              int        `db:"campaign_id" json:"campaign_id"`
	State                   string     `db:"state" json:"state"`
	Revision                int        `db:"revision" json:"revision"`
	SubmittedBy             *int       `db:"submitted_by" json:"submitted_by,omitempty"`
	ApprovedBy              *int       `db:"approved_by" json:"approved_by,omitempty"`
	RejectedBy              *int       `db:"rejected_by" json:"rejected_by,omitempty"`
	ScheduledBy             *int       `db:"scheduled_by" json:"scheduled_by,omitempty"`
	ScheduledAt             *time.Time `db:"scheduled_at" json:"scheduled_at,omitempty"`
	CancellationRequestedAt *time.Time `db:"cancellation_requested_at" json:"cancellation_requested_at,omitempty"`
	CompletedAt             *time.Time `db:"completed_at" json:"completed_at,omitempty"`
	CreatedAt               time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt               time.Time  `db:"updated_at" json:"updated_at"`
}

type CampaignTransitionInput struct {
	ToState        string     `json:"to_state"`
	Reason         string     `json:"reason"`
	IdempotencyKey string     `json:"idempotency_key"`
	ScheduledAt    *time.Time `json:"scheduled_at,omitempty"`
}

var campaignTransitions = map[string]map[string]struct{}{
	CampaignStateDraft:     {CampaignStateReview: {}},
	CampaignStateReview:    {CampaignStateApproved: {}, CampaignStateRejected: {}},
	CampaignStateRejected:  {CampaignStateReview: {}},
	CampaignStateApproved:  {CampaignStateScheduled: {}, CampaignStateCancelled: {}},
	CampaignStateScheduled: {CampaignStateSending: {}, CampaignStateCancelled: {}},
	CampaignStateSending:   {CampaignStateCompleted: {}, CampaignStateCancelled: {}},
}

// GetCampaignWorkflow returns the sidecar approval state. A draft row is
// created lazily so existing tenant campaigns remain compatible.
func (s *Service) GetCampaignWorkflow(ctx context.Context, campaignID int) (CampaignWorkflow, error) {
	var out CampaignWorkflow
	if campaignID < 1 {
		return out, ErrInvalid
	}
	tx, scope, err := tenant.Begin(ctx, s.db)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err := tx.GetContext(ctx, &out, `
INSERT INTO mv_campaign_workflows(tenant_id,campaign_id)
SELECT $1,id FROM campaigns WHERE tenant_id=$1 AND id=$2
ON CONFLICT(tenant_id,campaign_id) DO UPDATE SET campaign_id=EXCLUDED.campaign_id
RETURNING tenant_id,campaign_id,state,revision,submitted_by,approved_by,rejected_by,scheduled_by,
 scheduled_at,cancellation_requested_at,completed_at,created_at,updated_at`, scope.TenantID, campaignID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return out, ErrNotFound
		}
		return out, err
	}
	return out, tx.Commit()
}

// TransitionCampaign serializes transitions with SELECT FOR UPDATE and
// stores an idempotency key, preventing double scheduling/sending on retries.
func (s *Service) TransitionCampaign(ctx context.Context, campaignID int, in CampaignTransitionInput, actor Actor) (CampaignWorkflow, error) {
	var out CampaignWorkflow
	in.ToState = strings.ToLower(strings.TrimSpace(in.ToState))
	in.Reason = strings.TrimSpace(in.Reason)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if campaignID < 1 || len(in.IdempotencyKey) < 8 || len(in.IdempotencyKey) > 200 {
		return out, ErrInvalid
	}
	tx, scope, err := tenant.Begin(ctx, s.db)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()

	var priorCampaignID int
	err = tx.GetContext(ctx, &priorCampaignID, `SELECT campaign_id FROM mv_campaign_workflow_events WHERE tenant_id=$1 AND idempotency_key=$2`, scope.TenantID, in.IdempotencyKey)
	if err == nil {
		if priorCampaignID != campaignID {
			return out, ErrConflict
		}
		if err := selectCampaignWorkflow(ctx, tx, scope.TenantID, campaignID, &out, false); err != nil {
			return out, err
		}
		return out, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO mv_campaign_workflows(tenant_id,campaign_id)
 SELECT $1,id FROM campaigns WHERE tenant_id=$1 AND id=$2 ON CONFLICT DO NOTHING`, scope.TenantID, campaignID); err != nil {
		return out, err
	}
	if err := selectCampaignWorkflow(ctx, tx, scope.TenantID, campaignID, &out, true); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CampaignWorkflow{}, ErrNotFound
		}
		return CampaignWorkflow{}, err
	}
	if _, ok := campaignTransitions[out.State][in.ToState]; !ok {
		return CampaignWorkflow{}, ErrInvalidCampaignTransition
	}
	if in.ToState == CampaignStateRejected && len(in.Reason) < 3 {
		return CampaignWorkflow{}, ErrInvalid
	}
	if in.ToState == CampaignStateScheduled && (in.ScheduledAt == nil || !in.ScheduledAt.After(time.Now())) {
		return CampaignWorkflow{}, ErrInvalid
	}

	fromState := out.State
	if err := tx.GetContext(ctx, &out, `
UPDATE mv_campaign_workflows SET state=$3,revision=revision+1,updated_at=now(),
 submitted_by=CASE WHEN $3='review' THEN $4 ELSE submitted_by END,
 approved_by=CASE WHEN $3='approved' THEN $4 ELSE approved_by END,
 rejected_by=CASE WHEN $3='rejected' THEN $4 ELSE rejected_by END,
 scheduled_by=CASE WHEN $3='scheduled' THEN $4 ELSE scheduled_by END,
 scheduled_at=CASE WHEN $3='scheduled' THEN $5 ELSE scheduled_at END,
 cancellation_requested_at=CASE WHEN $3='cancelled' THEN now() ELSE cancellation_requested_at END,
 completed_at=CASE WHEN $3='completed' THEN now() ELSE completed_at END
WHERE tenant_id=$1 AND campaign_id=$2
RETURNING tenant_id,campaign_id,state,revision,submitted_by,approved_by,rejected_by,scheduled_by,
 scheduled_at,cancellation_requested_at,completed_at,created_at,updated_at`, scope.TenantID, campaignID, in.ToState, actor.UserID, in.ScheduledAt); err != nil {
		return CampaignWorkflow{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mv_campaign_workflow_events
 (id,tenant_id,campaign_id,from_state,to_state,actor_user_id,reason,idempotency_key)
 VALUES($1,$2,$3,$4,$5,NULLIF($6,0),$7,$8)`, uuid.Must(uuid.NewV4()), scope.TenantID, campaignID, fromState, in.ToState, actor.UserID, in.Reason, in.IdempotencyKey); err != nil {
		return CampaignWorkflow{}, err
	}
	if err := syncLegacyCampaignState(ctx, tx, scope.TenantID, campaignID, in); err != nil {
		return CampaignWorkflow{}, err
	}
	if err := appendAudit(ctx, tx, &scope.TenantID, actor, "campaign.workflow.transition", "campaign", stringInt(campaignID), "success", in.Reason, map[string]any{"from": fromState, "to": in.ToState, "revision": out.Revision}); err != nil {
		return CampaignWorkflow{}, err
	}
	return out, tx.Commit()
}

func selectCampaignWorkflow(ctx context.Context, tx interface {
	GetContext(context.Context, any, string, ...any) error
}, tenantID uuid.UUID, campaignID int, out *CampaignWorkflow, lock bool) error {
	query := `SELECT tenant_id,campaign_id,state,revision,submitted_by,approved_by,rejected_by,scheduled_by,
 scheduled_at,cancellation_requested_at,completed_at,created_at,updated_at
 FROM mv_campaign_workflows WHERE tenant_id=$1 AND campaign_id=$2`
	if lock {
		query += " FOR UPDATE"
	}
	return tx.GetContext(ctx, out, query, tenantID, campaignID)
}

func syncLegacyCampaignState(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, tenantID uuid.UUID, campaignID int, in CampaignTransitionInput) error {
	var status string
	switch in.ToState {
	case CampaignStateScheduled:
		status = "scheduled"
	case CampaignStateSending:
		status = "running"
	case CampaignStateCompleted:
		status = "finished"
	case CampaignStateCancelled:
		status = "cancelled"
	default:
		return nil
	}
	_, err := tx.ExecContext(ctx, `UPDATE campaigns SET status=$3::campaign_status,
 send_at=CASE WHEN $3='scheduled' THEN $4 ELSE send_at END,
 updated_at=now() WHERE tenant_id=$1 AND id=$2`, tenantID, campaignID, status, in.ScheduledAt)
	return err
}

func stringInt(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = digits[value%10]
		value /= 10
	}
	return string(buf[i:])
}
