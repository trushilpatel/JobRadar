// Package middleware provides HTTP middleware for the orchestrator server.
package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/orchestrator/nodeidentity"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	crawlernodecommon "github.com/trushilpatel/jobradar/service/crawlernode/common"
	crawlernodemodel "github.com/trushilpatel/jobradar/service/crawlernode/models"
)

// NodeAuthStore is the narrow store interface required by RequireNodeToken.
// Satisfied by the full NodeStore from the crawlernode service slice.
type NodeAuthStore interface {
	GetByTokenPrefix(ctx context.Context, prefix string) ([]*crawlernodemodel.CrawlerNode, error)
	UpdateLastSeen(ctx context.Context, nodeID string) error
}

// RequireNodeToken returns middleware that authenticates crawler nodes via
// HMAC-SHA256 Bearer tokens. On success it injects the NodeIdentity into the
// request context and fires an async last_seen_at update. On failure it writes
// 401 (invalid/missing) or 403 (inactive node) and halts the chain.
func RequireNodeToken(store NodeAuthStore, tokenSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := extractBearer(r)
			if !ok || len(token) < crawlernodecommon.TokenPrefixLen {
				handler.HandleError(w, domainerrs.ErrNodeUnauthorized)
				return
			}

			prefix := token[:crawlernodecommon.TokenPrefixLen]
			nodes, err := store.GetByTokenPrefix(r.Context(), prefix)
			if err != nil || len(nodes) == 0 {
				handler.HandleError(w, domainerrs.ErrNodeUnauthorized)
				return
			}

			node := matchNode(nodes, token, tokenSecret)
			if node == nil {
				handler.HandleError(w, domainerrs.ErrNodeUnauthorized)
				return
			}

			if node.Status != "active" {
				handler.HandleError(w, domainerrs.ErrNodeDisabled)
				return
			}

			// Fire-and-forget: update last_seen_at — not on the critical path.
			go func() { _ = store.UpdateLastSeen(context.Background(), node.ID) }()

			ctx := nodeidentity.NewContext(r.Context(), nodeidentity.NodeIdentity{
				ID:   node.ID,
				Name: node.Name,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractBearer pulls the Bearer token from the Authorization header.
// Returns ("", false) when the header is absent or uses a non-Bearer scheme.
func extractBearer(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", false
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], true
}

// matchNode finds the first node in candidates whose stored token_hash matches
// the HMAC of the provided token. Uses constant-time comparison to prevent
// timing attacks. Returns nil when no candidate matches.
func matchNode(candidates []*crawlernodemodel.CrawlerNode, token, secret string) *crawlernodemodel.CrawlerNode {
	expected := computeHMAC(token, secret)
	for _, n := range candidates {
		if n.TokenHash == nil {
			continue
		}
		stored, err := hex.DecodeString(*n.TokenHash)
		if err != nil {
			continue
		}
		if hmac.Equal(expected, stored) {
			return n
		}
	}
	return nil
}

func computeHMAC(token, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token))
	return mac.Sum(nil)
}
