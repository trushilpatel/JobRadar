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
	dashboardsvc "github.com/trushilpatel/jobradar/service/dashboard/services"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
	rolesvc "github.com/trushilpatel/jobradar/service/role/services"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
	"gorm.io/gorm"
)

// multiUser bundles the Phase 1 multi-user services, the /api/me route
// group, and the /admin/* route group with all admin service handlers.
type multiUser struct {
	userSvc  usersvc.Service
	roleSvc  rolesvc.Service
	auditSvc auditsvc.Service
	router   handler.RouteGroup
	admin    handler.RouteGroup
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
		},
	}

	return &multiUser{
		userSvc:  userValid,
		roleSvc:  roleValid,
		auditSvc: auditImpl,
		router:   meRouter,
		admin:    adminGroup,
	}
}

// Handlers returns the /api/me and /admin/* route groups for mounting
// alongside the rest of the App's handlers.
func (m *multiUser) Handlers() []handler.RouteGroup {
	if m == nil {
		return nil
	}
	groups := make([]handler.RouteGroup, 0, 2)
	if m.router != nil {
		groups = append(groups, m.router)
	}
	if m.admin != nil {
		groups = append(groups, m.admin)
	}
	return groups
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
