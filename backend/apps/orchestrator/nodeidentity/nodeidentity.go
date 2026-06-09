// Package nodeidentity carries the authenticated crawler node's identity
// through context.Context. It mirrors the pattern in common/identity but
// is scoped to crawler-node token auth, not JWT user auth.
package nodeidentity

import "context"

// NodeIdentity describes the authenticated crawler node making the current request.
// Built once per request by RequireNodeToken middleware; read downstream via
// FromContext / IDFrom.
type NodeIdentity struct {
	ID   string
	Name string
}

type contextKey struct{}

// NewContext returns a derived context carrying the given NodeIdentity.
func NewContext(ctx context.Context, n NodeIdentity) context.Context {
	return context.WithValue(ctx, contextKey{}, n)
}

// FromContext extracts the NodeIdentity from ctx.
// Returns the zero value and false when no identity is present.
func FromContext(ctx context.Context) (NodeIdentity, bool) {
	n, ok := ctx.Value(contextKey{}).(NodeIdentity)
	return n, ok
}

// IDFrom returns the node ID string from ctx, or "" when absent.
func IDFrom(ctx context.Context) string {
	n, _ := FromContext(ctx)
	return n.ID
}
