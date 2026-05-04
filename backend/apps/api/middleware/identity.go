// Package middleware — identity.go
//
// RequireIdentity is the next-generation auth gate that replaces the email
// allowlist with a DB-backed user + role model. It does not break the existing
// Auth middleware: routes that haven't been migrated yet keep using Auth and
// keep behaving exactly as before. New routes (starting with /api/me) wrap
// themselves in RequireIdentity.
//
// Pipeline per request:
//  1. VerifyJWT (signature + claims) — produces (sub, email).
//  2. Cache lookup at user:{sub} — fast-path identity bundle.
//  3. On miss: load user via UserService.Get, auto-provision on not-found,
//     grant 'admin' role on first sight if email is in ADMIN_EMAILS, then
//     ListForUser to enumerate roles.
//  4. Reject status='suspended' with 403.
//  5. Attach Identity to ctx and continue.
//
// The middleware is unauthenticated-callers-blocking but role-agnostic —
// downstream RequireRole middleware enforces admin scoping.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/cache"
	rolemodel "github.com/trushilpatel/jobradar/service/role/models"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
)

// userService is the narrow consumer interface this middleware needs from the
// user service. Defined here so middleware does not couple to the full
// service.Service contract — interface segregation, and keeps Bucket 3
// independently buildable without Bucket 4.
type userService interface {
	Get(ctx context.Context, id string) (*usermodel.User, error)
	Provision(ctx context.Context, id, email string) (*usermodel.User, error)
}

// roleService is the narrow consumer interface this middleware needs.
type roleService interface {
	ListForUser(ctx context.Context, userID string) ([]string, error)
	Grant(ctx context.Context, userID, roleID string, grantedBy *string) error
}

// RequireIdentity returns a middleware that authenticates the JWT, resolves
// the caller's full Identity (with role lookup + lazy provisioning + admin
// promotion), caches it, attaches it to ctx, and rejects suspended accounts.
//
// adminEmails is the set of emails that should be auto-granted the 'admin'
// role on first observation. Comparison is case-insensitive and trims
// whitespace on both sides. An empty list disables admin promotion entirely.
//
// ttl is the cache TTL for the resolved Identity; 60s in production, smaller
// in tests. A zero ttl disables the cache write but still consults the cache
// on read.
func RequireIdentity(
	usersvc userService,
	rolesvc roleService,
	c cache.Cache,
	ttl time.Duration,
	adminEmails []string,
	log logger.Logger,
) func(http.Handler) http.Handler {
	if usersvc == nil {
		panic("middleware.RequireIdentity: userService must not be nil")
	}
	if rolesvc == nil {
		panic("middleware.RequireIdentity: roleService must not be nil")
	}
	if log == nil {
		log = logger.NewNoop()
	}
	admins := normaliseAdmins(adminEmails)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sub, email, err := VerifyJWT(r)
			if err != nil {
				if errors.Is(err, ErrJWKSUnavailable) {
					log.Error("identity: jwks unavailable", err)
					w.Header().Set("Retry-After", "30")
					handler.RespondError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "authentication service unavailable")
					return
				}
				log.Warn("identity: jwt verification failed", logger.Err(err))
				handler.RespondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token")
				return
			}

			ctx := r.Context()
			ident, err := resolveIdentity(ctx, sub, email, usersvc, rolesvc, c, ttl, admins, log)
			if err != nil {
				if errors.Is(err, errSuspended) {
					handler.RespondError(w, http.StatusForbidden, "FORBIDDEN", "account suspended")
					return
				}
				log.Error("identity: resolve failed", err, logger.Str(logger.FieldUserID, sub))
				handler.RespondError(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
				return
			}

			next.ServeHTTP(w, r.WithContext(identity.NewContext(ctx, ident)))
		})
	}
}

// errSuspended signals that a resolved identity belongs to a suspended user.
// The caller maps this to a 403 with a public message. Internal so callers
// outside this file rely on the HTTP boundary instead.
var errSuspended = errors.New("identity: account suspended")

// resolveIdentity is the cache-aside body of RequireIdentity. Split out for
// testability — tests can drive the resolution path without an HTTP request.
func resolveIdentity(
	ctx context.Context,
	sub, email string,
	usersvc userService,
	rolesvc roleService,
	c cache.Cache,
	ttl time.Duration,
	admins map[string]struct{},
	log logger.Logger,
) (identity.Identity, error) {
	cacheKey := userCacheKey(sub)

	// Cache fast path. A cached suspended user is still rejected — never
	// trust the bundle blindly.
	if c != nil {
		var cached identity.Identity
		hit, err := c.Get(ctx, cacheKey, &cached)
		if err != nil {
			log.Warn("identity: cache get failed", logger.Err(err), logger.Str(logger.FieldUserID, sub))
		}
		if hit {
			if cached.Status == usermodel.StatusSuspended {
				return identity.Identity{}, errSuspended
			}
			return cached, nil
		}
	}

	user, err := usersvc.Get(ctx, sub)
	if err != nil && !isNotFound(err) {
		return identity.Identity{}, err
	}
	if user == nil {
		// First login — provision the user row. Provision is idempotent so
		// concurrent first-login requests converge on one row.
		user, err = usersvc.Provision(ctx, sub, email)
		if err != nil {
			return identity.Identity{}, err
		}
	}

	// Admin promotion. Idempotent: if the role is already granted, the role
	// service is responsible for treating Grant as a no-op.
	if _, isAdminEmail := admins[strings.ToLower(strings.TrimSpace(email))]; isAdminEmail {
		if err := rolesvc.Grant(ctx, sub, rolemodel.RoleAdmin, nil); err != nil {
			// Don't fail the request — the user is signed in and provisioned;
			// admin promotion can be retried on the next request.
			log.Warn("identity: admin grant failed", logger.Err(err), logger.Str(logger.FieldUserID, sub))
		}
	}

	roles, err := rolesvc.ListForUser(ctx, sub)
	if err != nil {
		return identity.Identity{}, err
	}

	// Defence in depth — Provision should grant `user`, but if for any reason
	// it didn't, fix up here. Grant is idempotent.
	if !containsRole(roles, rolemodel.RoleUser) {
		if err := rolesvc.Grant(ctx, sub, rolemodel.RoleUser, nil); err != nil {
			log.Warn("identity: user role backfill failed", logger.Err(err), logger.Str(logger.FieldUserID, sub))
		} else {
			roles = append(roles, rolemodel.RoleUser)
		}
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
			log.Warn("identity: cache set failed", logger.Err(err), logger.Str(logger.FieldUserID, sub))
		}
	}
	return ident, nil
}

// userCacheKey is the canonical cache key for a user's identity bundle.
// Centralised so cache invalidation in services targets the same string.
func userCacheKey(sub string) string { return "user:" + sub }

// normaliseAdmins lowercases + trims emails and stores them in a set so the
// hot path is an O(1) map lookup. Empty entries are dropped.
func normaliseAdmins(emails []string) map[string]struct{} {
	out := make(map[string]struct{}, len(emails))
	for _, e := range emails {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		out[e] = struct{}{}
	}
	return out
}

func containsRole(roles []string, want string) bool {
	for _, r := range roles {
		if r == want {
			return true
		}
	}
	return false
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// isNotFound reports whether err represents a "user not found" condition. The
// user service signals this either by returning a typed domain error or by
// returning (nil, nil); both are tolerated here so the middleware works with
// either convention as Bucket 4 firms up.
//
// Detection is by ServiceError.Code == "NOT_FOUND" rather than pointer
// equality because services build their errors via .WithMessage() which
// returns a fresh struct.
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
