package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/mailview/control"
	"github.com/labstack/echo/v4"
)

// requirePlatformPermission accepts either the legacy Listmonk Super Admin
// (the original Phase 1 bridge, kept so existing operators are not locked
// out) or a user holding a MailView platform role granting code. Tenant
// roles stored in the Control Plane never satisfy this check; they do not
// grant access to global Listmonk data.
func (a *App) requirePlatformPermission(code string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			u, ok := c.Get(auth.UserHTTPCtxKey).(auth.User)
			if !ok {
				return auth.ErrPermDenied
			}
			if u.UserRole.ID == auth.SuperAdminRoleID {
				return next(c)
			}
			can, err := a.mailview.HasPlatformPermission(c.Request().Context(), u.ID, code)
			if err != nil {
				return err
			}
			if !can {
				return auth.ErrPermDenied
			}
			return next(c)
		}
	}
}

// requirePlatformAdmin is the general Control Plane admin gate.
func (a *App) requirePlatformAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return a.requirePlatformPermission("tenant.manage.platform")(next)
}

// requirePlatformRoleAdmin additionally requires platform.roles.manage,
// separate from tenant.manage.platform so a support/ops role holder cannot
// grant themselves Super Admin.
func (a *App) requirePlatformRoleAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return a.requirePlatformPermission("platform.roles.manage")(next)
}

// requireImpersonationAdmin gates starting/revoking/listing impersonation
// grants on support.impersonate.platform, distinct from general tenant
// management so an Operations role holder cannot impersonate members.
func (a *App) requireImpersonationAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return a.requirePlatformPermission("support.impersonate.platform")(next)
}

func (a *App) requirePlatformIncidentAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return a.requirePlatformPermission("incident.manage.platform")(next)
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

func (a *App) ChangeMailViewTenantSlug(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	var in control.ChangeTenantSlugInput
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.ChangeTenantSlug(c.Request().Context(), tenantID, in, mailviewActor(c))
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
	userID, err := a.mailview.ReplaceMembershipRoles(c.Request().Context(), tenantID, membershipID, in.RoleIDs, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	if err := a.core.DeleteUserSessions(userID, ""); err != nil {
		a.log.Printf("invalidating sessions for user_id=%d after role change: %v", userID, err)
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

func (a *App) ListMailViewPlatformRoles(c echo.Context) error {
	out, err := a.mailview.ListPlatformRoles(c.Request().Context())
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) ListMailViewPlatformAssignments(c echo.Context) error {
	out, err := a.mailview.ListPlatformAssignments(c.Request().Context())
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) AssignMailViewPlatformRole(c echo.Context) error {
	var in struct {
		UserID int       `json:"user_id"`
		RoleID uuid.UUID `json:"role_id"`
	}
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.AssignPlatformRole(c.Request().Context(), in.UserID, in.RoleID, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	if err := a.core.DeleteUserSessions(in.UserID, ""); err != nil {
		a.log.Printf("invalidating sessions for user_id=%d after platform role assignment: %v", in.UserID, err)
	}
	return c.JSON(http.StatusCreated, okResp{Data: out})
}

func (a *App) RevokeMailViewPlatformRole(c echo.Context) error {
	userID, err := mailviewIntParam(c, "userID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	roleID, err := mailviewUUIDParam(c, "roleID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	if err := a.mailview.RevokePlatformRole(c.Request().Context(), userID, roleID, mailviewActor(c)); err != nil {
		return mailviewHTTPError(err)
	}
	if err := a.core.DeleteUserSessions(userID, ""); err != nil {
		a.log.Printf("invalidating sessions for user_id=%d after platform role revocation: %v", userID, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *App) GetMailViewDashboard(c echo.Context) error {
	out, err := a.mailview.GetPlatformDashboard(c.Request().Context())
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) ResetMailViewTenantOwner(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	var in struct {
		NewOwnerUserID int `json:"new_owner_user_id"`
	}
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.ResetTenantOwner(c.Request().Context(), tenantID, in.NewOwnerUserID, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) SetMailViewTenantInfrastructure(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	var in control.TenantInfrastructureInput
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.ConfigureTenantInfrastructure(c.Request().Context(), tenantID, in, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) GetMailViewTenantInfrastructure(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.ResolveTenantInfrastructure(c.Request().Context(), tenantID)
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) CreateMailViewTenantRole(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	var in control.CreateTenantRoleInput
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.CreateTenantRole(c.Request().Context(), tenantID, in, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusCreated, okResp{Data: out})
}

func (a *App) DenyMailViewRolePermission(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	roleID, err := mailviewUUIDParam(c, "roleID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	code := c.Param("code")
	if err := a.mailview.DenyRolePermission(c.Request().Context(), tenantID, roleID, code, mailviewActor(c)); err != nil {
		return mailviewHTTPError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *App) AllowMailViewRolePermission(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	roleID, err := mailviewUUIDParam(c, "roleID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	code := c.Param("code")
	if err := a.mailview.AllowRolePermission(c.Request().Context(), tenantID, roleID, code, mailviewActor(c)); err != nil {
		return mailviewHTTPError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *App) StartMailViewImpersonation(c echo.Context) error {
	var in control.StartImpersonationInput
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	actorUserID := mailviewActor(c).UserID
	out, err := a.mailview.StartImpersonation(c.Request().Context(), actorUserID, in, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusCreated, okResp{Data: out})
}

func (a *App) RevokeMailViewImpersonation(c echo.Context) error {
	grantID, err := mailviewUUIDParam(c, "grantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.RevokeImpersonation(c.Request().Context(), grantID, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) ListMailViewImpersonationGrants(c echo.Context) error {
	out, err := a.mailview.ListImpersonationGrants(c.Request().Context())
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) ListMailViewTenantDomains(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.ListTenantDomains(c.Request().Context(), tenantID)
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) CreateMailViewTenantDomain(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	var in control.CreateTenantDomainInput
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.CreateTenantDomain(c.Request().Context(), tenantID, in, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusCreated, okResp{Data: out})
}

func (a *App) VerifyMailViewTenantDomain(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	domainID, err := mailviewUUIDParam(c, "domainID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.MarkTenantDomainVerified(c.Request().Context(), tenantID, domainID, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) RevokeMailViewTenantDomain(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	domainID, err := mailviewUUIDParam(c, "domainID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.RevokeTenantDomain(c.Request().Context(), tenantID, domainID, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) SetMailViewTenantDomainTLSStatus(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	domainID, err := mailviewUUIDParam(c, "domainID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	var in struct {
		Status         string `json:"status"`
		CertificateRef string `json:"certificate_ref"`
	}
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.SetTenantDomainTLSStatus(c.Request().Context(), tenantID, domainID, in.Status, in.CertificateRef, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) RevalidateMailViewTenantDomains(c echo.Context) error {
	checked, err := a.mailview.RevalidateDueDomains(c.Request().Context(), mailviewActor(c), 100)
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: map[string]int{"checked": checked}})
}

func (a *App) ListMailViewTenantPlans(c echo.Context) error {
	out, err := a.mailview.ListTenantPlans(c.Request().Context())
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) GetMailViewTenantQuota(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.GetTenantQuota(c.Request().Context(), tenantID)
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) SetMailViewTenantQuotaPlan(c echo.Context) error {
	tenantID, err := mailviewUUIDParam(c, "tenantID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	var in struct {
		PlanCode string `json:"plan_code"`
	}
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.mailview.SetTenantQuotaPlan(c.Request().Context(), tenantID, in.PlanCode, mailviewActor(c))
	if err != nil {
		return mailviewHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func mailviewIntParam(c echo.Context, name string) (int, error) {
	v, err := strconv.Atoi(c.Param(name))
	if err != nil {
		return 0, control.ErrInvalid
	}
	return v, nil
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
	case errors.Is(err, control.ErrConflict), errors.Is(err, control.ErrInvalidCampaignTransition):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, control.ErrInvalid), errors.Is(err, control.ErrInvalidRole):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, control.ErrPlanDoesNotAllowCustomRoles):
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	case errors.Is(err, control.ErrMFANotRecent):
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	case errors.Is(err, control.ErrGrantExpired), errors.Is(err, control.ErrGrantNotOwned), errors.Is(err, control.ErrGrantPendingApproval):
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	default:
		return err
	}
}
