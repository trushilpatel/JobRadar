package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trushilpatel/jobradar/common/config"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/cache"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
)

// ── fake implementations ──────────────────────────────────────────────────────

// fakeUserService is a hand-rolled in-memory userService for tests.
type fakeUserService struct {
	mu          sync.Mutex
	users       map[string]*usermodel.User
	provisioned map[string]int
}

func newFakeUserService() *fakeUserService {
	return &fakeUserService{users: map[string]*usermodel.User{}, provisioned: map[string]int{}}
}

func (f *fakeUserService) seed(id, email, status string, displayName *string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[id] = &usermodel.User{ID: id, Email: email, Status: status, DisplayName: displayName}
}

func (f *fakeUserService) Get(_ context.Context, id string) (*usermodel.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return nil, domainerrs.ErrNotFound.WithMessage("user not found")
	}
	return u, nil
}

func (f *fakeUserService) Provision(_ context.Context, id, email string) (*usermodel.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.provisioned[id]++
	if u, ok := f.users[id]; ok {
		return u, nil
	}
	u := &usermodel.User{ID: id, Email: email, Status: usermodel.StatusActive}
	f.users[id] = u
	return u, nil
}

// ── JWKS test infrastructure ──────────────────────────────────────────────────

// newTestRSAKey generates a 2048-bit RSA key for test use.
func newTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

// startTestJWKSServer starts a JWKS server for the given RSA key.
func startTestJWKSServer(t *testing.T, key *rsa.PrivateKey, kid string) *httptest.Server {
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

// initIdentityTestPipeline boots the JWT pipeline for the given RSA key + JWKS server.
func initIdentityTestPipeline(t *testing.T, key *rsa.PrivateKey, jwksURL, devToken string) {
	t.Helper()
	cfg := config.AuthConfig{
		ClerkJWKSURL:   jwksURL,
		ClerkIssuerURL: testIssuer,
	}
	_, err := Auth(cfg, devToken, logger.NewNoop())
	require.NoError(t, err)
}

// signedIdentityRequest builds a real RS256 JWT carrying the given claims.
func signedIdentityRequest(t *testing.T, key *rsa.PrivateKey, kid, subject, email, role string) *http.Request {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": subject, "email": email, "role": role,
		"iss": testIssuer, "iat": time.Now().Unix(),
		"exp": time.Now().Add(5 * time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	require.NoError(t, err)

	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	return r
}

func requestWithDevToken(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	r.Header.Set("X-Dev-Token", token)
	return r
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestRequireIdentity_FirstLogin_ProvisionsUser(t *testing.T) {
	key := newTestRSAKey(t)
	srv := startTestJWKSServer(t, key, testKid)
	initIdentityTestPipeline(t, key, srv.URL, "")

	usr := newFakeUserService()
	mw := RequireIdentity(usr, cache.NewMemoryCache(), 60*time.Second, logger.NewNoop())
	var got identity.Identity
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = identity.MustFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedIdentityRequest(t, key, testKid, "user-new", "new@example.com", "user"))
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, 1, usr.provisioned["user-new"])
	assert.Equal(t, "user-new", got.UserID)
	assert.Equal(t, "new@example.com", got.Email)
	assert.Equal(t, usermodel.StatusActive, got.Status)
}

func TestRequireIdentity_RoleFromJWTClaim(t *testing.T) {
	key := newTestRSAKey(t)
	srv := startTestJWKSServer(t, key, testKid)
	initIdentityTestPipeline(t, key, srv.URL, "")

	usr := newFakeUserService()
	usr.seed("u-admin", "admin@example.com", usermodel.StatusActive, nil)

	mw := RequireIdentity(usr, cache.NewMemoryCache(), 60*time.Second, logger.NewNoop())
	var got identity.Identity
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = identity.MustFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedIdentityRequest(t, key, testKid, "u-admin", "admin@example.com", "admin"))
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, []string{"admin"}, got.Roles)
}

func TestRequireIdentity_EmptyRoleClaimProducesEmptyRoles(t *testing.T) {
	key := newTestRSAKey(t)
	srv := startTestJWKSServer(t, key, testKid)
	initIdentityTestPipeline(t, key, srv.URL, "")

	usr := newFakeUserService()
	usr.seed("u-norole", "norole@example.com", usermodel.StatusActive, nil)

	mw := RequireIdentity(usr, cache.NewMemoryCache(), 60*time.Second, logger.NewNoop())
	var got identity.Identity
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = identity.MustFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	// Empty role claim.
	h.ServeHTTP(rr, signedIdentityRequest(t, key, testKid, "u-norole", "norole@example.com", ""))
	assert.Equal(t, http.StatusOK, rr.Code)
	// Empty claim → no roles. Contract: empty claim → no roles → caller uses HasRole().
	assert.False(t, got.HasRole("admin"))
	assert.False(t, got.HasRole("user"))
}

func TestRequireIdentity_SuspendedUser_403(t *testing.T) {
	key := newTestRSAKey(t)
	srv := startTestJWKSServer(t, key, testKid)
	initIdentityTestPipeline(t, key, srv.URL, "")

	usr := newFakeUserService()
	usr.seed("u-bad", "bad@example.com", usermodel.StatusSuspended, nil)

	mw := RequireIdentity(usr, cache.NewMemoryCache(), 60*time.Second, logger.NewNoop())
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedIdentityRequest(t, key, testKid, "u-bad", "bad@example.com", "user"))
	assert.False(t, called, "next must not be called for suspended user")
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestRequireIdentity_CacheHit_AvoidsDBCall(t *testing.T) {
	key := newTestRSAKey(t)
	srv := startTestJWKSServer(t, key, testKid)
	initIdentityTestPipeline(t, key, srv.URL, "")

	usr := newFakeUserService()
	usr.seed("u-cached", "c@example.com", usermodel.StatusActive, nil)

	c := cache.NewMemoryCache()
	mw := RequireIdentity(usr, c, 60*time.Second, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First call populates cache.
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, signedIdentityRequest(t, key, testKid, "u-cached", "c@example.com", "user"))
	assert.Equal(t, http.StatusOK, rr1.Code)

	// Wipe the user from the service. If cache is consulted, second call still works.
	preProv := usr.provisioned["u-cached"]
	usr.mu.Lock()
	delete(usr.users, "u-cached")
	usr.mu.Unlock()

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, signedIdentityRequest(t, key, testKid, "u-cached", "c@example.com", "user"))
	assert.Equal(t, http.StatusOK, rr2.Code)
	assert.Equal(t, preProv, usr.provisioned["u-cached"], "Provision must not be called after cache was populated")
}

func TestRequireIdentity_BadJWT_401(t *testing.T) {
	key := newTestRSAKey(t)
	srv := startTestJWKSServer(t, key, testKid)
	initIdentityTestPipeline(t, key, srv.URL, "")

	usr := newFakeUserService()
	mw := RequireIdentity(usr, cache.NewMemoryCache(), 60*time.Second, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/me", nil) // no Authorization
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestRequireIdentity_DevTokenBypassWorks(t *testing.T) {
	const devToken = "test-dev-mid-token"
	key := newTestRSAKey(t)
	srv := startTestJWKSServer(t, key, testKid)
	initIdentityTestPipeline(t, key, srv.URL, devToken)

	usr := newFakeUserService()
	mw := RequireIdentity(usr, cache.NewMemoryCache(), 60*time.Second, logger.NewNoop())
	var got identity.Identity
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = identity.MustFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, requestWithDevToken(devToken))
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "dev-bypass", got.UserID)
}

// TestRequireIdentity_NilDeps verifies the constructor panics when userService is nil.
func TestRequireIdentity_NilDeps(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil userService")
		}
	}()
	RequireIdentity(nil, cache.NewMemoryCache(), time.Second, logger.NewNoop())
}

// erroringUserService surfaces a non-NotFound error from Get.
type erroringUserService struct {
	err error
}

func (e *erroringUserService) Get(_ context.Context, _ string) (*usermodel.User, error) {
	return nil, e.err
}

func (e *erroringUserService) Provision(_ context.Context, id, email string) (*usermodel.User, error) {
	return &usermodel.User{ID: id, Email: email, Status: usermodel.StatusActive}, nil
}

func TestRequireIdentity_GetError_500(t *testing.T) {
	key := newTestRSAKey(t)
	srv := startTestJWKSServer(t, key, testKid)
	initIdentityTestPipeline(t, key, srv.URL, "")

	usr := &erroringUserService{err: errors.New("db boom")}
	mw := RequireIdentity(usr, cache.NewMemoryCache(), time.Second, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedIdentityRequest(t, key, testKid, "u-err", "e@example.com", "user"))
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// errorCache is a cache.Cache that returns a configurable error from Get.
type errorCache struct{ getErr error }

func (e *errorCache) Get(_ context.Context, _ string, _ any) (bool, error)          { return false, e.getErr }
func (e *errorCache) Set(_ context.Context, _ string, _ any, _ time.Duration) error { return nil }
func (e *errorCache) Delete(_ context.Context, _ ...string) error                   { return nil }

func TestResolveIdentity_CacheGetError_StillResolvesViaDB(t *testing.T) {
	key := newTestRSAKey(t)
	srv := startTestJWKSServer(t, key, testKid)
	initIdentityTestPipeline(t, key, srv.URL, "")

	usr := newFakeUserService()
	usr.seed("u-cacheerr", "ce@example.com", usermodel.StatusActive, nil)

	badCache := &errorCache{getErr: errors.New("redis timeout")}
	mw := RequireIdentity(usr, badCache, 60*time.Second, logger.NewNoop())
	var got identity.Identity
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = identity.MustFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedIdentityRequest(t, key, testKid, "u-cacheerr", "ce@example.com", "user"))
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "u-cacheerr", got.UserID)
}

func TestResolveIdentity_CachedSuspendedUser_403(t *testing.T) {
	key := newTestRSAKey(t)
	srv := startTestJWKSServer(t, key, testKid)
	initIdentityTestPipeline(t, key, srv.URL, "")

	usr := newFakeUserService()
	usr.seed("u-susp-cached", "s@example.com", usermodel.StatusSuspended, nil)

	c := cache.NewMemoryCache()
	mw := RequireIdentity(usr, c, 60*time.Second, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request populates the cache with suspended identity.
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, signedIdentityRequest(t, key, testKid, "u-susp-cached", "s@example.com", "user"))
	assert.Equal(t, http.StatusForbidden, rr1.Code)

	// Second request: cached suspended user must still return 403.
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, signedIdentityRequest(t, key, testKid, "u-susp-cached", "s@example.com", "user"))
	assert.Equal(t, http.StatusForbidden, rr2.Code)
}

func TestResolveIdentity_DisplayName_NonNil(t *testing.T) {
	key := newTestRSAKey(t)
	srv := startTestJWKSServer(t, key, testKid)
	initIdentityTestPipeline(t, key, srv.URL, "")

	usr := newFakeUserService()
	name := "Alice"
	usr.seed("u-named", "alice@example.com", usermodel.StatusActive, &name)

	mw := RequireIdentity(usr, cache.NewMemoryCache(), 60*time.Second, logger.NewNoop())
	var got identity.Identity
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = identity.MustFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedIdentityRequest(t, key, testKid, "u-named", "alice@example.com", "user"))
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "Alice", got.DisplayName)
}

func TestDerefString(t *testing.T) {
	assert.Equal(t, "", derefString(nil))
	s := "hello"
	assert.Equal(t, "hello", derefString(&s))
}

func TestIsNotFound_NonServiceError(t *testing.T) {
	assert.False(t, isNotFound(errors.New("some other error")))
}

func TestIsNotFound_Nil(t *testing.T) {
	assert.False(t, isNotFound(nil))
}

// provisionErrorUserService returns ErrNotFound from Get (triggering provision)
// and an error from Provision.
type provisionErrorUserService struct{}

func (e *provisionErrorUserService) Get(_ context.Context, _ string) (*usermodel.User, error) {
	return nil, domainerrs.ErrNotFound.WithMessage("not found")
}
func (e *provisionErrorUserService) Provision(_ context.Context, _, _ string) (*usermodel.User, error) {
	return nil, errors.New("provision: db constraint violation")
}

func TestResolveIdentity_ProvisionError_500(t *testing.T) {
	key := newTestRSAKey(t)
	srv := startTestJWKSServer(t, key, testKid)
	initIdentityTestPipeline(t, key, srv.URL, "")

	usr := &provisionErrorUserService{}
	mw := RequireIdentity(usr, cache.NewMemoryCache(), time.Second, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedIdentityRequest(t, key, testKid, "u-prov-err", "p@example.com", "user"))
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// writeErrorCache implements cache.Cache with a Set that returns an error.
type writeErrorCache struct{ setErr error }

func (w *writeErrorCache) Get(_ context.Context, _ string, _ any) (bool, error) { return false, nil }
func (w *writeErrorCache) Set(_ context.Context, _ string, _ any, _ time.Duration) error {
	return w.setErr
}
func (w *writeErrorCache) Delete(_ context.Context, _ ...string) error { return nil }

func TestResolveIdentity_CacheSetError_StillSucceeds(t *testing.T) {
	key := newTestRSAKey(t)
	srv := startTestJWKSServer(t, key, testKid)
	initIdentityTestPipeline(t, key, srv.URL, "")

	usr := newFakeUserService()
	usr.seed("u-setcache-err", "sc@example.com", usermodel.StatusActive, nil)

	badWrite := &writeErrorCache{setErr: errors.New("redis write error")}
	mw := RequireIdentity(usr, badWrite, 60*time.Second, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedIdentityRequest(t, key, testKid, "u-setcache-err", "sc@example.com", "user"))
	assert.Equal(t, http.StatusOK, rr.Code)
}
