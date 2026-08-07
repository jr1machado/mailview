package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/mailview/control"
	"github.com/knadh/listmonk/internal/mailview/dataplane"
	"github.com/knadh/listmonk/internal/mailview/tenant"
	"github.com/labstack/echo/v4"
)

func (a *App) mailviewTenantContext(c echo.Context) (contextErr error) {
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

func (a *App) mailviewRequestContext(c echo.Context) (context.Context, error) {
	if !a.tenantRoutingEnabled {
		return nil, nil
	}
	host := strings.ToLower(c.Request().Host)
	if value, _, err := net.SplitHostPort(host); err == nil {
		host = value
	}
	suffix := "." + a.tenantBaseDomain
	if a.tenantBaseDomain == "" || !strings.HasSuffix(host, suffix) {
		return nil, echo.NewHTTPError(http.StatusNotFound, "tenant host not found")
	}
	slug := strings.TrimSuffix(host, suffix)
	if slug == "" || strings.Contains(slug, ".") {
		return nil, echo.NewHTTPError(http.StatusNotFound, "tenant host not found")
	}
	tenantRecord, err := a.mailview.GetTenantBySlug(c.Request().Context(), slug)
	if err != nil || tenantRecord.Status != control.TenantStatusActive {
		return nil, echo.NewHTTPError(http.StatusNotFound, "tenant not found")
	}
	u := auth.GetUser(c)
	ok, err := a.mailview.HasActiveMembership(c.Request().Context(), tenantRecord.ID, u.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, auth.ErrPermDenied
	}
	return tenant.WithContext(c.Request().Context(), tenant.Context{TenantID: tenantRecord.ID, UserID: u.ID, RequestID: strings.TrimSpace(c.Request().Header.Get("X-Request-ID"))}), nil
}
