package main

import (
	"net/http"
	"strings"

	"github.com/knadh/listmonk/internal/mailview/control"
	"github.com/labstack/echo/v4"
)

func (a *App) GetMailViewTenantHome(c echo.Context) error {
	ctx, err := a.mailviewRequestContextFor(c, "analytics.read.tenant")
	if err != nil {
		return err
	}
	if ctx == nil {
		return echo.NewHTTPError(http.StatusNotFound, "tenant portal is unavailable on the platform host")
	}
	out, err := a.mailview.GetTenantHome(ctx)
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) GetMailViewCampaignWorkflow(c echo.Context) error {
	id, err := mailviewIntParam(c, "id")
	if err != nil {
		return mailviewHTTPError(err)
	}
	ctx, err := a.mailviewRequestContextFor(c, "campaign.read.tenant")
	if err != nil {
		return err
	}
	out, err := a.mailview.GetCampaignWorkflow(ctx, id)
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) TransitionMailViewCampaign(c echo.Context) error {
	id, err := mailviewIntParam(c, "id")
	if err != nil {
		return mailviewHTTPError(err)
	}
	var in control.CampaignTransitionInput
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	if in.IdempotencyKey == "" {
		in.IdempotencyKey = strings.TrimSpace(c.Request().Header.Get("Idempotency-Key"))
	}
	ctx, err := a.mailviewRequestContextFor(c, campaignTransitionPermission(in.ToState))
	if err != nil {
		return err
	}
	out, err := a.mailview.TransitionCampaign(ctx, id, in, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func campaignTransitionPermission(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case control.CampaignStateReview:
		return "campaign.review.tenant"
	case control.CampaignStateApproved, control.CampaignStateRejected:
		return "campaign.approve.tenant"
	case control.CampaignStateScheduled, control.CampaignStateSending, control.CampaignStateCompleted:
		return "campaign.schedule.tenant"
	case control.CampaignStateCancelled:
		return "campaign.cancel.tenant"
	default:
		return "campaign.manage.tenant"
	}
}

func (a *App) ApproveMailViewImpersonation(c echo.Context) error {
	grantID, err := mailviewUUIDParam(c, "grantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	actor := mailviewActor(c)
	out, err := a.mailview.ApproveImpersonation(c.Request().Context(), grantID, actor.UserID, actor)
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) ListMailViewAPIKeys(c echo.Context) error {
	ctx, err := a.mailviewRequestContextFor(c, "apikey.manage.tenant")
	if err != nil {
		return err
	}
	out, err := a.mailview.ListAPIKeys(ctx)
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) CreateMailViewAPIKey(c echo.Context) error {
	var in control.CreateAPIKeyInput
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	ctx, err := a.mailviewRequestContextFor(c, "apikey.manage.tenant")
	if err != nil {
		return err
	}
	out, err := a.mailview.CreateAPIKey(ctx, in, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusCreated, okResp{Data: out})
}

func (a *App) RevokeMailViewAPIKey(c echo.Context) error {
	id, err := mailviewUUIDParam(c, "keyID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	ctx, err := a.mailviewRequestContextFor(c, "apikey.manage.tenant")
	if err != nil {
		return err
	}
	if err := a.mailview.RevokeAPIKey(ctx, id, mailviewActor(c)); err != nil {
		return mailviewHTTPError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *App) GetMailViewContactGovernance(c echo.Context) error {
	id, err := mailviewIntParam(c, "id")
	if err != nil {
		return mailviewHTTPError(err)
	}
	ctx, err := a.mailviewRequestContextFor(c, "subscriber.read.tenant")
	if err != nil {
		return err
	}
	out, err := a.mailview.GetContactGovernance(ctx, id)
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) UpdateMailViewContactGovernance(c echo.Context) error {
	id, err := mailviewIntParam(c, "id")
	if err != nil {
		return mailviewHTTPError(err)
	}
	var in control.UpdateContactGovernanceInput
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	ctx, err := a.mailviewRequestContextFor(c, "subscriber.manage.tenant")
	if err != nil {
		return err
	}
	out, err := a.mailview.UpdateContactGovernance(ctx, id, in, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) ListMailViewIncidents(c echo.Context) error {
	out, err := a.mailview.ListIncidents(c.Request().Context())
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) CreateMailViewIncident(c echo.Context) error {
	var in control.CreateIncidentInput
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.CreateIncident(c.Request().Context(), in, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusCreated, okResp{Data: out})
}

func (a *App) ResolveMailViewIncident(c echo.Context) error {
	id, err := mailviewUUIDParam(c, "incidentID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.ResolveIncident(c.Request().Context(), id, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}
