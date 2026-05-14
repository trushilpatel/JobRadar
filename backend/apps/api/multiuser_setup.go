package main

import (
	"net/http"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	auditsvc "github.com/trushilpatel/jobradar/service/audit/services"
	rolesvc "github.com/trushilpatel/jobradar/service/role/services"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
)

// multiUser bundles the Phase 2 multi-user services, the /me route group
// (mounted under /api by NewRouter), and the /admin/* route group with all
// admin service handlers. Construction lives in wire_multiuser.go.
type multiUser struct {
	userSvc     usersvc.Service
	roleSvc     rolesvc.Service
	auditSvc    auditsvc.Service
	identityMW  func(http.Handler) http.Handler
	rateLimitMW func(http.Handler) http.Handler
	router      handler.RouteGroup // /me routes — mounted under /api by NewRouter
	admin       handler.RouteGroup // /admin/* routes — mounted at root by NewRouter
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
