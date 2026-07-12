package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	stderrors "errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	apimiddleware "github.com/trushilpatel/jobradar/apps/api/middleware"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// These tests all call apimiddleware.Auth, which writes to that package's
// process-global JWT pipeline (see apps/api/middleware/auth.go) — never
// mark them t.Parallel(), or concurrent calls race on that shared state
// (the exact class of bug fixed in apihuma.NewZoneConfig during Slice 7).

const (
	testIssuer = "https://example.clerk.accounts.dev"
	testKid    = "test-key-id"
)

func generateRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return key
}

func makeJWKSServer(t *testing.T, key *rsa.PrivateKey, kid string) *httptest.Server {
	t.Helper()
	pub := &key.PublicKey
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	eBig := big.NewInt(int64(pub.E))
	e := base64.RawURLEncoding.EncodeToString(eBig.Bytes())
	doc := map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA", "alg": "RS256", "use": "sig",
			"kid": kid, "n": n, "e": e,
		}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func makeClerkToken(t *testing.T, key *rsa.PrivateKey, kid, sub, email, role string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": sub, "email": email, "role": role,
		"iss": testIssuer, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// initTestAuth spins up a fake JWKS server and calls the real
// apimiddleware.Auth against it, returning a signer for minting tokens that
// verify successfully. Must not run in parallel with other tests that also
// call Auth (see the package-doc note above).
func initTestAuth(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key := generateRSAKey(t)
	jwks := makeJWKSServer(t, key, testKid)
	authCfg := config.AuthConfig{ClerkJWKSURL: jwks.URL, ClerkIssuerURL: testIssuer}
	if _, err := apimiddleware.Auth(authCfg, "", logger.NewNoop()); err != nil {
		t.Fatalf("Auth init: %v", err)
	}
	return key
}

// newAdminGateTestDB returns an in-memory sqlite DB with the users and
// admin_audit_log tables wireAdminGate's wired services touch. Mirrors
// service/user/tests and service/audit/tests' own sqlite schema helpers —
// production uses Postgres-specific features (citext, gen_random_uuid())
// with no sqlite equivalent, but the columns these two repos actually
// read/write work identically here.
func newAdminGateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	stmts := []string{
		`CREATE TABLE users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			display_name TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE admin_audit_log (
			id            TEXT PRIMARY KEY,
			actor_user_id TEXT,
			action        TEXT NOT NULL,
			target_type   TEXT,
			target_id     TEXT,
			payload       BLOB,
			created_at    DATETIME NOT NULL
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return db
}

// alwaysCalled is a handler.RouteGroup double that records whether it was
// reached, so tests can assert the gate did (or did not) let a request
// through to the inner router.
type recordingRouteGroup struct {
	called bool
}

func (r *recordingRouteGroup) Register(router chi.Router) {
	router.HandleFunc("/*", func(w http.ResponseWriter, _ *http.Request) {
		r.called = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestWireAdminGate_AuthInitFailed_Returns503WithoutTouchingNext(t *testing.T) {
	inner := &recordingRouteGroup{}
	gate := wireAdminGate(nil, &config.Config{}, logger.NewNoop(), errJWKSUnreachable, inner)

	r := chi.NewRouter()
	r.Route("/admin/crawler-nodes", gate.Register)

	req := httptest.NewRequest(http.MethodGet, "/admin/crawler-nodes/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	if inner.called {
		t.Error("inner router must not be reached when auth failed to initialise")
	}
}

var errJWKSUnreachable = stderrors.New("jwks unreachable")

func TestWireAdminGate_NoAuthorizationHeader_Returns401(t *testing.T) {
	initTestAuth(t)
	db := newAdminGateTestDB(t)
	inner := &recordingRouteGroup{}
	gate := wireAdminGate(db, &config.Config{}, logger.NewNoop(), nil, inner)

	r := chi.NewRouter()
	r.Route("/admin/crawler-nodes", gate.Register)

	req := httptest.NewRequest(http.MethodGet, "/admin/crawler-nodes/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
	if inner.called {
		t.Error("inner router must not be reached without a valid token")
	}
}

func TestWireAdminGate_NonAdminRole_Returns403(t *testing.T) {
	key := initTestAuth(t)
	db := newAdminGateTestDB(t)
	inner := &recordingRouteGroup{}
	gate := wireAdminGate(db, &config.Config{}, logger.NewNoop(), nil, inner)

	r := chi.NewRouter()
	r.Route("/admin/crawler-nodes", gate.Register)

	token := makeClerkToken(t, key, testKid, "user-1", "user1@example.com", "user")
	req := httptest.NewRequest(http.MethodGet, "/admin/crawler-nodes/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
	if inner.called {
		t.Error("inner router must not be reached for a non-admin role")
	}
}

func TestWireAdminGate_AdminRole_ReachesInnerHandlerAndAudits(t *testing.T) {
	key := initTestAuth(t)
	db := newAdminGateTestDB(t)
	inner := &recordingRouteGroup{}
	gate := wireAdminGate(db, &config.Config{}, logger.NewNoop(), nil, inner)

	r := chi.NewRouter()
	r.Route("/admin/crawler-nodes", gate.Register)

	token := makeClerkToken(t, key, testKid, "admin-1", "admin@example.com", "admin")
	req := httptest.NewRequest(http.MethodPost, "/admin/crawler-nodes/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !inner.called {
		t.Fatal("inner router must be reached for an admin-role token")
	}

	var auditCount int64
	if err := db.Table("admin_audit_log").Count(&auditCount).Error; err != nil {
		t.Fatalf("count admin_audit_log: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("admin_audit_log rows = %d, want 1 (mutating POST must be audited)", auditCount)
	}
}
