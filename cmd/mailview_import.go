package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/knadh/listmonk/internal/mailview/importjob"
	"github.com/knadh/listmonk/internal/mailview/tenant"
	"github.com/labstack/echo/v4"
)

// CreateMailViewImportJob uploads a CSV of subscribers for the tenant and
// starts the worker in the background, mirroring how the legacy
// ImportSubscribers handler kicks off subimporter sessions asynchronously.
func (a *App) CreateMailViewImportJob(c echo.Context) error {
	if err := a.mailviewTenantContext(c); err != nil {
		return mailviewHTTPError(err)
	}
	tenantCtx := c.Get("mailview_tenant_context").(context.Context)

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	in := importjob.CreateJobInput{IdempotencyKey: c.FormValue("idempotency_key")}
	for _, raw := range strings.Split(c.FormValue("list_ids"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		id, err := strconv.Atoi(raw)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid list_ids")
		}
		in.ListIDs = append(in.ListIDs, id)
	}

	job, err := a.importJobs.CreateJob(tenantCtx, in, src)
	if err != nil {
		return importJobHTTPError(err)
	}

	if job.Status == importjob.StatusPending {
		// Detach from the request context so the worker survives past the
		// response, carrying only the tenant scope forward.
		if scope, ok := tenant.FromContext(tenantCtx); ok {
			bgCtx := tenant.WithContext(context.Background(), scope)
			go func() {
				_ = a.importJobs.ProcessJob(bgCtx, job.ID)
			}()
		}
	}

	return c.JSON(http.StatusCreated, okResp{Data: job})
}

func (a *App) ListMailViewImportJobs(c echo.Context) error {
	if err := a.mailviewTenantContext(c); err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.importJobs.ListJobs(c.Get("mailview_tenant_context").(context.Context))
	if err != nil {
		return importJobHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) GetMailViewImportJob(c echo.Context) error {
	if err := a.mailviewTenantContext(c); err != nil {
		return mailviewHTTPError(err)
	}
	id, err := mailviewUUIDParam(c, "jobID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	out, err := a.importJobs.GetJob(c.Get("mailview_tenant_context").(context.Context), id)
	if err != nil {
		return importJobHTTPError(err)
	}
	return c.JSON(http.StatusOK, okResp{Data: out})
}

func (a *App) CancelMailViewImportJob(c echo.Context) error {
	if err := a.mailviewTenantContext(c); err != nil {
		return mailviewHTTPError(err)
	}
	id, err := mailviewUUIDParam(c, "jobID")
	if err != nil {
		return mailviewHTTPError(err)
	}
	if err := a.importJobs.CancelJob(c.Get("mailview_tenant_context").(context.Context), id); err != nil {
		return importJobHTTPError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func importJobHTTPError(err error) error {
	switch {
	case errors.Is(err, importjob.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, importjob.ErrInvalid):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, importjob.ErrSigningUnavailable):
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	default:
		return err
	}
}
