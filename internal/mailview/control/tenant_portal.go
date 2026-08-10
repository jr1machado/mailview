package control

import (
	"context"
	"time"

	"github.com/knadh/listmonk/internal/mailview/tenant"
)

type TenantHome struct {
	EmailsSentThisMonth  int64    `db:"emails_sent" json:"emails_sent"`
	ActiveCampaigns      int      `db:"active_campaigns" json:"active_campaigns"`
	Contacts             int      `db:"contacts" json:"contacts"`
	BouncesThisMonth     int      `db:"bounces" json:"bounces"`
	PlanCode             string   `db:"plan_code" json:"plan_code"`
	PlanEmailLimit       *int     `db:"max_emails_month" json:"plan_email_limit,omitempty"`
	PendingDomains       int      `db:"pending_domains" json:"pending_domains"`
	DeliverabilityAlerts []string `db:"-" json:"deliverability_alerts"`
}

// GetTenantHome provides the tenant portal summary from one RLS-bound
// transaction. No tenant identifier is accepted from the request body.
func (s *Service) GetTenantHome(ctx context.Context) (TenantHome, error) {
	var out TenantHome
	tx, scope, err := tenant.Begin(ctx, s.db)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	period := time.Now().UTC().Format("2006-01-01")
	err = tx.GetContext(ctx, &out, `
SELECT
 COALESCE((SELECT emails_sent FROM mv_tenant_usage WHERE tenant_id=$1 AND period_start=date_trunc('month',$2::date)::date),0) emails_sent,
 (SELECT count(*) FROM campaigns WHERE tenant_id=$1 AND status IN ('running','scheduled')) active_campaigns,
 (SELECT count(*) FROM subscribers WHERE tenant_id=$1) contacts,
 (SELECT count(*) FROM bounces WHERE tenant_id=$1 AND created_at>=date_trunc('month',now())) bounces,
 COALESCE((SELECT q.plan_code FROM mv_tenant_quotas q WHERE q.tenant_id=$1),'starter') plan_code,
 COALESCE((SELECT q.max_emails_month FROM mv_tenant_quotas q WHERE q.tenant_id=$1),
          (SELECT p.max_emails_month FROM mv_tenant_plans p WHERE p.code='starter')) max_emails_month,
 (SELECT count(*) FROM mv_tenant_domains WHERE tenant_id=$1 AND status<>'verified') pending_domains`, scope.TenantID, period)
	if err != nil {
		return TenantHome{}, err
	}
	if out.PendingDomains > 0 {
		out.DeliverabilityAlerts = append(out.DeliverabilityAlerts, "domain_verification_pending")
	}
	if out.EmailsSentThisMonth > 0 && int64(out.BouncesThisMonth)*100/out.EmailsSentThisMonth >= 5 {
		out.DeliverabilityAlerts = append(out.DeliverabilityAlerts, "high_bounce_rate")
	}
	return out, tx.Commit()
}
