package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	apimiddleware "github.com/trushilpatel/jobradar/apps/api/middleware"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/cache"
	auditsvc "github.com/trushilpatel/jobradar/service/audit/services"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
	"gorm.io/gorm"
)

// routeGroupFunc adapts a plain func(chi.Router) to handler.RouteGroup.
type routeGroupFunc func(chi.Router)

func (f routeGroupFunc) Register(r chi.Router) { f(r) }

// wireAdminGate mounts next (the crawler-node admin RestRouter) behind the
// same admin-auth stack every /admin/* route in apps/api already uses: JWT
// verify -> RequireIdentity (JWT-claim roles) -> RequireRole("admin") ->
// AuditAdminMutation. Fixes a real, pre-existing gap: /admin/crawler-nodes/*
// previously had NO admin authentication at all — only the unrelated
// node-token auth that gates /node/* (see
// docs/superpowers/plans/2026-06-13-huma-migration-plan.md, Section 9).
//
// authErr is the error (if any) returned by apimiddleware.Auth's boot-time
// JWKS fetch in main.go. RequireIdentity's VerifyJWT call depends on that
// fetch having populated the package-level JWT pipeline — if it failed
// (e.g. Clerk unreachable), the admin surface is mounted as an
// unconditional 503 rather than left unauthenticated. /node/* (the core
// crawl-assignment pipeline) never depends on Clerk and must stay up
// regardless — this is why main.go does not abort orchestrator's boot on
// an Auth init failure the way apps/api does.
func wireAdminGate(db *gorm.DB, cfg *config.Config, log logger.Logger, authErr error, next handler.RouteGroup) handler.RouteGroup {
	if authErr != nil {
		return routeGroupFunc(func(r chi.Router) {
			r.HandleFunc("/*", func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "admin surface unavailable: auth not initialised", http.StatusServiceUnavailable)
			})
		})
	}

	userRepo := usersvc.NewRepositoryService(db)
	userImpl := usersvc.NewImplService(userRepo, nil) // nil clerk client: only Get/Provision are ever called here
	auditRepo := auditsvc.NewRepositoryService(db)
	auditImpl := auditsvc.NewImplService(auditRepo)

	identityMW := apimiddleware.RequireIdentity(
		userImpl,
		cache.NewMemoryCache(),
		time.Duration(cfg.MultiUser.CacheTTLUserSeconds)*time.Second,
		log,
	)
	roleMW := apimiddleware.RequireRole("admin")
	auditMW := apimiddleware.AuditAdminMutation(auditImpl, log)

	return routeGroupFunc(func(r chi.Router) {
		r.Group(func(sub chi.Router) {
			sub.Use(identityMW)
			sub.Use(roleMW)
			sub.Use(auditMW)
			next.Register(sub)
		})
	})
}
