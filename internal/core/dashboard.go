package core

import (
	"net/http"

	"github.com/jmoiron/sqlx/types"
	"github.com/labstack/echo/v4"
)

// GetDashboardCharts returns chart data points to render on the dashboard.
func (c *Core) GetDashboardCharts() (types.JSONText, error) {
	if c.tx != nil {
		var out types.JSONText
		err := c.tx.Get(&out, `SELECT JSON_BUILD_OBJECT(
'link_clicks',COALESCE((SELECT JSON_AGG(x) FROM (SELECT count(*) AS count,created_at::date AS date FROM link_clicks WHERE created_at>=current_date-30 GROUP BY created_at::date ORDER BY date) x),'[]'),
'campaign_views',COALESCE((SELECT JSON_AGG(x) FROM (SELECT count(*) AS count,created_at::date AS date FROM campaign_views WHERE created_at>=current_date-30 GROUP BY created_at::date ORDER BY date) x),'[]'))`)
		return out, err
	}
	_ = c.refreshCache(matDashboardCharts, false)

	var out types.JSONText
	if err := c.q.GetDashboardCharts.Get(&out); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetDashboardCounts returns stats counts to show on the dashboard.
func (c *Core) GetDashboardCounts() (types.JSONText, error) {
	if c.tx != nil {
		var out types.JSONText
		err := c.tx.Get(&out, `SELECT JSON_BUILD_OBJECT(
'subscribers',JSON_BUILD_OBJECT('total',(SELECT count(*) FROM subscribers),'blocklisted',(SELECT count(*) FROM subscribers WHERE status='blocklisted'),'orphans',(SELECT count(*) FROM subscribers s LEFT JOIN subscriber_lists sl ON sl.subscriber_id=s.id WHERE sl.subscriber_id IS NULL)),
'lists',JSON_BUILD_OBJECT('total',(SELECT count(*) FROM lists),'private',(SELECT count(*) FROM lists WHERE type='private'),'public',(SELECT count(*) FROM lists WHERE type='public'),'optin_single',(SELECT count(*) FROM lists WHERE optin='single'),'optin_double',(SELECT count(*) FROM lists WHERE optin='double')),
'campaigns',JSON_BUILD_OBJECT('total',(SELECT count(*) FROM campaigns),'by_status',(SELECT COALESCE(JSON_OBJECT_AGG(status,num),'{}') FROM (SELECT status,count(*) num FROM campaigns GROUP BY status) x)),
'messages',COALESCE((SELECT sum(sent) FROM campaigns),0))`)
		return out, err
	}
	_ = c.refreshCache(matDashboardCounts, false)

	var out types.JSONText
	if err := c.q.GetDashboardCounts.Get(&out); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard stats", "error", pqErrMsg(err)))
	}

	return out, nil
}
