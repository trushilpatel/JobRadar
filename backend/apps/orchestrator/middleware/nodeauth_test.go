package middleware_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trushilpatel/jobradar/apps/orchestrator/middleware"
	"github.com/trushilpatel/jobradar/apps/orchestrator/nodeidentity"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	crawlernodemodel "github.com/trushilpatel/jobradar/service/crawlernode/models"
)

const testSecret = "test-secret-key"

func computeTestHMAC(token, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

// mockNodeAuthStore implements middleware.NodeAuthStore.
type mockNodeAuthStore struct {
	getByPrefixFn    func(ctx context.Context, prefix string) ([]*crawlernodemodel.CrawlerNode, error)
	updateLastSeenFn func(ctx context.Context, nodeID string) error
}

func (m *mockNodeAuthStore) GetByTokenPrefix(ctx context.Context, prefix string) ([]*crawlernodemodel.CrawlerNode, error) {
	return m.getByPrefixFn(ctx, prefix)
}
func (m *mockNodeAuthStore) UpdateLastSeen(ctx context.Context, nodeID string) error {
	if m.updateLastSeenFn != nil {
		return m.updateLastSeenFn(ctx, nodeID)
	}
	return nil
}

func activeNode(id, name, token, secret string) *crawlernodemodel.CrawlerNode {
	hash := computeTestHMAC(token, secret)
	return &crawlernodemodel.CrawlerNode{
		ID:          id,
		Name:        name,
		TokenPrefix: token[:8],
		TokenHash:   &hash,
		Status:      "active",
	}
}

func makeHandler(next http.Handler, store middleware.NodeAuthStore) http.Handler {
	return middleware.RequireNodeToken(store, testSecret)(next)
}

func okHandler(_ *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// --- Missing / malformed Authorization ---

func TestRequireNodeToken_MissingAuthHeader_Returns401(t *testing.T) {
	store := &mockNodeAuthStore{
		getByPrefixFn: func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) {
			return nil, nil
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/work", nil)
	makeHandler(okHandler(t), store).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("missing auth header: status = %d, want 401", rr.Code)
	}
}

func TestRequireNodeToken_NonBearerScheme_Returns401(t *testing.T) {
	store := &mockNodeAuthStore{
		getByPrefixFn: func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) {
			return nil, nil
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/work", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	makeHandler(okHandler(t), store).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("non-Bearer scheme: status = %d, want 401", rr.Code)
	}
}

func TestRequireNodeToken_TooShortToken_Returns401(t *testing.T) {
	store := &mockNodeAuthStore{
		getByPrefixFn: func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) {
			return nil, nil
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/work", nil)
	req.Header.Set("Authorization", "Bearer short")
	makeHandler(okHandler(t), store).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("short token: status = %d, want 401", rr.Code)
	}
}

// --- Store lookup failures ---

func TestRequireNodeToken_StoreError_Returns401(t *testing.T) {
	store := &mockNodeAuthStore{
		getByPrefixFn: func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) {
			return nil, domainerrs.ErrInternal.WithMessage("db down")
		},
	}
	token := "abcdef1234567890abcdef1234567890" // 32 chars
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/work", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	makeHandler(okHandler(t), store).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("store error: status = %d, want 401", rr.Code)
	}
}

func TestRequireNodeToken_NoMatchingNode_Returns401(t *testing.T) {
	store := &mockNodeAuthStore{
		getByPrefixFn: func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) {
			return nil, nil
		},
	}
	token := "abcdef1234567890abcdef1234567890"
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/work", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	makeHandler(okHandler(t), store).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("no match: status = %d, want 401", rr.Code)
	}
}

// --- HMAC mismatch ---

func TestRequireNodeToken_HMACMismatch_Returns401(t *testing.T) {
	token := "abcdef1234567890abcdef1234567890"
	badHash := computeTestHMAC(token, "wrong-secret")
	node := &crawlernodemodel.CrawlerNode{
		ID: "node-1", Name: "n1", TokenPrefix: token[:8],
		TokenHash: &badHash, Status: "active",
	}
	store := &mockNodeAuthStore{
		getByPrefixFn: func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) {
			return []*crawlernodemodel.CrawlerNode{node}, nil
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/work", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	makeHandler(okHandler(t), store).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HMAC mismatch: status = %d, want 401", rr.Code)
	}
}

// --- Inactive node ---

func TestRequireNodeToken_DisabledNode_Returns403(t *testing.T) {
	token := "abcdef1234567890abcdef1234567890"
	hash := computeTestHMAC(token, testSecret)
	node := &crawlernodemodel.CrawlerNode{
		ID: "node-1", Name: "n1", TokenPrefix: token[:8],
		TokenHash: &hash, Status: "disabled",
	}
	store := &mockNodeAuthStore{
		getByPrefixFn: func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) {
			return []*crawlernodemodel.CrawlerNode{node}, nil
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/work", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	makeHandler(okHandler(t), store).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("disabled node: status = %d, want 403", rr.Code)
	}
}

func TestRequireNodeToken_RetiredNode_Returns403(t *testing.T) {
	token := "abcdef1234567890abcdef1234567890"
	hash := computeTestHMAC(token, testSecret)
	node := &crawlernodemodel.CrawlerNode{
		ID: "node-1", Name: "n1", TokenPrefix: token[:8],
		TokenHash: &hash, Status: "retired",
	}
	store := &mockNodeAuthStore{
		getByPrefixFn: func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) {
			return []*crawlernodemodel.CrawlerNode{node}, nil
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/work", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	makeHandler(okHandler(t), store).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("retired node: status = %d, want 403", rr.Code)
	}
}

// --- Happy path ---

func TestRequireNodeToken_ValidToken_InjectsIdentityAndCalls200(t *testing.T) {
	token := "abcdef1234567890abcdef1234567890"
	node := activeNode("node-1", "prod-node", token, testSecret)
	store := &mockNodeAuthStore{
		getByPrefixFn: func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) {
			return []*crawlernodemodel.CrawlerNode{node}, nil
		},
	}

	var capturedID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = nodeidentity.IDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/work", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	makeHandler(inner, store).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("valid token: status = %d, want 200", rr.Code)
	}
	if capturedID != "node-1" {
		t.Errorf("nodeidentity ID = %q, want node-1", capturedID)
	}
}

// --- Malformed hex in token_hash ---

func TestRequireNodeToken_MalformedTokenHash_Returns401(t *testing.T) {
	token := "abcdef1234567890abcdef1234567890"
	badHash := "not-valid-hex!!"
	node := &crawlernodemodel.CrawlerNode{
		ID: "node-1", Name: "n1", TokenPrefix: token[:8],
		TokenHash: &badHash, Status: "active",
	}
	store := &mockNodeAuthStore{
		getByPrefixFn: func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) {
			return []*crawlernodemodel.CrawlerNode{node}, nil
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/work", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	makeHandler(okHandler(t), store).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("malformed token_hash: status = %d, want 401", rr.Code)
	}
}

// --- NilTokenHash handled ---

func TestRequireNodeToken_NilTokenHash_Returns401(t *testing.T) {
	token := "abcdef1234567890abcdef1234567890"
	node := &crawlernodemodel.CrawlerNode{
		ID: "node-1", Name: "n1", TokenPrefix: token[:8],
		TokenHash: nil, Status: "active",
	}
	store := &mockNodeAuthStore{
		getByPrefixFn: func(_ context.Context, _ string) ([]*crawlernodemodel.CrawlerNode, error) {
			return []*crawlernodemodel.CrawlerNode{node}, nil
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/node/work", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	makeHandler(okHandler(t), store).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("nil token_hash (retired?): status = %d, want 401", rr.Code)
	}
}
