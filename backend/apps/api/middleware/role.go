// Package middleware — role.go
//
// RequireRole gates a handler chain on a single role membership check. It
// trusts that RequireIdentity ran upstream and surfaced the caller's roles on
// the request context — RequireRole never reaches into the database or cache
// itself.
//
// Failure mode is uniform: 403 with a generic JSON body. The middleware does
// not leak which role was missing because the same middleware guards every
// admin route — leaking the requirement would leak the route's purpose.
package middleware

import (
	"net/http"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/common/identity"
)

// RequireRole returns middleware that allows a request through only when the
// ctx-bound Identity carries `role`. Missing identity is treated identically
// to a missing role — both surface as 403 to avoid leaking whether the caller
// was authenticated at all.
//
// Empty role is a programmer error and panics at construction; routes should
// not be silently un-gated by typo.
func RequireRole(role string) func(http.Handler) http.Handler {
	if role == "" {
		panic("middleware.RequireRole: role must not be empty")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !identity.HasRole(r.Context(), role) {
				handler.RespondError(w, http.StatusForbidden, "FORBIDDEN", "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
