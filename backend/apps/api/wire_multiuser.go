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

// buildMultiUser composes the user / role / audit decorator chains, wraps
// them in a /api/me RestRouter, and builds the /admin/* subtree with all
// admin handlers behind RequireIdentity + RequireRole("admin") + RateLimit
// + AuditAdminMutation middleware. Pure wiring; equivalent to wire_gen.go.
func buildMultiUser(
	db *gorm.DB,
	cfg *config.Config,
	c cache.Cache,
	limiter ratelimit.Limiter,
	supabase supabaseadmin.Client,
	log logger.Logger,
	pipelineSvc dashboardsvc.PipelineService,
	insightsRouter *insightssvc.RestRouter,
	companyAdminRouter *companysvc.AdminRestRouter,
) *multiUser {
	userRepo := usersvc.NewRepositoryService(db)
	userImpl := usersvc.NewImplService(userRepo, supabase)
	userCache := usersvc.NewCacheService(userImpl, c, time.Duration(cfg.MultiUser.CacheTTLUserSeconds)*time.Second)
	userValid := usersvc.NewValidationService(userCache)
	userRest := usersvc.NewRestService(userValid)

	roleRepo := rolesvc.NewRepositoryService(db)
	roleImpl := rolesvc.NewImplService(roleRepo)
	roleValid := rolesvc.NewValidationService(roleImpl)

	auditRepo := auditsvc.NewRepositoryService(db)
	auditImpl := auditsvc.NewImplService(auditRepo)

	userAdminRepo := usersvc.NewAdminRepositoryService(db)
	userAdminImpl := usersvc.NewAdminImplService(userAdminRepo, supabase, c)
	userAdminValid := usersvc.NewAdminValidationService(userAdminImpl)
	userAdminRest := usersvc.NewAdminRestService(userAdminValid)
	userAdminRouter := usersvc.NewAdminRestRouter(userAdminRest)

	auditAdminRest := auditsvc.NewAdminRestService(auditImpl)
	auditAdminRouter := auditsvc.NewAdminRestRouter(auditAdminRest)

	pipelineRest := dashboardsvc.NewPipelineRestService(pipelineSvc)
	pipelineRouter := dashboardsvc.NewPipelineRestRouter(pipelineRest)

	identityMW := middleware.RequireIdentity(
		userValid,
		roleValid,
		c,
		time.Duration(cfg.MultiUser.CacheTTLUserSeconds)*time.Second,
		log,
	)
	rateLimitMW := middleware.RateLimit(limiter, log)

	meRouter := usersvc.NewRestRouter(userRest, identityMW, rateLimitMW)

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
