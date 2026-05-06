// Package middleware — audit.go
//
// AuditAdminMutation appends an admin_audit_log entry after every successful
// admin mutation. It is meant to wrap admin-only routers (the post-Phase-2
// /admin/* tree); in Phase 1 it ships unwired so Bucket 4 can mount it once
// the admin services exist.
//
// Recording rules:
//   - Only mutating methods are recorded — POST / PATCH / PUT / DELETE.
//   - Only 2xx responses are recorded — failed mutations are not interesting
//     enough to retain in the audit log, and they double-up with error logs.
//   - Audit append failures NEVER affect the response. The handler has
//     already finished by the time we try to write; we log and move on.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/common/logger"
	auditmodel "github.com/trushilpatel/jobradar/service/audit/models"
)

// auditService is the narrow consumer interface this middleware needs from
// the audit service. Defined locally so middleware does not couple to the
// full service.Service contract — and so Bucket 3 builds independently of
// Bucket 4 where the audit service slice lives.
type auditService interface {
	Append(ctx context.Context, entry auditmodel.Entry) error
}

// AuditAdminMutation returns middleware that logs successful admin
// mutations. svc may be nil — in that case the middleware is a no-op
// pass-through, useful for tests that don't want an audit dependency.
func AuditAdminMutation(svc auditService, log logger.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = logger.NewNoop()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if svc == nil || !isMutatingMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)

			if rw.status < 200 || rw.status >= 300 {
				return
			}

			actor := identity.UserIDFrom(r.Context())
			var actorPtr *string
			if actor != "" {
				actor := actor // capture
				actorPtr = &actor
			}
			targetType := inferTargetType(r.URL.Path)
			var targetID *string
			if id := chi.URLParamFromCtx(r.Context(), "id"); id != "" {
				targetID = &id
			}
			entry := auditmodel.Entry{
				ActorUserID: actorPtr,
				Action:      r.Method + " " + r.URL.Path,
				TargetType:  targetType,
				TargetID:    targetID,
			}
			if err := svc.Append(r.Context(), entry); err != nil {
				log.Warn("audit: append failed",
					logger.Err(err),
					logger.Str(logger.FieldMethod, r.Method),
					logger.Str(logger.FieldPath, r.URL.Path),
					logger.Str(logger.FieldUserID, actor),
				)
			}
		})
	}
}

// isMutatingMethod reports whether the HTTP method changes server state.
// HEAD / GET / OPTIONS are read-only; everything else is treated as a write.
func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// inferTargetType extracts a human-readable entity type from an admin route
// path. Admin paths follow the pattern /admin/{entity}/{id}/... so we split
// and return the entity segment as the target type.
//
// Recognised entities: "companies" → "company", "users" → "user".
// Returns nil for any path that does not match a known entity.
func inferTargetType(path string) *string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// Expect at least /admin/{entity}
	if len(parts) < 2 || parts[0] != "admin" {
		return nil
	}
	var t string
	switch parts[1] {
	case "companies":
		t = "company"
	case "users":
		t = "user"
	default:
		return nil
	}
	return &t
}
