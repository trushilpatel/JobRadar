package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/config"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/cache"
	rolemodel "github.com/trushilpatel/jobradar/service/role/models"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
)

// fakeUserService is a hand-rolled in-memory userService used by tests so we
// don't depend on Bucket 4. Get returns ErrNotFound (or the typed
// ServiceError equivalent) when a user has not been provisioned.
type fakeUserService struct {
	mu          sync.Mutex
	users       map[string]*usermodel.User
	provisioned map[string]int // count of Provision calls per id
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

// fakeRoleService records grants and serves role lists. Grant is idempotent —
// duplicate grants are silently ignored.
type fakeRoleService struct {
	mu     sync.Mutex
	roles  map[string]map[string]struct{}
	grants []roleGrant
}

type roleGrant struct {
	UserID string
	RoleID string
}

func newFakeRoleService() *fakeRoleService {
	return &fakeRoleService{roles: map[string]map[string]struct{}{}}
}

func (f *fakeRoleService) seed(userID string, roles ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.roles[userID]; !ok {
		f.roles[userID] = map[string]struct{}{}
	}
	for _, r := range roles {
		f.roles[userID][r] = struct{}{}
	}
}

func (f *fakeRoleService) ListForUser(_ context.Context, userID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for r := range f.roles[userID] {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeRoleService) Grant(_ context.Context, userID, roleID string, _ *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.roles[userID]; !ok {
		f.roles[userID] = map[string]struct{}{}
	}
	if _, dup := f.roles[userID][roleID]; !dup {
		f.grants = append(f.grants, roleGrant{userID, roleID})
	}
	f.roles[userID][roleID] = struct{}{}
	return nil
}

func (f *fakeRoleService) hasRole(userID, role string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.roles[userID][role]
	return ok
}

// initIdentityTestPipeline boots the JWT pipeline with HS256 dev-token bypass
// so identity tests can authenticate without crafting JWTs.
func initIdentityTestPipeline(t *testing.T, devToken string) {
	t.Helper()
	cfg := config.AuthConfig{
		SupabaseJWTSecret: "test-secret",
	}
	if _, err := Auth(cfg, devToken, logger.NewNoop()); err != nil {
		t.Fatalf("Auth init: %v", err)
	}
}

// requestWithDevToken returns a request that the dev-bypass branch of
// VerifyJWT will accept — sub=dev-bypass, email="".
func requestWithDevToken(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	r.Header.Set("X-Dev-Token", token)
	return r
}

// signedRequest builds a real HS256 JWT carrying the given subject + email
// so identity tests can drive RequireIdentity through the full VerifyJWT
// path with deterministic claims.
func signedRequest(t *testing.T, subject, email string) *http.Request {
	t.Helper()
	tok := signToken(t, tokenOptions{
		alg:       "HS256",
		hmacKey:   []byte(testHMACSecret),
		issuer:    testIssuer,
		audience:  testAudience,
		email:     email,
		expiresAt: time.Now().Add(5 * time.Minute),
		subject:   subject,
	})
	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	return r
}

// initIdentityTestPipelineHS sets up the JWT pipeline for HS256 signed tokens
// (so signedRequest works). AllowedEmail is irrelevant — RequireIdentity does
// not enforce it.
func initIdentityTestPipelineHS(t *testing.T) {
	t.Helper()
	cfg := baseHSConfig()
	if _, err := Auth(cfg, "", logger.NewNoop()); err != nil {
		t.Fatalf("Auth init: %v", err)
	}
}

func TestRequireIdentity_FirstLogin_ProvisionsUser(t *testing.T) {
	initIdentityTestPipelineHS(t)
	usr := newFakeUserService()
	rls := newFakeRoleService()

	mw := RequireIdentity(usr, rls, cache.NewMemoryCache(), 60*time.Second, logger.NewNoop())
	var got identity.Identity
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = identity.MustFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedRequest(t, "user-new", "new@example.com"))
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d want 200", rr.Code)
	}
	if usr.provisioned["user-new"] != 1 {
		t.Fatalf("Provision should be called once, got %d", usr.provisioned["user-new"])
	}
	if got.UserID != "user-new" || got.Email != "new@example.com" {
		t.Fatalf("identity=%+v", got)
	}
	if got.Status != usermodel.StatusActive {
		t.Fatalf("status=%q want active", got.Status)
	}
}

func TestRequireIdentity_SuspendedUser_403(t *testing.T) {
	initIdentityTestPipelineHS(t)
	usr := newFakeUserService()
	usr.seed("u-bad", "bad@example.com", usermodel.StatusSuspended, nil)
	rls := newFakeRoleService()
	rls.seed("u-bad", rolemodel.RoleUser)

	mw := RequireIdentity(usr, rls, cache.NewMemoryCache(), 60*time.Second, logger.NewNoop())
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedRequest(t, "u-bad", "bad@example.com"))
	if called {
		t.Fatal("next must not be called for suspended user")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("got %d want 403", rr.Code)
	}
}

func TestRequireIdentity_CacheHit_AvoidsDBCall(t *testing.T) {
	initIdentityTestPipelineHS(t)
	usr := newFakeUserService()
	usr.seed("u-cached", "c@example.com", usermodel.StatusActive, nil)
	rls := newFakeRoleService()
	rls.seed("u-cached", rolemodel.RoleUser)

	c := cache.NewMemoryCache()
	mw := RequireIdentity(usr, rls, c, 60*time.Second, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First call populates cache.
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, signedRequest(t, "u-cached", "c@example.com"))
	if rr1.Code != http.StatusOK {
		t.Fatalf("first call: got %d", rr1.Code)
	}

	// Wipe service state. If the cache is consulted, the second call still
	// works; if it isn't, Get returns NotFound and Provision runs.
	preProv := usr.provisioned["u-cached"]
	usr.mu.Lock()
	delete(usr.users, "u-cached")
	usr.mu.Unlock()

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, signedRequest(t, "u-cached", "c@example.com"))
	if rr2.Code != http.StatusOK {
		t.Fatalf("second call: got %d", rr2.Code)
	}
	if usr.provisioned["u-cached"] != preProv {
		t.Fatalf("Provision was called after deletion — cache was bypassed")
	}
}

func TestRequireIdentity_BadJWT_401(t *testing.T) {
	initIdentityTestPipelineHS(t)
	usr := newFakeUserService()
	rls := newFakeRoleService()
	mw := RequireIdentity(usr, rls, cache.NewMemoryCache(), 60*time.Second, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r) // no Authorization header
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401", rr.Code)
	}
}

func TestRequireIdentity_DevTokenBypassWorks(t *testing.T) {
	const devToken = "test-dev-mid"
	initIdentityTestPipeline(t, devToken)
	usr := newFakeUserService()
	rls := newFakeRoleService()
	mw := RequireIdentity(usr, rls, cache.NewMemoryCache(), 60*time.Second, logger.NewNoop())
	var got identity.Identity
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = identity.MustFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, requestWithDevToken(devToken))
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d want 200", rr.Code)
	}
	if got.UserID != "dev-bypass" {
		t.Fatalf("user id=%q want dev-bypass", got.UserID)
	}
}

func TestRequireIdentity_BackfillsUserRole(t *testing.T) {
	initIdentityTestPipelineHS(t)
	usr := newFakeUserService()
	usr.seed("u-no-role", "x@example.com", usermodel.StatusActive, nil)
	rls := newFakeRoleService() // no roles seeded → backfill path

	mw := RequireIdentity(usr, rls, cache.NewMemoryCache(), 60*time.Second, logger.NewNoop())
	var got identity.Identity
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = identity.MustFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedRequest(t, "u-no-role", "x@example.com"))
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d want 200", rr.Code)
	}
	if !got.HasRole(rolemodel.RoleUser) {
		t.Fatalf("user role should be backfilled, roles=%v", got.Roles)
	}
}

// TestRequireIdentity_NilDeps verifies the constructor refuses to wire up
// without a real userService / roleService — silent nil acceptance would
// produce a runtime panic in the hot path which is harder to debug.
func TestRequireIdentity_NilDeps(t *testing.T) {
	cases := []struct {
		name string
		u    userService
		r    roleService
	}{
		{"nil user", nil, newFakeRoleService()},
		{"nil role", newFakeUserService(), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic")
				}
			}()
			RequireIdentity(tc.u, tc.r, cache.NewMemoryCache(), time.Second, logger.NewNoop())
		})
	}
}

// erroringUserService surfaces a non-NotFound error from Get so we can verify
// the middleware does not mistake transport errors for "user not found".
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
	initIdentityTestPipelineHS(t)
	usr := &erroringUserService{err: errors.New("db boom")}
	rls := newFakeRoleService()
	mw := RequireIdentity(usr, rls, cache.NewMemoryCache(), time.Second, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedRequest(t, "u-err", "e@example.com"))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("got %d want 500", rr.Code)
	}
}
