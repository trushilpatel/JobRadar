package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
)

const (
	testIssuer = "https://example.clerk.accounts.dev"
	testKid    = "test-key-id"
)

// generateRSAKey generates a 2048-bit RSA key for test use.
func generateRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

// makeJWKSServer starts an httptest.Server serving a JWKS document for the
// given RSA private key. The server is registered for cleanup.
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

// makeClerkToken builds a signed RS256 JWT with Clerk-style claims.
func makeClerkToken(t *testing.T, key *rsa.PrivateKey, kid, issuer, sub, email, role string, exp time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": sub, "email": email, "role": role,
		"iss": issuer, "iat": time.Now().Unix(), "exp": exp.Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	require.NoError(t, err)
	return signed
}

// clerkAuthConfig builds a config.AuthConfig pointing at a test JWKS server.
func clerkAuthConfig(jwksURL string) config.AuthConfig {
	return config.AuthConfig{
		ClerkJWKSURL:   jwksURL,
		ClerkIssuerURL: testIssuer,
	}
}

// nextHandler returns a handler that records the identity on the context and
// writes 200 OK.
type observedIdentity struct {
	ident Identity
	saw   bool
}

func nextHandler(obs *observedIdentity) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := FromContext(r.Context())
		obs.ident = id
		obs.saw = ok
		w.WriteHeader(http.StatusOK)
	})
}

func doRequest(t *testing.T, mw func(http.Handler) http.Handler, bearer string) (*httptest.ResponseRecorder, *observedIdentity) {
	t.Helper()
	obs := &observedIdentity{}
	h := mw(nextHandler(obs))
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w, obs
}

func TestAuth_ValidToken(t *testing.T) {
	key := generateRSAKey(t)
	srv := makeJWKSServer(t, key, testKid)

	cfg := clerkAuthConfig(srv.URL)
	mw, err := Auth(cfg, "", logger.NewNoop())
	require.NoError(t, err)

	tok := makeClerkToken(t, key, testKid, testIssuer, "user-123", "test@example.com", "user", time.Now().Add(5*time.Minute))
	w, obs := doRequest(t, mw, "Bearer "+tok)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, obs.saw)
	assert.Equal(t, "user-123", obs.ident.UserID)
	assert.Equal(t, "test@example.com", obs.ident.Email)
}

func TestAuth_ExpiredToken(t *testing.T) {
	key := generateRSAKey(t)
	srv := makeJWKSServer(t, key, testKid)

	cfg := clerkAuthConfig(srv.URL)
	mw, err := Auth(cfg, "", logger.NewNoop())
	require.NoError(t, err)

	// Expired well beyond clock skew.
	tok := makeClerkToken(t, key, testKid, testIssuer, "user-123", "test@example.com", "user", time.Now().Add(-10*time.Minute))
	w, _ := doRequest(t, mw, "Bearer "+tok)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_MissingBearer(t *testing.T) {
	key := generateRSAKey(t)
	srv := makeJWKSServer(t, key, testKid)

	cfg := clerkAuthConfig(srv.URL)
	mw, err := Auth(cfg, "", logger.NewNoop())
	require.NoError(t, err)

	w, _ := doRequest(t, mw, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_WrongIssuer(t *testing.T) {
	key := generateRSAKey(t)
	srv := makeJWKSServer(t, key, testKid)

	cfg := clerkAuthConfig(srv.URL)
	mw, err := Auth(cfg, "", logger.NewNoop())
	require.NoError(t, err)

	// Token signed with valid key but wrong iss.
	claims := jwt.MapClaims{
		"sub": "user-x", "email": "x@e.com", "role": "user",
		"iss": "https://evil.attacker.dev",
		"iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testKid
	signed, err := tok.SignedString(key)
	require.NoError(t, err)

	w, _ := doRequest(t, mw, "Bearer "+signed)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_DevTokenBypass(t *testing.T) {
	key := generateRSAKey(t)
	srv := makeJWKSServer(t, key, testKid)

	const devToken = "test-dev-secret-abc"
	cfg := clerkAuthConfig(srv.URL)
	mw, err := Auth(cfg, devToken, logger.NewNoop())
	require.NoError(t, err)

	obs := &observedIdentity{}
	h := mw(nextHandler(obs))
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	req.Header.Set("X-Dev-Token", devToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "dev-bypass", obs.ident.UserID)
}

func TestAuth_WrongDevToken_401(t *testing.T) {
	key := generateRSAKey(t)
	srv := makeJWKSServer(t, key, testKid)

	cfg := clerkAuthConfig(srv.URL)
	mw, err := Auth(cfg, "correct-token", logger.NewNoop())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	req.Header.Set("X-Dev-Token", "wrong-token")
	w := httptest.NewRecorder()
	mw(nextHandler(&observedIdentity{})).ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_DevTokenDisabledWhenEmpty(t *testing.T) {
	key := generateRSAKey(t)
	srv := makeJWKSServer(t, key, testKid)

	cfg := clerkAuthConfig(srv.URL)
	mw, err := Auth(cfg, "", logger.NewNoop()) // devToken="" → bypass disabled
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	req.Header.Set("X-Dev-Token", "anything")
	w := httptest.NewRecorder()
	mw(nextHandler(&observedIdentity{})).ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVerifyJWT_ReturnsRoleFromClaims(t *testing.T) {
	key := generateRSAKey(t)
	srv := makeJWKSServer(t, key, testKid)

	cfg := clerkAuthConfig(srv.URL)
	_, err := Auth(cfg, "", logger.NewNoop())
	require.NoError(t, err)

	tok := makeClerkToken(t, key, testKid, testIssuer, "user-admin", "admin@example.com", "admin", time.Now().Add(5*time.Minute))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+tok)

	claims, err := VerifyJWT(r)
	require.NoError(t, err)
	assert.Equal(t, "user-admin", claims.Subject)
	assert.Equal(t, "admin@example.com", claims.Email)
	assert.Equal(t, "admin", claims.Role)
}

func TestVerifyJWT_EmptyRoleClaimReturnsEmptyRole(t *testing.T) {
	key := generateRSAKey(t)
	srv := makeJWKSServer(t, key, testKid)

	cfg := clerkAuthConfig(srv.URL)
	_, err := Auth(cfg, "", logger.NewNoop())
	require.NoError(t, err)

	// No role claim in token.
	claims := jwt.MapClaims{
		"sub": "user-norole", "email": "norole@example.com",
		"iss": testIssuer, "iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testKid
	signed, err := tok.SignedString(key)
	require.NoError(t, err)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+signed)

	vc, err := VerifyJWT(r)
	require.NoError(t, err)
	assert.Equal(t, "", vc.Role)
}

func TestVerifyJWT_DevTokenBypass(t *testing.T) {
	key := generateRSAKey(t)
	srv := makeJWKSServer(t, key, testKid)

	const devToken = "verify-dev-token"
	cfg := clerkAuthConfig(srv.URL)
	_, err := Auth(cfg, devToken, logger.NewNoop())
	require.NoError(t, err)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Dev-Token", devToken)

	vc, err := VerifyJWT(r)
	require.NoError(t, err)
	assert.Equal(t, "dev-bypass", vc.Subject)
	assert.Equal(t, "admin", vc.Role)
}

func TestVerifyJWT_MissingBearer(t *testing.T) {
	key := generateRSAKey(t)
	srv := makeJWKSServer(t, key, testKid)

	cfg := clerkAuthConfig(srv.URL)
	_, err := Auth(cfg, "", logger.NewNoop())
	require.NoError(t, err)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err = VerifyJWT(r)
	assert.Error(t, err)
}

func TestVerifyJWT_PipelineNotInitialised(t *testing.T) {
	// Temporarily nil out the global pipeline slot.
	pipelineMu.Lock()
	saved := pipelineSlot
	pipelineSlot = nil
	pipelineMu.Unlock()
	t.Cleanup(func() {
		pipelineMu.Lock()
		pipelineSlot = saved
		pipelineMu.Unlock()
	})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := VerifyJWT(r)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "before Auth was initialised")
}

func TestFromContext_NoIdentity(t *testing.T) {
	_, ok := FromContext(context.Background())
	assert.False(t, ok)
}

func TestExtractBearer(t *testing.T) {
	cases := []struct {
		in     string
		tok    string
		wantOK bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer abc", "abc", true},
		{"Bearer  spaced  ", "spaced", true},
		{"", "", false},
		{"Bearer", "", false},
		{"Bearer ", "", false},
		{"Bearer   ", "", false},
		{"Token abc", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			tok, ok := extractBearer(tc.in)
			assert.Equal(t, tc.wantOK, ok)
			if ok {
				assert.Equal(t, tc.tok, tok)
			}
		})
	}
}

func TestAuth_MalformedBearer_Cases(t *testing.T) {
	key := generateRSAKey(t)
	srv := makeJWKSServer(t, key, testKid)
	cfg := clerkAuthConfig(srv.URL)
	mw, err := Auth(cfg, "", logger.NewNoop())
	require.NoError(t, err)

	for _, h := range []string{"Token abc", "Bearer", "Bearer ", "abc"} {
		t.Run(h, func(t *testing.T) {
			w, _ := doRequest(t, mw, h)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestVerifyToken_MissingExp(t *testing.T) {
	key := generateRSAKey(t)
	srv := makeJWKSServer(t, key, testKid)

	cfg := clerkAuthConfig(srv.URL)
	_, err := Auth(cfg, "", logger.NewNoop())
	require.NoError(t, err)

	p := loadPipeline()
	require.NotNil(t, p)

	// Build a token without exp — jwt library may or may not reject this;
	// verifyToken checks exp explicitly after parse.
	// We test verifyToken directly by creating a token where ExpiresAt would
	// be omitted. jwt.MapClaims without "exp" key leaves it out.
	rawClaims := jwt.MapClaims{
		"sub": "u", "email": "u@e.com",
		"iss": testIssuer, "iat": time.Now().Unix(),
		// deliberately no "exp"
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, rawClaims)
	tok.Header["kid"] = testKid
	signed, err := tok.SignedString(key)
	require.NoError(t, err)

	// Parsing with WithIssuedAt() will reject tokens without exp in newer jwt
	// versions. Either way, verifyToken should return an error.
	_, verifyErr := verifyToken(p, signed)
	assert.Error(t, verifyErr)
}
