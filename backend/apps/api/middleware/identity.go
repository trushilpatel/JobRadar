// Package middleware — identity.go
//
// RequireIdentity is the auth gate that backs every /api/* route with a
// DB-driven user model and JWT-carried role.
//
// Pipeline per request:
//  1. VerifyJWT (signature + claims) — produces VerifiedClaims{Subject, Email, Role}.
//  2. Cache lookup at user:{sub} — fast-path identity bundle.
//  3. On miss: load user via UserService.Get, auto-provision on not-found.
//  4. Reject status='suspended' with 403.
//  5. Build identity.Roles from VerifiedClaims.Role (no DB role lookup).
//  6. Attach Identity to ctx and continue.
//
// Roles are sourced exclusively from the Clerk JWT claim "role" (injected via
// Clerk publicMetadata). DB role tables (roles/user_roles) were removed in
// migration 081. There is no DB role lookup — RequireRole("admin") gates on
// the claim directly.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/cache"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
)

// userService is the narrow consumer interface this middleware needs from the
// user service. Defined here so middleware does not couple to the full
// service.Service contract.
type userService interface {
	Get(ctx context.Context, id string) (*usermodel.User, error)
	Provision(ctx context.Context, id, email string) (*usermodel.User, error)
}

// RequireIdentity returns a middleware that authenticates the JWT, resolves
// the caller's full Identity (with lazy provisioning), caches it, attaches it
// to ctx, and rejects suspended accounts.
//
// Roles are taken from the JWT claim — no DB lookup is performed. A "role"
// claim value of "admin" produces identity.Roles = ["admin"]; an empty or
// absent "role" claim produces identity.Roles = [].
//
// ttl is the cache TTL for the resolved Identity; 60s in production, smaller
// in tests. A zero ttl disables the cache write but still consults the cache
// on read.
func RequireIdentity(
	usersvc userService,
	c cache.Cache,
	ttl time.Duration,
	log logger.Logger,
) func(http.Handler) http.Handler {
	if usersvc == nil {
		panic("middleware.RequireIdentity: userService must not be nil")
	}
	if log == nil {
		log = logger.NewNoop()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			vc, err := VerifyJWT(r)
			if err != nil {
				log.Warn("identity: jwt verification failed", logger.Err(err))
				handler.RespondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token")
				return
			}

			ctx := r.Context()
			ident, err := resolveIdentity(ctx, vc, usersvc, c, ttl, log)
			if err != nil {
				if errors.Is(err, errSuspended) {
					handler.RespondError(w, http.StatusForbidden, "FORBIDDEN", "account suspended")
					return
				}
				log.Error("identity: resolve failed", err, logger.Str(logger.FieldUserID, vc.Subject))
				handler.RespondError(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
				return
			}

			next.ServeHTTP(w, r.WithContext(identity.NewContext(ctx, ident)))
		})
	}
}

// errSuspended signals that a resolved identity belongs to a suspended user.
var errSuspended = errors.New("identity: account suspended")

// devBypassSub is the synthetic subject returned by the dev-token bypass in
// Auth middleware. RequireIdentity short-circuits here to avoid a DB lookup
// that would always fail with "invalid user id format".
const devBypassSub = "dev-bypass"

// resolveIdentity is the cache-aside body of RequireIdentity. Split out for
// testability — tests can drive the resolution path without an HTTP request.
func resolveIdentity(
	ctx context.Context,
	vc VerifiedClaims,
	usersvc userService,
	c cache.Cache,
	ttl time.Duration,
	log logger.Logger,
) (identity.Identity, error) {
	// Dev-token bypass — Auth middleware already verified the secret; return a
	// synthetic identity without touching the DB (sub is not a real user ID).
	if vc.Subject == devBypassSub {
		return identity.Identity{
			UserID: devBypassSub,
			Email:  "",
			Roles:  []string{"user", "admin"},
		}, nil
	}

	cacheKey := userCacheKey(vc.Subject)

	// Cache fast path. A cached suspended user is still rejected — never
	// trust the bundle blindly.
	if c != nil {
		var cached identity.Identity
		hit, err := c.Get(ctx, cacheKey, &cached)
		if err != nil {
			log.Warn("identity: cache get failed", logger.Err(err), logger.Str(logger.FieldUserID, vc.Subject))
		}
		if hit {
			if cached.Status == usermodel.StatusSuspended {
				return identity.Identity{}, errSuspended
			}
			return cached, nil
		}
	}

	user, err := usersvc.Get(ctx, vc.Subject)
	if err != nil && !isNotFound(err) {
		return identity.Identity{}, err
	}
	if user == nil {
		// First login — provision the user row. Provision is idempotent so
		// concurrent first-login requests converge on one row.
		user, err = usersvc.Provision(ctx, vc.Subject, vc.Email)
		if err != nil {
			return identity.Identity{}, err
		}
	}

	// Roles come exclusively from the JWT claim — no DB lookup needed.
	// Initialise as empty slice (not nil) so JSON serialises to [] not null.
	roles := []string{}
	if vc.Role != "" {
		roles = []string{vc.Role}
	}

	ident := identity.Identity{
		UserID:      user.ID,
		Email:       user.Email,
		Roles:       roles,
		Status:      user.Status,
		DisplayName: derefString(user.DisplayName),
	}

	if ident.Status == usermodel.StatusSuspended {
		// Still cache so subsequent rejections don't hit the DB again.
		if c != nil && ttl > 0 {
			_ = c.Set(ctx, cacheKey, ident, ttl)
		}
		return identity.Identity{}, errSuspended
	}

	if c != nil && ttl > 0 {
		if err := c.Set(ctx, cacheKey, ident, ttl); err != nil {
			log.Warn("identity: cache set failed", logger.Err(err), logger.Str(logger.FieldUserID, vc.Subject))
		}
	}
	return ident, nil
}

// userCacheKey is the canonical cache key for a user's identity bundle.
func userCacheKey(sub string) string { return "user:" + sub }

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// isNotFound reports whether err represents a "user not found" condition.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var sErr *domainerrs.ServiceError
	if errors.As(err, &sErr) && sErr.Code == domainerrs.ErrNotFound.Code {
		return true
	}
	return false
}
