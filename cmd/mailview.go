package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/mailview/control"
	"github.com/labstack/echo/v4"
)

// requirePlatformAdmin temporarily maps the existing Listmonk Super Admin to
// the platform administrator. Phase 1 deliberately keeps this bridge narrow;
// tenant roles are stored in the MailView Control Plane and do not grant
// access to global Listmonk data.
func (a *App) requirePlatformAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		u, ok := c.Get(auth.UserHTTPCtxKey).(auth.User)
		if !ok || u.UserRoleID != auth.SuperAdminRoleID {
			return auth.ErrPermDenied
		}
		return next(c)
	}
}

func (a *App) CreateMailViewTenant(c echo.Context) error {
	var in control.CreateTenantInput
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.CreateTenant(c.Request().Context(), in, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusCreated, okResp{Data: out})
}

func (a *App) ListMailViewTenants(c echo.Context) error {
	out, err := a.mailview.ListTenants(c.Request().Context())
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) GetMailViewTenant(c echo.Context) error {
	id, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.GetTenant(c.Request().Context(), id)
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) UpdateMailViewTenantStatus(c echo.Context) error {
	id, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	var in struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.UpdateTenantStatus(c.Request().Context(), id, in.Status, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) ListMailViewRoles(c echo.Context) error {
	id, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.ListRoles(c.Request().Context(), id)
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) ListMailViewMemberships(c echo.Context) error {
	id, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.ListMemberships(c.Request().Context(), id)
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) CreateMailViewMembership(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	var in control.CreateMembershipInput
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.CreateMembership(c.Request().Context(), tenantID, in, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusCreated, okResp{Data: out})
}

func (a *App) ReplaceMailViewMembershipRoles(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	membershipID, err := mailviewUUIDParam(c, "membershipID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	var in struct {
		RoleIDs []uuid.UUID `json:"role_ids"`
	}
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	if err := a.mailview.ReplaceMembershipRoles(c.Request().Context(), tenantID, membershipID, in.RoleIDs, mailviewActor(c)); err != nil {
		return mailviewHTTPError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *App) ListMailViewAuditEvents(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.ListAuditEvents(c.Request().Context(), tenantID, 0)
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) GenerateMailViewRecoveryCodes(c echo.Context) error {
	u, ok := c.Get(auth.UserHTTPCtxKey).(auth.User)
	if !ok || u.TwofaType != "totp" {
		return echo.NewHTTPError(http.StatusConflict, "enable TOTP before generating recovery codes")
	}
	codes, err := a.mailview.GenerateRecoveryCodes(c.Request().Context(), u.ID, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusCreated, okResp{Data: map[string]any{"codes": codes}})
}

func mailviewUUIDParam(c echo.Context, name string) (uuid.UUID, error) {
	id, err := uuid.FromString(c.Param(name))
	if err != nil {
		return uuid.Nil, control.ErrInvalid
	}
	return id, nil
}

func mailviewActor(c echo.Context) control.Actor {
	u, _ := c.Get(auth.UserHTTPCtxKey).(auth.User)
	return control.Actor{UserID: u.ID, RequestID: strings.TrimSpace(c.Request().Header.Get("X-Request-ID")), SourceIP: c.RealIP(), UserAgent: c.Request().UserAgent()}
}

func mailviewHTTPError(err error) error {
	switch {
	case errors.Is(err, control.ErrNotFound), errors.Is(err, control.ErrUserNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, control.ErrConflict):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, control.ErrInvalid), errors.Is(err, control.ErrInvalidRole):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		return err
	}
}
