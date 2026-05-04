package main

import (
	"time"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/api/middleware"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/cache"
	"github.com/trushilpatel/jobradar/platform/ratelimit"
	"github.com/trushilpatel/jobradar/platform/supabaseadmin"
	auditsvc "github.com/trushilpatel/jobradar/service/audit/services"
	rolesvc "github.com/trushilpatel/jobradar/service/role/services"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
	"gorm.io/gorm"
)

// multiUser bundles the Phase 1 multi-user services and the /api/me route
// group built around them. Kept out of the Wire graph because its
// construction takes runtime-built middleware (RequireIdentity, RateLimit)
// that depends on cache/limiter instances also built at runtime.
type multiUser struct {
	userSvc  usersvc.Service
	roleSvc  rolesvc.Service
	auditSvc auditsvc.Service
	router   handler.RouteGroup
}

// buildMultiUser composes the user / role / audit decorator chains, wraps
// them in a /api/me RestRouter with the RequireIdentity + RateLimit
// middleware applied at the subtree level, and returns the bundle for
// mounting onto the public router.
//
// The order is deliberate:
//
//   - User chain: Repository → Impl(supabase) → Cache → Validation → Rest.
//   - Role chain: Repository → Impl → Validation.
//   - Audit chain: Repository → Impl.
//
// All three are constructed before the middleware factory call so the
// middleware can reach the user/role services it depends on.
func buildMultiUser(
	db *gorm.DB,
	cfg *config.Config,
	c cache.Cache,
	limiter ratelimit.Limiter,
	supabase supabaseadmin.Client,
	log logger.Logger,
) *multiUser {
	// User decorator chain.
	userRepo := usersvc.NewRepositoryService(db)
	userImpl := usersvc.NewImplService(userRepo, supabase)
	userCache := usersvc.NewCacheService(userImpl, c, time.Duration(cfg.MultiUser.CacheTTLUserSeconds)*time.Second)
	userValid := usersvc.NewValidationService(userCache)
	userRest := usersvc.NewRestService(userValid)

	// Role decorator chain.
	roleRepo := rolesvc.NewRepositoryService(db)
	roleImpl := rolesvc.NewImplService(roleRepo)
	roleValid := rolesvc.NewValidationService(roleImpl)

	// Audit decorator chain.
	auditRepo := auditsvc.NewRepositoryService(db)
	auditImpl := auditsvc.NewImplService(auditRepo)

	// Middleware applied to /api/me: identity resolution then rate limit.
	identityMW := middleware.RequireIdentity(
		userValid,
		roleValid,
		c,
		time.Duration(cfg.MultiUser.CacheTTLUserSeconds)*time.Second,
		cfg.MultiUser.AdminEmails,
		log,
	)
	rateLimitMW := middleware.RateLimit(limiter, log)

	router := usersvc.NewRestRouter(userRest, identityMW, rateLimitMW)

	return &multiUser{
		userSvc:  userValid,
		roleSvc:  roleValid,
		auditSvc: auditImpl,
		router:   router,
	}
}

// Handlers returns the /api/me route group for mounting alongside the rest
// of the App's handlers.
func (m *multiUser) Handlers() []handler.RouteGroup {
	if m == nil || m.router == nil {
		return nil
	}
	return []handler.RouteGroup{m.router}
}
