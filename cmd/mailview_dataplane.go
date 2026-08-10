package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/mailview/control"
	"github.com/knadh/listmonk/internal/mailview/dataplane"
	"github.com/knadh/listmonk/internal/mailview/tenant"
	"github.com/labstack/echo/v4"
)

func (a *App) mailviewTenantContext(c echo.Context) (contextErr error) {
	if scoped, ok := c.Get("mailview_tenant_context").(context.Context); ok && scoped != nil {
		return nil
	}
	id, err := uuid.FromString(c.Param("tenantID"))
	if err != nil {
		return control.ErrInvalid
	}
	c.Set("mailview_tenant_context", tenant.WithContext(c.Request().Context(), tenant.Context{TenantID: id, UserID: mailviewActor(c).UserID, RequestID: mailviewActor(c).RequestID}))
	return nil
}
func (a *App) CreateMailViewList(c echo.Context) error {
	if err := a.mailviewTenantContext(c); err != nil {
		return mailviewHTTPError(err)
	}
	var in dataplane.CreateListInput
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.dataplane.CreateList(c.Get("mailview_tenant_context").(context.Context), in)
	if err != nil {
		return dataplaneHTTPError(err)
	}
	return c.JSON(http.StatusCreated, okResp{Data: out})
}
func (a *App) ListMailViewLists(c echo.Context) error {
	if err := a.mailviewTenantContext(c); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.dataplane.ListLists(c.Get("mailview_tenant_context").(context.Context))
	if err != nil {
		return dataplaneHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}
func (a *App) CreateMailViewSubscriber(c echo.Context) error {
	if err := a.mailviewTenantContext(c); err != nil {
		return mailviewHTTPError(err)
	}
	var in dataplane.CreateSubscriberInput
	if err := c.Bind(&in); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.dataplane.CreateSubscriber(c.Get("mailview_tenant_context").(context.Context), in)
	if err != nil {
		return dataplaneHTTPError(err)
	}
	return c.JSON(http.StatusCreated, okResp{Data: out})
}
func (a *App) ListMailViewSubscribers(c echo.Context) error {
	if err := a.mailviewTenantContext(c); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.dataplane.ListSubscribers(c.Get("mailview_tenant_context").(context.Context))
	if err != nil {
		return dataplaneHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}
func dataplaneHTTPError(err error) error {
	if errors.Is(err, dataplane.ErrInvalid) {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return err
}

// impersonationHeader carries an active grant ID (control.StartImpersonation)
// so a platform support user can act as a tenant member on the tenant
// data-plane routes. It is only ever read here: control-plane admin routes,
// billing and MFA/recovery-code endpoints never look at it, so a grant can
// never widen access beyond subscribers/lists for the target tenant.
const impersonationHeader = "X-Mailview-Impersonation"

func (a *App) mailviewDataPerm(next echo.HandlerFunc, tenantPermission string, legacyPermissions ...string) echo.HandlerFunc {
	return func(c echo.Context) error {
		scoped, err := a.mailviewRequestContextFor(c, tenantPermission)
		if err != nil {
			return err
		}
		if scoped != nil {
			c.Set("mailview_tenant_context", scoped)
			tx, _, err := tenant.Begin(scoped, a.db)
			if err != nil {
				return err
			}
			defer tx.Rollback()
			c.Set("mailview_core", a.core.WithTx(tx))
			if err := next(c); err != nil {
				return err
			}
			return tx.Commit()
		}
		if len(legacyPermissions) == 0 {
			return next(c)
		}
		return a.auth.Perm(next, legacyPermissions...)(c)
	}
}

// mailviewPublicTenant binds unauthenticated tracking/subscription/archive
// requests to a tenant resolved exclusively from a verified host.
func (a *App) mailviewPublicTenant(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !a.tenantRoutingEnabled {
			return next(c)
		}
		tenantRecord, err := a.resolveMailviewTenantHost(c.Request().Context(), c.Request().Host)
		if err != nil || tenantRecord.Status != control.TenantStatusActive {
			return echo.NewHTTPError(http.StatusNotFound, "tenant host not found")
		}
		if redirected, err := a.redirectCanonicalTenantHost(c, tenantRecord); redirected {
			return err
		}
		scoped := tenant.WithContext(c.Request().Context(), tenant.Context{TenantID: tenantRecord.ID, RequestID: strings.TrimSpace(c.Request().Header.Get("X-Request-ID"))})
		tx, _, err := tenant.Begin(scoped, a.db)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		c.Set("mailview_tenant_context", scoped)
		c.Set("mailview_core", a.core.WithTx(tx))
		if err := next(c); err != nil {
			return err
		}
		return tx.Commit()
	}
}

func (a *App) blockGlobalAPIsOnTenantHost(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !a.tenantRoutingEnabled {
			return next(c)
		}
		path := c.Request().URL.Path
		blocked := []string{"/api/users", "/api/roles", "/api/settings", "/api/logs", "/api/events", "/api/maintenance", "/api/admin", "/api/tx"}
		for _, prefix := range blocked {
			if strings.HasPrefix(path, prefix) {
				if _, err := a.resolveMailviewTenantHost(c.Request().Context(), c.Request().Host); err == nil {
					return echo.NewHTTPError(http.StatusForbidden, "global Listmonk API is unavailable on a tenant host")
				}
			}
		}
		return next(c)
	}
}

func (a *App) mailviewCore(c echo.Context) *core.Core {
	if scoped, ok := c.Get("mailview_core").(*core.Core); ok && scoped != nil {
		return scoped
	}
	return a.core
}

func (a *App) mailviewRequestContext(c echo.Context) (context.Context, error) {
	return a.mailviewRequestContextFor(c, "")
}

// mailviewRequestContextFor resolves the tenant exclusively from the host,
// validates active membership (or an active support grant), and enforces the
// granular tenant permission before any Data Plane query starts.
func (a *App) mailviewRequestContextFor(c echo.Context, permission string) (context.Context, error) {
	if scoped, ok := c.Get("mailview_tenant_context").(context.Context); ok && scoped != nil {
		return scoped, nil
	}
	if !a.tenantRoutingEnabled {
		return nil, nil
	}
	tenantRecord, err := a.resolveMailviewTenantHost(c.Request().Context(), c.Request().Host)
	if err != nil || tenantRecord.Status != control.TenantStatusActive {
		return nil, echo.NewHTTPError(http.StatusNotFound, "tenant not found")
	}
	if redirected, err := a.redirectCanonicalTenantHost(c, tenantRecord); redirected {
		return nil, err
	}
	u := auth.GetUser(c)
	effectiveUserID := u.ID

	if grantIDStr := strings.TrimSpace(c.Request().Header.Get(impersonationHeader)); grantIDStr != "" {
		grantID, err := uuid.FromString(grantIDStr)
		if err != nil {
			return nil, control.ErrInvalid
		}
		grant, err := a.mailview.ValidateImpersonationGrant(c.Request().Context(), grantID, u.ID)
		if err != nil {
			return nil, mailviewHTTPError(err)
		}
		if grant.TenantID != tenantRecord.ID {
			return nil, echo.NewHTTPError(http.StatusForbidden, "impersonation grant does not cover this tenant")
		}
		effectiveUserID = grant.TargetUserID
		c.Response().Header().Set("X-MailView-Impersonating", "true")
		c.Response().Header().Set("X-MailView-Impersonation-Expires-At", grant.ExpiresAt.UTC().Format(time.RFC3339))
		c.Response().Header().Set("X-MailView-Impersonation-Target", strconv.Itoa(grant.TargetUserID))
	}

	ok, err := a.mailview.HasActiveMembership(c.Request().Context(), tenantRecord.ID, effectiveUserID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, auth.ErrPermDenied
	}
	if permission != "" {
		allowed, err := a.mailview.HasTenantPermission(c.Request().Context(), tenantRecord.ID, effectiveUserID, permission)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, auth.ErrPermDenied
		}
	}
	return tenant.WithContext(c.Request().Context(), tenant.Context{TenantID: tenantRecord.ID, UserID: effectiveUserID, RequestID: strings.TrimSpace(c.Request().Header.Get("X-Request-ID"))}), nil
}

func (a *App) redirectCanonicalTenantHost(c echo.Context, tenantRecord control.Tenant) (bool, error) {
	host := strings.ToLower(c.Request().Host)
	port := ""
	if value, valuePort, err := net.SplitHostPort(host); err == nil {
		host, port = value, valuePort
	}
	suffix := "." + a.tenantBaseDomain
	if a.tenantBaseDomain == "" || !strings.HasSuffix(host, suffix) {
		return false, nil
	}
	requestedSlug := strings.TrimSuffix(host, suffix)
	if requestedSlug == tenantRecord.Slug || requestedSlug == "" || strings.Contains(requestedSlug, ".") {
		return false, nil
	}
	canonicalHost := tenantRecord.Slug + suffix
	if port != "" {
		canonicalHost = net.JoinHostPort(canonicalHost, port)
	}
	target := "https://" + canonicalHost + c.Request().URL.RequestURI()
	if c.Request().TLS == nil {
		target = "http://" + canonicalHost + c.Request().URL.RequestURI()
	}
	return true, c.Redirect(http.StatusPermanentRedirect, target)
}

func (a *App) resolveMailviewTenantHost(ctx context.Context, rawHost string) (control.Tenant, error) {
	host := strings.ToLower(rawHost)
	if value, _, err := net.SplitHostPort(host); err == nil {
		host = value
	}
	suffix := "." + a.tenantBaseDomain
	if a.tenantBaseDomain != "" && strings.HasSuffix(host, suffix) {
		slug := strings.TrimSuffix(host, suffix)
		if slug != "" && !strings.Contains(slug, ".") {
			return a.mailview.GetTenantBySlug(ctx, slug)
		}
	}
	return a.mailview.GetTenantByVerifiedHostname(ctx, host)
}
