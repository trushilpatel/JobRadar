package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/api/middleware"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/cache"
	"github.com/trushilpatel/jobradar/platform/ratelimit"
	"github.com/trushilpatel/jobradar/platform/supabaseadmin"
	auditsvc "github.com/trushilpatel/jobradar/service/audit/services"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	dashboardsvc "github.com/trushilpatel/jobradar/service/dashboard/services"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
	rolesvc "github.com/trushilpatel/jobradar/service/role/services"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
	"gorm.io/gorm"
)

// multiUser bundles the Phase 2 multi-user services, the /me route group
// (mounted under /api by NewRouter), and the /admin/* route group with all
// admin service handlers.
type multiUser struct {
	userSvc     usersvc.Service
	roleSvc     rolesvc.Service
	auditSvc    auditsvc.Service
	identityMW  func(http.Handler) http.Handler
	rateLimitMW func(http.Handler) http.Handler
	router      handler.RouteGroup // /me routes — mounted under /api by NewRouter
	admin       handler.RouteGroup // /admin/* routes — mounted at root by NewRouter
}

// buildMultiUser composes the user / role / audit decorator chains, wraps
// them in a /api/me RestRouter, and builds the /admin/* subtree with all
// admin handlers behind RequireIdentity + RequireRole("admin") + RateLimit
// + AuditAdminMutation middleware.
func buildMultiUser(
	db *gorm.DB,
	cfg *config.Config,
	c cache.Cache,
	limiter ratelimit.Limiter,
	supabase supabaseadmin.Client,
	log logger.Logger,
	// pipelineSvc is passed in from main so that the admin pipeline handler
	// shares the same PipelineService instance built by the Wire graph.
	pipelineSvc dashboardsvc.PipelineService,
	// insightsRouter is passed in so the insights routes move to /admin.
	insightsRouter *insightssvc.RestRouter,
	// companyAdminRouter mounts /admin/companies CRUD + scan-settings routes.
	companyAdminRouter *companysvc.AdminRestRouter,
) *multiUser {
	// ── User decorator chain ────────────────────────────────────────────────
	userRepo := usersvc.NewRepositoryService(db)
	userImpl := usersvc.NewImplService(userRepo, supabase)
	userCache := usersvc.NewCacheService(userImpl, c, time.Duration(cfg.MultiUser.CacheTTLUserSeconds)*time.Second)
	userValid := usersvc.NewValidationService(userCache)
	userRest := usersvc.NewRestService(userValid)

	// ── Role decorator chain ────────────────────────────────────────────────
	roleRepo := rolesvc.NewRepositoryService(db)
	roleImpl := rolesvc.NewImplService(roleRepo)
	roleValid := rolesvc.NewValidationService(roleImpl)

	// ── Audit decorator chain ───────────────────────────────────────────────
	auditRepo := auditsvc.NewRepositoryService(db)
	auditImpl := auditsvc.NewImplService(auditRepo)

	// ── User admin decorator chain ──────────────────────────────────────────
	userAdminRepo := usersvc.NewAdminRepositoryService(db)
	userAdminImpl := usersvc.NewAdminImplService(userAdminRepo, supabase)
	userAdminValid := usersvc.NewAdminValidationService(userAdminImpl)
	userAdminRest := usersvc.NewAdminRestService(userAdminValid)
	userAdminRouter := usersvc.NewAdminRestRouter(userAdminRest)

	// ── Audit admin REST ────────────────────────────────────────────────────
	auditAdminRest := auditsvc.NewAdminRestService(auditImpl)
	auditAdminRouter := auditsvc.NewAdminRestRouter(auditAdminRest)

	// ── Dashboard pipeline admin REST ───────────────────────────────────────
	pipelineRest := dashboardsvc.NewPipelineRestService(pipelineSvc)
	pipelineRouter := dashboardsvc.NewPipelineRestRouter(pipelineRest)

	// ── Shared middleware ───────────────────────────────────────────────────
	identityMW := middleware.RequireIdentity(
		userValid,
		roleValid,
		c,
		time.Duration(cfg.MultiUser.CacheTTLUserSeconds)*time.Second,
		cfg.MultiUser.AdminEmails,
		log,
	)
	rateLimitMW := middleware.RateLimit(limiter, log)

	// ── /api/me route group ─────────────────────────────────────────────────
	meRouter := usersvc.NewRestRouter(userRest, identityMW, rateLimitMW)

	// ── /admin/* route group ────────────────────────────────────────────────
	requireAdminMW := middleware.RequireRole("admin")
	auditMW := middleware.AuditAdminMutation(auditImpl, log)

	adminGroup := &adminRouteGroup{
		identityMW: identityMW,
		roleMW:     requireAdminMW,
		rateMW:     rateLimitMW,
		auditMW:    auditMW,
		handlers: []handler.RouteGroup{
			userAdminRouter,
			auditAdminRouter,
			pipelineRouter,
			insightsRouter,
			companyAdminRouter,
		},
	}

	return &multiUser{
		userSvc:     userValid,
		roleSvc:     roleValid,
		auditSvc:    auditImpl,
		identityMW:  identityMW,
		rateLimitMW: rateLimitMW,
		router:      meRouter,
		admin:       adminGroup,
	}
}

// IdentityMW returns the RequireIdentity middleware for use on the /api subrouter.
func (m *multiUser) IdentityMW() func(http.Handler) http.Handler { return m.identityMW }

// RateLimitMW returns the RateLimit middleware for use on the /api subrouter.
func (m *multiUser) RateLimitMW() func(http.Handler) http.Handler { return m.rateLimitMW }

// APIHandlers returns route groups for the /api subrouter (currently just /me).
func (m *multiUser) APIHandlers() []handler.RouteGroup {
	if m == nil || m.router == nil {
		return nil
	}
	return []handler.RouteGroup{m.router}
}

// AdminHandlers returns the /admin/* route group for root-level mounting.
func (m *multiUser) AdminHandlers() []handler.RouteGroup {
	if m == nil || m.admin == nil {
		return nil
	}
	return []handler.RouteGroup{m.admin}
}

// adminRouteGroup mounts all admin handlers under /admin with the full
// middleware chain: RequireIdentity → RequireRole("admin") → RateLimit →
// AuditAdminMutation.
type adminRouteGroup struct {
	identityMW func(http.Handler) http.Handler
	roleMW     func(http.Handler) http.Handler
	rateMW     func(http.Handler) http.Handler
	auditMW    func(http.Handler) http.Handler
	handlers   []handler.RouteGroup
}

// Register implements handler.RouteGroup. It creates a /admin subrouter
// with all four middleware applied in order, then delegates to each handler.
func (a *adminRouteGroup) Register(r chi.Router) {
	r.Route("/admin", func(sub chi.Router) {
		if a.identityMW != nil {
			sub.Use(a.identityMW)
		}
		if a.roleMW != nil {
			sub.Use(a.roleMW)
		}
		if a.rateMW != nil {
			sub.Use(a.rateMW)
		}
		if a.auditMW != nil {
			sub.Use(a.auditMW)
		}
		for _, h := range a.handlers {
			h.Register(sub)
		}
	})
}
