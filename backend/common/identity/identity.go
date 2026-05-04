// Package identity carries the authenticated caller's identity through
// context.Context. It is the single source of truth for "who is making this
// request" across handlers, middleware, services, and tests.
//
// The contract is intentionally small: middleware builds an Identity from the
// JWT + DB lookup and attaches it via NewContext. Downstream code reads it via
// FromContext / UserIDFrom / IsAdmin / HasRole. Tests use WithTestIdentity to
// inject a fake caller without spinning up middleware.
package identity

import "context"

// Identity describes the authenticated caller making the current request.
// It is constructed once per request by the auth middleware and read everywhere
// else via context helpers — never mutate after construction.
type Identity struct {
	UserID      string
	Email       string
	Roles       []string
	Status      string // "active" | "suspended"
	DisplayName string
}

// HasRole reports whether the identity carries the given role.
// Role comparison is case-sensitive — middleware is responsible for
// normalising role names before constructing the Identity.
func (i Identity) HasRole(role string) bool {
	for _, r := range i.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsAdmin is shorthand for HasRole("admin"). Use it everywhere admin gating
// is checked so the constant lives in one place.
func (i Identity) IsAdmin() bool { return i.HasRole(roleAdmin) }

// roleAdmin is duplicated from service/role to avoid an import cycle —
// the role service does not yet exist when this package is compiled.
const roleAdmin = "admin"

// ctxKey is unexported so no other package can collide on the same key.
type ctxKey struct{}

// NewContext returns a copy of ctx with the given Identity attached.
// Used by the auth middleware once per request.
func NewContext(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the Identity attached to ctx, if any. The boolean is
// false when no identity has been attached — callers that require an identity
// should use MustFromContext.
func FromContext(ctx context.Context) (Identity, bool) {
	if ctx == nil {
		return Identity{}, false
	}
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// MustFromContext returns the Identity attached to ctx, panicking if none is
// present. Use only inside HTTP handlers wired behind the auth middleware,
// where the absence of an identity is a programmer error, not a user error.
func MustFromContext(ctx context.Context) Identity {
	id, ok := FromContext(ctx)
	if !ok {
		panic("identity: no Identity in context — handler not wired behind auth middleware")
	}
	return id
}

// UserIDFrom returns the caller's user ID from ctx, or the empty string if no
// identity is attached. Safe to call from any code path; pair with an explicit
// check when the empty string is a real possibility (e.g. unauthenticated
// rate-limit fallback).
func UserIDFrom(ctx context.Context) string {
	id, ok := FromContext(ctx)
	if !ok {
		return ""
	}
	return id.UserID
}

// IsAdmin reports whether the ctx-bound identity has the "admin" role.
// Returns false when no identity is attached — never panics.
func IsAdmin(ctx context.Context) bool {
	id, ok := FromContext(ctx)
	if !ok {
		return false
	}
	return id.IsAdmin()
}

// HasRole reports whether the ctx-bound identity has the given role.
// Returns false when no identity is attached — never panics.
func HasRole(ctx context.Context, role string) bool {
	id, ok := FromContext(ctx)
	if !ok {
		return false
	}
	return id.HasRole(role)
}

// WithTestIdentity returns a context carrying a synthetic Identity with the
// given userID and roles. Tests should prefer this over building an Identity
// by hand so the construction stays in one place if the struct changes.
func WithTestIdentity(ctx context.Context, userID string, roles ...string) context.Context {
	return NewContext(ctx, Identity{
		UserID: userID,
		Roles:  append([]string(nil), roles...),
		Status: "active",
	})
}
