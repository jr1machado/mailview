package main

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/mailview/tenant"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const (
	// stdInputMaxLen is the maximum allowed length for a standard input field.
	stdInputMaxLen = 2000

	// URIs.
	uriAdmin = "/admin"
)

type okResp struct {
	Data any `json:"data"`
}

var (
	reUUID = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
)

// registerHandlers registers HTTP handlers.
func initHTTPHandlers(e *echo.Echo, a *App) {
	// Every request receives a stable correlation ID. The completion log adds
	// tenant/user context when resolution/authentication made it available.
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			requestID := strings.TrimSpace(c.Request().Header.Get("X-Request-ID"))
			if requestID == "" {
				requestID = uuid.Must(uuid.NewV4()).String()
				c.Request().Header.Set("X-Request-ID", requestID)
			}
			c.Response().Header().Set("X-Request-ID", requestID)
			started := time.Now()
			err := next(c)
			userID, tenantID := 0, ""
			if user, ok := c.Get(auth.UserHTTPCtxKey).(auth.User); ok {
				userID = user.ID
			}
			if scoped, ok := c.Get("mailview_tenant_context").(context.Context); ok {
				if value, found := tenant.FromContext(scoped); found {
					tenantID = value.TenantID.String()
					if value.UserID != 0 {
						userID = value.UserID
					}
				}
			}
			a.log.Printf("request_id=%s tenant_id=%s user_id=%d method=%s path=%s status=%d duration=%s", requestID, tenantID, userID, c.Request().Method, c.Request().URL.Path, c.Response().Status, time.Since(started))
			return err
		}
	})

	// Default error handler.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		// Generic, non-echo error. Log it.
		if _, ok := err.(*echo.HTTPError); !ok {
			a.log.Println(err.Error())
		}
		e.DefaultHTTPErrorHandler(err, c)
	}

	// Configure CORS middleware if domains are configured.
	if corsOrigins := trustedURLsToCORSOrigins(a.cfg.Security.TrustedURLs); len(corsOrigins) > 0 {
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: corsOrigins,
			AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
		}))
	}
	e.Use(a.blockGlobalAPIsOnTenantHost)

	// =================================================================
	// Authenticated non /api handlers.
	{
		// Attach a middleware to the group that checks for auth.
		g := e.Group("", a.auth.Middleware, func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				u := c.Get(auth.UserHTTPCtxKey)

				// On no-auth, redirect to login page
				if _, ok := u.(*echo.HTTPError); ok {
					u, _ := url.Parse(a.urlCfg.LoginURL)
					q := url.Values{}
					q.Set("next", c.Request().RequestURI)
					u.RawQuery = q.Encode()
					return c.Redirect(http.StatusTemporaryRedirect, u.String())
				}

				return next(c)
			}
		})

		// Authenticated endpoints.
		g.GET(path.Join(uriAdmin, ""), a.AdminPage)
		g.GET(path.Join(uriAdmin, "/custom.css"), serveCustomAppearance("admin.custom_css"))
		g.GET(path.Join(uriAdmin, "/custom.js"), serveCustomAppearance("admin.custom_js"))
		g.GET(path.Join(uriAdmin, "/*"), a.AdminPage)
	}

	// =================================================================
	// Authenticated /api/* handlers.
	{
		var (
			// Permission check middleware.
			pm = a.auth.Perm

			// Attach a middleware to the group that checks for auth.
			g = e.Group("", a.auth.Middleware, func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c echo.Context) error {
					u := c.Get(auth.UserHTTPCtxKey)

					// On no-auth, respond with a JSON error.
					if err, ok := u.(*echo.HTTPError); ok {
						return err
					}

					return next(c)
				}
			})
		)

		// API endpoints.
		g.GET("/api/health", a.HealthCheck)
		g.GET("/api/config", a.GetServerConfig)
		g.GET("/api/lang/:lang", a.GetI18nLang)
		g.GET("/api/dashboard/charts", a.mailviewDataPerm(a.GetDashboardCharts, "analytics.read.tenant"))
		g.GET("/api/dashboard/counts", a.mailviewDataPerm(a.GetDashboardCounts, "analytics.read.tenant"))
		g.GET("/api/mailview/home", a.GetMailViewTenantHome)
		g.GET("/api/mailview/api-keys", a.ListMailViewAPIKeys)
		g.POST("/api/mailview/api-keys", a.CreateMailViewAPIKey)
		g.DELETE("/api/mailview/api-keys/:keyID", a.RevokeMailViewAPIKey)
		g.GET("/api/mailview/contacts/:id/governance", a.GetMailViewContactGovernance)
		g.PUT("/api/mailview/contacts/:id/governance", a.UpdateMailViewContactGovernance)

		g.GET("/api/settings", pm(a.GetSettings, "settings:get"))
		g.PUT("/api/settings", pm(a.UpdateSettings, "settings:manage"))
		g.PUT("/api/settings/:key", pm(a.UpdateSettingsByKey, "settings:manage"))
		g.POST("/api/settings/smtp/test", pm(a.TestSMTPSettings, "settings:manage"))
		g.POST("/api/admin/reload", pm(a.ReloadApp, "settings:manage"))
		g.GET("/api/logs", pm(a.GetLogs, "settings:get"))
		g.GET("/api/events", pm(a.EventStream, "settings:get"))
		g.GET("/api/about", a.GetAboutInfo)

		g.GET("/api/subscribers", a.mailviewDataPerm(a.QuerySubscribers, "subscriber.read.tenant", "subscribers:get_all", "subscribers:get"))
		g.GET("/api/subscribers/:id", a.mailviewDataPerm(hasID(a.GetSubscriber), "subscriber.read.tenant", "subscribers:get_all", "subscribers:get"))
		g.GET("/api/subscribers/:id/activity", a.mailviewDataPerm(hasID(a.GetSubscriberActivity), "analytics.read.tenant", "subscribers:get_all", "subscribers:get"))
		g.GET("/api/subscribers/:id/export", a.mailviewDataPerm(hasID(a.ExportSubscriberData), "subscriber.export.tenant", "subscribers:get_all", "subscribers:get"))
		g.GET("/api/subscribers/:id/bounces", a.mailviewDataPerm(hasID(a.GetSubscriberBounces), "bounce.read.tenant", "bounces:get"))
		g.DELETE("/api/subscribers/:id/bounces", a.mailviewDataPerm(hasID(a.DeleteSubscriberBounces), "bounce.manage.tenant", "bounces:manage"))
		g.POST("/api/subscribers", a.mailviewDataPerm(a.CreateSubscriber, "subscriber.manage.tenant", "subscribers:manage"))
		g.PUT("/api/subscribers/:id", a.mailviewDataPerm(hasID(a.UpdateSubscriber), "subscriber.manage.tenant", "subscribers:manage"))
		g.PATCH("/api/subscribers/:id", a.mailviewDataPerm(hasID(a.PatchSubscriber), "subscriber.manage.tenant", "subscribers:manage"))
		g.POST("/api/subscribers/:id/optin", a.mailviewDataPerm(hasID(a.SubscriberSendOptin), "subscriber.manage.tenant", "subscribers:manage"))
		g.PUT("/api/subscribers/blocklist", a.mailviewDataPerm(a.BlocklistSubscribers, "subscriber.manage.tenant", "subscribers:manage"))
		g.PUT("/api/subscribers/:id/blocklist", a.mailviewDataPerm(hasID(a.BlocklistSubscriber), "subscriber.manage.tenant", "subscribers:manage"))
		g.PUT("/api/subscribers/lists/:id", a.mailviewDataPerm(a.ManageSubscriberLists, "subscriber.manage.tenant", "subscribers:manage"))
		g.PUT("/api/subscribers/lists", a.mailviewDataPerm(a.ManageSubscriberLists, "subscriber.manage.tenant", "subscribers:manage"))
		g.DELETE("/api/subscribers/:id", a.mailviewDataPerm(hasID(a.DeleteSubscriber), "subscriber.manage.tenant", "subscribers:manage"))
		g.DELETE("/api/subscribers", a.mailviewDataPerm(a.DeleteSubscribers, "subscriber.manage.tenant", "subscribers:manage"))

		g.GET("/api/bounces", a.mailviewDataPerm(a.GetBounces, "bounce.read.tenant", "bounces:get"))
		g.PUT("/api/bounces/blocklist", a.mailviewDataPerm(a.BlocklistBouncedSubscribers, "bounce.manage.tenant", "bounces:manage"))
		g.GET("/api/bounces/:id", a.mailviewDataPerm(hasID(a.GetBounce), "bounce.read.tenant", "bounces:get"))
		g.DELETE("/api/bounces", a.mailviewDataPerm(a.DeleteBounces, "bounce.manage.tenant", "bounces:manage"))
		g.DELETE("/api/bounces/:id", a.mailviewDataPerm(hasID(a.DeleteBounce), "bounce.manage.tenant", "bounces:manage"))

		// Subscriber operations based on arbitrary SQL queries.
		// These aren't very REST-like.
		g.POST("/api/subscribers/query/delete", a.mailviewDataPerm(a.DeleteSubscribersByQuery, "subscriber.manage.tenant", "subscribers:manage"))
		g.PUT("/api/subscribers/query/blocklist", a.mailviewDataPerm(a.BlocklistSubscribersByQuery, "subscriber.manage.tenant", "subscribers:manage"))
		g.PUT("/api/subscribers/query/lists", a.mailviewDataPerm(a.ManageSubscriberListsByQuery, "subscriber.manage.tenant", "subscribers:manage"))
		g.GET("/api/subscribers/export",
			a.mailviewDataPerm(middleware.GzipWithConfig(middleware.GzipConfig{Level: 9})(a.ExportSubscribers), "subscriber.export.tenant", "subscribers:get_all", "subscribers:get"))

		g.GET("/api/import/subscribers", a.mailviewDataPerm(a.GetImportSubscribers, "subscriber.import.tenant", "subscribers:import"))
		g.GET("/api/import/subscribers/logs", a.mailviewDataPerm(a.GetImportSubscriberStats, "subscriber.import.tenant", "subscribers:import"))
		g.POST("/api/import/subscribers", a.mailviewDataPerm(a.ImportSubscribers, "subscriber.import.tenant", "subscribers:import"))
		g.DELETE("/api/import/subscribers", a.mailviewDataPerm(a.StopImportSubscribers, "subscriber.import.tenant", "subscribers:import"))
		g.GET("/api/mailview/data/import-jobs", a.mailviewDataPerm(a.ListMailViewImportJobs, "subscriber.import.tenant"))
		g.POST("/api/mailview/data/import-jobs", a.mailviewDataPerm(a.CreateMailViewImportJob, "subscriber.import.tenant"))
		g.GET("/api/mailview/data/import-jobs/:jobID", a.mailviewDataPerm(a.GetMailViewImportJob, "subscriber.import.tenant"))
		g.POST("/api/mailview/data/import-jobs/:jobID/cancel", a.mailviewDataPerm(a.CancelMailViewImportJob, "subscriber.import.tenant"))

		// Individual list permissions are applied directly within handleGetLists.
		g.GET("/api/lists", a.mailviewDataPerm(a.GetLists, "list.read.tenant"))
		g.GET("/api/lists/:id", a.mailviewDataPerm(hasID(a.GetList), "list.read.tenant"))
		g.POST("/api/lists", a.mailviewDataPerm(a.CreateList, "list.manage.tenant", "lists:manage_all"))
		g.PUT("/api/lists/:id", a.mailviewDataPerm(hasID(a.UpdateList), "list.manage.tenant"))
		g.DELETE("/api/lists", a.mailviewDataPerm(a.DeleteLists, "list.manage.tenant"))
		g.DELETE("/api/lists/:id", a.mailviewDataPerm(hasID(a.DeleteList), "list.manage.tenant"))

		g.GET("/api/campaigns", a.mailviewDataPerm(a.GetCampaigns, "campaign.read.tenant", "campaigns:get_all", "campaigns:get"))
		g.GET("/api/campaigns/running/stats", a.mailviewDataPerm(a.GetRunningCampaignStats, "campaign.read.tenant", "campaigns:get_all", "campaigns:get"))
		g.GET("/api/campaigns/:id", a.mailviewDataPerm(hasID(a.GetCampaign), "campaign.read.tenant", "campaigns:get_all", "campaigns:get"))
		g.GET("/api/campaigns/analytics/:type", a.mailviewDataPerm(a.GetCampaignViewAnalytics, "analytics.read.tenant", "campaigns:get_analytics"))
		g.GET("/api/campaigns/:id/preview", a.mailviewDataPerm(hasID(a.PreviewCampaign), "campaign.read.tenant", "campaigns:get_all", "campaigns:get"))
		g.POST("/api/campaigns/:id/preview/archive", a.mailviewDataPerm(hasID(a.PreviewCampaignArchive), "campaign.read.tenant", "campaigns:get_all", "campaigns:get"))
		g.POST("/api/campaigns/:id/preview", a.mailviewDataPerm(hasID(a.PreviewCampaign), "campaign.read.tenant", "campaigns:get_all", "campaigns:get"))
		g.POST("/api/campaigns/:id/content", a.mailviewDataPerm(hasID(a.CampaignContent), "campaign.manage.tenant", "campaigns:manage_all", "campaigns:manage"))
		g.POST("/api/campaigns/:id/text", a.mailviewDataPerm(hasID(a.PreviewCampaign), "campaign.read.tenant", "campaigns:get"))
		g.POST("/api/campaigns/:id/test", a.mailviewDataPerm(hasID(a.TestCampaign), "campaign.manage.tenant", "campaigns:manage_all", "campaigns:manage"))
		g.POST("/api/campaigns", a.mailviewDataPerm(a.CreateCampaign, "campaign.manage.tenant", "campaigns:manage_all", "campaigns:manage"))
		g.PUT("/api/campaigns/:id", a.mailviewDataPerm(hasID(a.UpdateCampaign), "campaign.manage.tenant", "campaigns:manage_all", "campaigns:manage"))
		g.PUT("/api/campaigns/:id/status", a.mailviewDataPerm(hasID(a.UpdateCampaignStatus), "campaign.send.tenant", "campaigns:send"))
		g.PUT("/api/campaigns/:id/archive", a.mailviewDataPerm(hasID(a.UpdateCampaignArchive), "campaign.manage.tenant", "campaigns:manage_all", "campaigns:manage"))
		g.DELETE("/api/campaigns", a.mailviewDataPerm(a.DeleteCampaigns, "campaign.manage.tenant", "campaigns:manage", "campaigns:manage_all"))
		g.DELETE("/api/campaigns/:id", a.mailviewDataPerm(hasID(a.DeleteCampaign), "campaign.manage.tenant", "campaigns:manage_all", "campaigns:manage"))
		g.GET("/api/mailview/campaigns/:id/workflow", a.GetMailViewCampaignWorkflow)
		g.POST("/api/mailview/campaigns/:id/workflow/transitions", a.TransitionMailViewCampaign)

		g.GET("/api/media", a.mailviewDataPerm(a.GetAllMedia, "media.read.tenant", "media:get"))
		g.GET("/api/media/:id", a.mailviewDataPerm(hasID(a.GetMedia), "media.read.tenant", "media:get"))
		g.POST("/api/media", a.mailviewDataPerm(a.UploadMedia, "media.manage.tenant", "media:manage"))
		g.DELETE("/api/media/:id", a.mailviewDataPerm(hasID(a.DeleteMedia), "media.manage.tenant", "media:manage"))

		g.GET("/api/templates", a.mailviewDataPerm(a.GetTemplates, "template.read.tenant", "templates:get"))
		g.GET("/api/templates/:id", a.mailviewDataPerm(hasID(a.GetTemplate), "template.read.tenant", "templates:get"))
		g.GET("/api/templates/:id/preview", a.mailviewDataPerm(hasID(a.PreviewTemplate), "template.read.tenant", "templates:get"))
		g.POST("/api/templates/preview", a.mailviewDataPerm(a.PreviewTemplateBody, "template.read.tenant", "templates:get"))
		g.POST("/api/templates", a.mailviewDataPerm(a.CreateTemplate, "template.manage.tenant", "templates:manage"))
		g.PUT("/api/templates/:id", a.mailviewDataPerm(hasID(a.UpdateTemplate), "template.manage.tenant", "templates:manage"))
		g.PUT("/api/templates/:id/default", a.mailviewDataPerm(hasID(a.TemplateSetDefault), "template.manage.tenant", "templates:manage"))
		g.DELETE("/api/templates/:id", a.mailviewDataPerm(hasID(a.DeleteTemplate), "template.manage.tenant", "templates:manage"))

		g.DELETE("/api/maintenance/subscribers/:type", pm(a.GCSubscribers, "settings:maintain"))
		g.DELETE("/api/maintenance/analytics/:type", pm(a.GCCampaignAnalytics, "settings:maintain"))
		g.GET("/api/maintenance/analytics/:type/export", pm(a.ExportCampaignAnalytics, "settings:maintain"))
		g.DELETE("/api/maintenance/subscriptions/unconfirmed", pm(a.GCSubscriptions, "settings:maintain"))

		g.POST("/api/tx", pm(a.SendTxMessage, "tx:send"))

		g.GET("/api/profile", a.GetUserProfile)
		g.PUT("/api/profile", a.UpdateUserProfile)
		g.GET("/api/users", pm(a.GetUsers, "users:get"))
		g.GET("/api/users/:id", pm(hasID(a.GetUser), "users:get"))
		g.POST("/api/users", pm(a.CreateUser, "users:manage"))
		g.PUT("/api/users/:id", pm(hasID(a.UpdateUser), "users:manage"))
		g.DELETE("/api/users", pm(a.DeleteUsers, "users:manage"))
		g.DELETE("/api/users/:id", pm(hasID(a.DeleteUser), "users:manage"))
		g.POST("/api/logout", a.Logout)

		// TOTP 2FA endpoints
		g.GET("/api/users/:id/twofa/totp", hasID(a.GenerateTOTPQR))
		g.PUT("/api/users/:id/twofa", hasID(a.EnableTOTP))
		g.DELETE("/api/users/:id/twofa", hasID(a.DisableTOTP))

		g.GET("/api/roles/users", pm(a.GetUserRoles, "roles:get"))
		g.GET("/api/roles/lists", pm(a.GeListRoles, "roles:get"))
		g.POST("/api/roles/users", pm(a.CreateUserRole, "roles:manage"))
		g.POST("/api/roles/lists", pm(a.CreateListRole, "roles:manage"))
		g.PUT("/api/roles/users/:id", pm(hasID(a.UpdateUserRole), "roles:manage"))
		g.PUT("/api/roles/lists/:id", pm(hasID(a.UpdateListRole), "roles:manage"))
		g.DELETE("/api/roles/:id", pm(hasID(a.DeleteRole), "roles:manage"))

		// MailView Control Plane. Endpoints accept the legacy platform Super
		// Admin or a user holding the matching MailView platform role.
		cp := g.Group("/api/mailview", a.requirePlatformAdmin)
		cp.GET("/tenants", a.ListMailViewTenants)
		cp.POST("/tenants", a.CreateMailViewTenant)
		cp.GET("/tenants/:tenantID", a.GetMailViewTenant)
		cp.PATCH("/tenants/:tenantID", a.UpdateMailViewTenantStatus)
		cp.POST("/tenants/:tenantID/slug", a.ChangeMailViewTenantSlug)
		cp.GET("/tenants/:tenantID/roles", a.ListMailViewRoles)
		cp.GET("/tenants/:tenantID/memberships", a.ListMailViewMemberships)
		cp.POST("/tenants/:tenantID/memberships", a.CreateMailViewMembership)
		cp.PUT("/tenants/:tenantID/memberships/:membershipID/roles", a.ReplaceMailViewMembershipRoles)
		cp.GET("/tenants/:tenantID/audit-events", a.ListMailViewAuditEvents)
		cp.GET("/tenants/:tenantID/data/lists", a.ListMailViewLists)
		cp.POST("/tenants/:tenantID/data/lists", a.CreateMailViewList)
		cp.GET("/tenants/:tenantID/data/subscribers", a.ListMailViewSubscribers)
		cp.POST("/tenants/:tenantID/data/subscribers", a.CreateMailViewSubscriber)
		cp.GET("/tenants/:tenantID/data/import-jobs", a.ListMailViewImportJobs)
		cp.POST("/tenants/:tenantID/data/import-jobs", a.CreateMailViewImportJob)
		cp.GET("/tenants/:tenantID/data/import-jobs/:jobID", a.GetMailViewImportJob)
		cp.POST("/tenants/:tenantID/data/import-jobs/:jobID/cancel", a.CancelMailViewImportJob)

		cp.GET("/tenants/:tenantID/domains", a.ListMailViewTenantDomains)
		cp.POST("/tenants/:tenantID/domains", a.CreateMailViewTenantDomain)
		cp.POST("/tenants/:tenantID/domains/:domainID/verify", a.VerifyMailViewTenantDomain)
		cp.POST("/tenants/:tenantID/domains/:domainID/revoke", a.RevokeMailViewTenantDomain)
		cp.POST("/tenants/:tenantID/domains/:domainID/tls", a.SetMailViewTenantDomainTLSStatus)
		cp.POST("/domains/revalidate", a.RevalidateMailViewTenantDomains)
		cp.GET("/tenants/:tenantID/quota", a.GetMailViewTenantQuota)
		cp.PUT("/tenants/:tenantID/quota", a.SetMailViewTenantQuotaPlan)
		cp.GET("/plans", a.ListMailViewTenantPlans)
		cp.GET("/dashboard", a.GetMailViewDashboard)
		cp.POST("/tenants/:tenantID/owner", a.ResetMailViewTenantOwner)
		cp.POST("/tenants/:tenantID/infrastructure", a.SetMailViewTenantInfrastructure)
		cp.GET("/tenants/:tenantID/infrastructure", a.GetMailViewTenantInfrastructure)
		cp.POST("/tenants/:tenantID/roles", a.CreateMailViewTenantRole)
		cp.POST("/tenants/:tenantID/roles/:roleID/permissions/:code/deny", a.DenyMailViewRolePermission)
		cp.DELETE("/tenants/:tenantID/roles/:roleID/permissions/:code/deny", a.AllowMailViewRolePermission)

		// Platform role assignment is gated by platform.roles.manage, a
		// narrower permission than tenant.manage.platform: an Operations
		// role holder can manage tenants but cannot grant itself Super Admin.
		cpRoles := g.Group("/api/mailview/platform", a.requirePlatformRoleAdmin)
		cpRoles.GET("/roles", a.ListMailViewPlatformRoles)
		cpRoles.GET("/assignments", a.ListMailViewPlatformAssignments)
		cpRoles.POST("/assignments", a.AssignMailViewPlatformRole)
		cpRoles.DELETE("/assignments/:userID/:roleID", a.RevokeMailViewPlatformRole)

		// Impersonation is gated by support.impersonate.platform, narrower
		// still: an Operations/Billing role holder cannot act as a tenant
		// member even though they can manage tenants.
		cpImpersonation := g.Group("/api/mailview/platform/impersonation", a.requireImpersonationAdmin)
		cpImpersonation.POST("", a.StartMailViewImpersonation)
		cpImpersonation.GET("", a.ListMailViewImpersonationGrants)
		cpImpersonation.POST("/:grantID/revoke", a.RevokeMailViewImpersonation)
		cpImpersonation.POST("/:grantID/approve", a.ApproveMailViewImpersonation)

		cpIncidents := g.Group("/api/mailview/platform/incidents", a.requirePlatformIncidentAdmin)
		cpIncidents.GET("", a.ListMailViewIncidents)
		cpIncidents.POST("", a.CreateMailViewIncident)
		cpIncidents.POST("/:incidentID/resolve", a.ResolveMailViewIncident)

		// Available to any authenticated user with TOTP enabled. Codes are
		// displayed only in this response and persisted as bcrypt hashes.
		g.POST("/api/mailview/profile/mfa/recovery-codes", a.GenerateMailViewRecoveryCodes)

		if a.cfg.BounceWebhooksEnabled {
			// Private authenticated bounce endpoint.
			g.POST("/webhooks/bounce", pm(a.BounceWebhook, "webhooks:post_bounce"))
		}
	}

	// =================================================================
	// Public API endpoints.
	{
		// Public unauthenticated endpoints.
		g := e.Group("")

		if a.cfg.BounceWebhooksEnabled {
			// Public bounce endpoints for webservices like SES.
			g.POST("/webhooks/service/:service", a.BounceWebhook)
		}

		// Landing page.
		g.GET("/", func(c echo.Context) error {
			return c.Render(http.StatusOK, "home", publicTpl{Title: "listmonk"})
		})

		// Public admin endpoints (login page, OIDC endpoints, password reset).
		g.GET(path.Join(uriAdmin, "/login"), a.LoginPage)
		g.POST(path.Join(uriAdmin, "/login"), a.LoginPage)
		g.GET(path.Join(uriAdmin, "/login/twofa"), a.TwofaPage)
		g.POST(path.Join(uriAdmin, "/login/twofa"), a.TwofaPage)
		g.GET(path.Join(uriAdmin, "/forgot"), a.ForgotPage)
		g.POST(path.Join(uriAdmin, "/forgot"), a.ForgotPage)
		g.GET(path.Join(uriAdmin, "/reset"), a.ResetPage)
		g.POST(path.Join(uriAdmin, "/reset"), a.ResetPage)

		if a.cfg.Security.OIDC.Enabled {
			g.POST("/auth/oidc", a.OIDCLogin)
			g.GET("/auth/oidc", a.OIDCFinish)
		}

		// Public APIs.
		g.GET("/api/public/lists", a.mailviewPublicTenant(a.GetPublicLists))
		g.POST("/api/public/subscription", a.mailviewPublicTenant(a.PublicSubscription))
		g.GET("/api/public/captcha/altcha", a.AltchaChallenge)
		if a.cfg.EnablePublicArchive {
			g.GET("/api/public/archive", a.mailviewPublicTenant(a.GetCampaignArchives))
		}

		// /public/static/* file server is registered in initHTTPServer().
		// Public subscriber facing views.
		g.GET("/subscription/form", a.mailviewPublicTenant(a.SubscriptionFormPage))
		g.POST("/subscription/form", a.mailviewPublicTenant(a.SubscriptionForm))
		g.GET("/subscription/:campUUID/:subUUID", a.mailviewPublicTenant(noIndex(a.hasUUID(a.hasSub(a.SubscriptionPage), "campUUID", "subUUID"))))
		g.POST("/subscription/:campUUID/:subUUID", a.mailviewPublicTenant(a.hasUUID(a.hasSub(a.SubscriptionPrefs), "campUUID", "subUUID")))
		g.GET("/subscription/optin/:subUUID", a.mailviewPublicTenant(noIndex(a.hasUUID(a.hasSub(a.OptinPage), "subUUID"))))
		g.POST("/subscription/optin/:subUUID", a.mailviewPublicTenant(a.hasUUID(a.hasSub(a.OptinPage), "subUUID")))
		g.POST("/subscription/export/:subUUID", a.mailviewPublicTenant(a.hasUUID(a.hasSub(a.SelfExportSubscriberData), "subUUID")))
		g.POST("/subscription/wipe/:subUUID", a.mailviewPublicTenant(a.hasUUID(a.hasSub(a.WipeSubscriberData), "subUUID")))
		g.GET("/link/:linkUUID/:campUUID/:subUUID", a.mailviewPublicTenant(noIndex(a.hasUUID(a.LinkRedirect, "linkUUID", "campUUID", "subUUID"))))
		g.GET("/campaign/:campUUID/:subUUID", a.mailviewPublicTenant(noIndex(a.hasUUID(a.ViewCampaignMessage, "campUUID", "subUUID"))))
		g.GET("/campaign/:campUUID/:subUUID/px.png", a.mailviewPublicTenant(noIndex(a.hasUUID(a.RegisterCampaignView, "campUUID", "subUUID"))))

		if a.cfg.EnablePublicArchive {
			g.GET("/archive", a.mailviewPublicTenant(a.CampaignArchivesPage))
			g.GET("/archive.xml", a.mailviewPublicTenant(a.GetCampaignArchivesFeed))
			g.GET("/archive/:id", a.mailviewPublicTenant(a.CampaignArchivePage))
			g.GET("/archive/latest", a.mailviewPublicTenant(a.CampaignArchivePageLatest))
		}

		g.GET("/public/custom.css", serveCustomAppearance("public.custom_css"))
		g.GET("/public/custom.js", serveCustomAppearance("public.custom_js"))

		// Public health API endpoint.
		g.GET("/health", a.HealthCheck)
		g.GET("/robots.txt", a.RobotsTxt)

		// 404 pages.
		g.RouteNotFound("/*", func(c echo.Context) error {
			return c.Render(http.StatusNotFound, tplMessage,
				makeMsgTpl("404 - "+a.i18n.T("public.notFoundTitle"), "", ""))
		})
		g.RouteNotFound("/api/*", func(c echo.Context) error {
			return echo.NewHTTPError(http.StatusNotFound, "404 unknown endpoint")
		})
		g.RouteNotFound("/admin/*", func(c echo.Context) error {
			return echo.NewHTTPError(http.StatusNotFound, "404 page not found")
		})
	}
}

// AdminPage is the root handler that renders the Javascript admin frontend.
func (a *App) AdminPage(c echo.Context) error {
	b, err := a.fs.Read(path.Join(uriAdmin, "/index.html"))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	b = bytes.ReplaceAll(b, []byte("asset_version"), []byte(a.cfg.AssetVersion))

	return c.HTMLBlob(http.StatusOK, b)
}

// HealthCheck is a healthcheck endpoint that returns a 200 response.
func (a *App) HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, okResp{true})
}

// RobotsTxt serves the robots.txt file from the static filesystem.
func (a *App) RobotsTxt(c echo.Context) error {
	b, err := a.fs.Read("/public/static/robots.txt")
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "robots.txt not found")
	}
	return c.Blob(http.StatusOK, "text/plain; charset=utf-8", b)
}

// serveCustomAppearance serves the given custom CSS/JS appearance blob
// meant for customizing public and admin pages from the admin settings UI.
func serveCustomAppearance(name string) echo.HandlerFunc {
	return func(c echo.Context) error {
		var (
			app = c.Get("app").(*App)

			out []byte
			hdr string
		)

		switch name {
		case "admin.custom_css":
			out = app.cfg.Appearance.AdminCSS
			hdr = "text/css; charset=utf-8"

		case "admin.custom_js":
			out = app.cfg.Appearance.AdminJS
			hdr = "application/javascript; charset=utf-8"

		case "public.custom_css":
			out = app.cfg.Appearance.PublicCSS
			hdr = "text/css; charset=utf-8"

		case "public.custom_js":
			out = app.cfg.Appearance.PublicJS
			hdr = "application/javascript; charset=utf-8"
		}

		return c.Blob(http.StatusOK, hdr, out)
	}
}

// hasUUID middleware validates the UUID string format for a given set of params.
func (a *App) hasUUID(next echo.HandlerFunc, params ...string) echo.HandlerFunc {
	return func(c echo.Context) error {
		for _, p := range params {
			if !reUUID.MatchString(c.Param(p)) {
				return c.Render(http.StatusBadRequest, tplMessage, makeMsgTpl(a.i18n.T("public.errorTitle"), "",
					a.i18n.T("globals.messages.invalidUUID")))
			}
		}
		return next(c)
	}
}

// hasID middleware validates the :id param in the URL and sets its int value in the context.
func hasID(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, _ := strconv.Atoi(c.Param("id"))
		if id < 1 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid ID")
		}

		c.Set("id", id)
		return next(c)
	}
}

// hasSub middleware checks if a subscriber exists given the UUID
// param in a request.
func (a *App) hasSub(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		subUUID := c.Param("subUUID")

		if _, err := a.mailviewCore(c).GetSubscriber(0, subUUID, ""); err != nil {
			if er, ok := err.(*echo.HTTPError); ok && er.Code == http.StatusBadRequest {
				return c.Render(http.StatusNotFound, tplMessage,
					makeMsgTpl(a.i18n.T("public.notFoundTitle"), "", er.Message.(string)))
			}

			a.log.Printf("error checking subscriber existence: %v", err)
			return c.Render(http.StatusInternalServerError, tplMessage,
				makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("public.errorProcessingRequest")))
		}

		return next(c)
	}
}

// noIndex adds the HTTP header requesting robots to not crawl the page.
func noIndex(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set("X-Robots-Tag", "noindex")
		return next(c)
	}
}

// getID returns the :id param from the URL parsed and stored as an int by the hasID middleware.
func getID(c echo.Context) int {
	return c.Get("id").(int)
}

// trustedURLsToCORSOrigins takes a list of trusted URLs and returns a list of
// unique origin domains to be used in CORS middleware configuration, including '*' if it exists.
func trustedURLsToCORSOrigins(urls []string) []string {
	mp := map[string]struct{}{}

	for _, s := range urls {
		if s == "*" {
			mp[s] = struct{}{}
		}

		u, err := url.ParseRequestURI(s)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		s = u.Scheme + "://" + u.Host
		mp[s] = struct{}{}
	}

	out := make([]string, 0, len(mp))
	for u := range mp {
		out = append(out, u)
	}

	return out
}
