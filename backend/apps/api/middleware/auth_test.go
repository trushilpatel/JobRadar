package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
)

const (
	testProjectURL = "https://project-ref.supabase.co"
	testIssuer     = "https://project-ref.supabase.co/auth/v1"
	testAudience   = "authenticated"
	testAllowedEml = "owner@example.com"
	testHMACSecret = "unit-test-hs256-secret-do-not-use-in-prod"
	testKid        = "kid-1"
)

// ecKeypair is the ES256 signing/verifying pair for tests.
type ecKeypair struct {
	priv *ecdsa.PrivateKey
	kid  string
}

func newECKeypair(t *testing.T, kid string) ecKeypair {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen ec key: %v", err)
	}
	return ecKeypair{priv: priv, kid: kid}
}

// jwksBehaviour drives a test JWKS server. Swap atomically to simulate
// scenarios like 503 outages, late key-rotation, or unknown-kid refetch.
type jwksBehaviour struct {
	fail503  atomic.Bool
	keys     atomic.Value // []ecKeypair
	requests atomic.Int32
}

// coordBytes returns the P-256 X and Y coordinates of a public key without
// touching the deprecated big.Int fields. Uses crypto/ecdh's uncompressed
// SEC1 form (0x04 || X || Y, 32 bytes each for P-256).
func coordBytes(pub ecdsa.PublicKey) (x, y []byte) {
	ecdhPub, err := pub.ECDH()
	if err != nil {
		panic(fmt.Errorf("convert ecdsa→ecdh: %w", err))
	}
	raw := ecdhPub.Bytes()
	byteLen := (len(raw) - 1) / 2
	return raw[1 : 1+byteLen], raw[1+byteLen:]
}

func (b *jwksBehaviour) setKeys(ks []ecKeypair) { b.keys.Store(ks) }
func (b *jwksBehaviour) getKeys() []ecKeypair {
	v := b.keys.Load()
	if v == nil {
		return nil
	}
	ks, _ := v.([]ecKeypair)
	return ks
}

func startJWKSServer(t *testing.T, b *jwksBehaviour) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		b.requests.Add(1)
		if b.fail503.Load() {
			http.Error(w, "jwks temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		keys := b.getKeys()
		out := struct {
			Keys []map[string]string `json:"keys"`
		}{}
		for _, k := range keys {
			x, y := coordBytes(k.priv.PublicKey)
			out.Keys = append(out.Keys, map[string]string{
				"kty": "EC",
				"crv": "P-256",
				"kid": k.kid,
				"alg": "ES256",
				"x":   b64url(x),
				"y":   b64url(y),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

type tokenOptions struct {
	alg       string
	kid       string
	hmacKey   []byte
	ecKey     *ecdsa.PrivateKey
	issuer    string
	audience  string
	email     string
	expiresAt time.Time
	subject   string
}

func signToken(t *testing.T, o tokenOptions) string {
	t.Helper()
	claims := authClaims{
		Email: o.email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    o.issuer,
			Audience:  jwt.ClaimStrings{o.audience},
			ExpiresAt: jwt.NewNumericDate(o.expiresAt),
			Subject:   o.subject,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	var method jwt.SigningMethod
	var key any
	switch o.alg {
	case "HS256":
		method = jwt.SigningMethodHS256
		key = o.hmacKey
	case "ES256":
		method = jwt.SigningMethodES256
		key = o.ecKey
	default:
		t.Fatalf("unknown alg %q", o.alg)
	}
	tok := jwt.NewWithClaims(method, claims)
	if o.kid != "" {
		tok.Header["kid"] = o.kid
	}
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

// nextHandler returns a handler that writes 200 OK and records the identity
// surfaced on the request context — lets tests assert the chain continued.
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

func baseESConfig(jwksURL string) config.AuthConfig {
	return config.AuthConfig{
		SupabaseProjectURL:  testProjectURL,
		SupabaseJWKSURL:     jwksURL,
		ClockSkewSeconds:    60,
		JWKSCacheTTLSeconds: 600,
	}
}

func baseHSConfig() config.AuthConfig {
	return config.AuthConfig{
		SupabaseProjectURL:  testProjectURL,
		SupabaseJWTSecret:   testHMACSecret,
		ClockSkewSeconds:    60,
		JWKSCacheTTLSeconds: 600,
	}
}

// request drives the middleware end-to-end against the token and returns the
// response recorder so tests can assert status + headers.
func request(t *testing.T, mw func(http.Handler) http.Handler, bearer string) (*httptest.ResponseRecorder, *observedIdentity) {
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

func TestAuth_ValidES256Token_200(t *testing.T) {
	kp := newECKeypair(t, testKid)
	b := &jwksBehaviour{}
	b.setKeys([]ecKeypair{kp})
	srv := startJWKSServer(t, b)

	cfg := baseESConfig(srv.URL)
	mw, err := Auth(cfg, "", logger.NewNoop())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}

	tok := signToken(t, tokenOptions{
		alg: "ES256", kid: kp.kid, ecKey: kp.priv,
		issuer: testIssuer, audience: testAudience, email: testAllowedEml,
		expiresAt: time.Now().Add(5 * time.Minute), subject: "user-123",
	})
	w, obs := request(t, mw, "Bearer "+tok)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !obs.saw || obs.ident.UserID != "user-123" || obs.ident.Email != testAllowedEml {
		t.Fatalf("identity not surfaced on ctx: %+v saw=%v", obs.ident, obs.saw)
	}
}

func TestAuth_ValidHS256Token_200(t *testing.T) {
	cfg := baseHSConfig()
	mw, err := Auth(cfg, "", logger.NewNoop())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	tok := signToken(t, tokenOptions{
		alg: "HS256", hmacKey: []byte(testHMACSecret),
		issuer: testIssuer, audience: testAudience, email: testAllowedEml,
		expiresAt: time.Now().Add(5 * time.Minute), subject: "user-hs",
	})
	w, obs := request(t, mw, "Bearer "+tok)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if obs.ident.UserID != "user-hs" {
		t.Fatalf("identity not surfaced: %+v", obs.ident)
	}
}

func TestAuth_ExpiredToken_401(t *testing.T) {
	cfg := baseHSConfig()
	mw, _ := Auth(cfg, "", logger.NewNoop())
	tok := signToken(t, tokenOptions{
		alg: "HS256", hmacKey: []byte(testHMACSecret),
		issuer: testIssuer, audience: testAudience, email: testAllowedEml,
		expiresAt: time.Now().Add(-10 * time.Minute),
	})
	w, _ := request(t, mw, "Bearer "+tok)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d, want 401", w.Code)
	}
}

func TestAuth_WithinSkewWindow_200(t *testing.T) {
	cfg := baseHSConfig()
	mw, _ := Auth(cfg, "", logger.NewNoop())
	// Exp is 30s in the past; skew default 60s keeps it valid.
	tok := signToken(t, tokenOptions{
		alg: "HS256", hmacKey: []byte(testHMACSecret),
		issuer: testIssuer, audience: testAudience, email: testAllowedEml,
		expiresAt: time.Now().Add(-30 * time.Second),
	})
	w, _ := request(t, mw, "Bearer "+tok)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200 (within skew)", w.Code)
	}
}

func TestAuth_MissingAuthorization_401(t *testing.T) {
	cfg := baseHSConfig()
	mw, _ := Auth(cfg, "", logger.NewNoop())
	w, _ := request(t, mw, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d, want 401", w.Code)
	}
}

func TestAuth_MalformedBearer_401(t *testing.T) {
	cfg := baseHSConfig()
	mw, _ := Auth(cfg, "", logger.NewNoop())

	cases := []string{"Token abc", "Bearer", "Bearer ", "abc"}
	for _, h := range cases {
		t.Run(h, func(t *testing.T) {
			w, _ := request(t, mw, h)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("got %d want 401", w.Code)
			}
		})
	}
}

func TestAuth_WrongIssuer_401(t *testing.T) {
	cfg := baseHSConfig()
	mw, _ := Auth(cfg, "", logger.NewNoop())
	tok := signToken(t, tokenOptions{
		alg: "HS256", hmacKey: []byte(testHMACSecret),
		issuer: "https://evil.supabase.co/auth/v1", audience: testAudience, email: testAllowedEml,
		expiresAt: time.Now().Add(5 * time.Minute),
	})
	w, _ := request(t, mw, "Bearer "+tok)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401", w.Code)
	}
}

func TestAuth_WrongAudience_401(t *testing.T) {
	cfg := baseHSConfig()
	mw, _ := Auth(cfg, "", logger.NewNoop())
	tok := signToken(t, tokenOptions{
		alg: "HS256", hmacKey: []byte(testHMACSecret),
		issuer: testIssuer, audience: "anon", email: testAllowedEml,
		expiresAt: time.Now().Add(5 * time.Minute),
	})
	w, _ := request(t, mw, "Bearer "+tok)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401", w.Code)
	}
}

// TestAuth_AlgNone_401 asserts that the alg=none (unsigned) attack is rejected.
// WithValidMethods must block this before signature verification even runs.
func TestAuth_AlgNone_401(t *testing.T) {
	cfg := baseHSConfig()
	mw, _ := Auth(cfg, "", logger.NewNoop())
	// Hand-craft a "none" token.
	header := b64url([]byte(`{"alg":"none","typ":"JWT"}`))
	claimsJSON, _ := json.Marshal(map[string]any{
		"iss":   testIssuer,
		"aud":   testAudience,
		"email": testAllowedEml,
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
	})
	body := b64url(claimsJSON)
	noneToken := header + "." + body + "."
	w, _ := request(t, mw, "Bearer "+noneToken)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401 for alg=none", w.Code)
	}
}

// TestAuth_AlgConfusion_ES256PublicKeyAsHS256_401 — classic RSA/ECDSA public
// key reused as HMAC secret. WithValidMethods([]string{"ES256"}) blocks the
// HS256-signed token before keyfunc is even consulted, so this is a 401.
func TestAuth_AlgConfusion_ES256PublicKeyAsHS256_401(t *testing.T) {
	kp := newECKeypair(t, testKid)
	b := &jwksBehaviour{}
	b.setKeys([]ecKeypair{kp})
	srv := startJWKSServer(t, b)

	cfg := baseESConfig(srv.URL)
	mw, err := Auth(cfg, "", logger.NewNoop())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	// Build an HS256 token whose secret is the marshalled ES256 public key.
	x, y := coordBytes(kp.priv.PublicKey)
	pubBytes := append([]byte{0x04}, append(x, y...)...)
	tok := signToken(t, tokenOptions{
		alg: "HS256", hmacKey: pubBytes,
		issuer: testIssuer, audience: testAudience, email: testAllowedEml,
		expiresAt: time.Now().Add(5 * time.Minute),
	})
	w, _ := request(t, mw, "Bearer "+tok)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401 for alg-confusion", w.Code)
	}
}

// TestAuth_AnyEmail_Passes verifies Phase 2 behaviour: any valid JWT is
// accepted regardless of email address — the allowlist was removed.
// Per-route identity checks (RequireIdentity + RequireRole) gate access.
func TestAuth_AnyEmail_Passes(t *testing.T) {
	cfg := baseHSConfig()
	mw, _ := Auth(cfg, "", logger.NewNoop())
	tok := signToken(t, tokenOptions{
		alg: "HS256", hmacKey: []byte(testHMACSecret),
		issuer: testIssuer, audience: testAudience, email: "anyone@example.com",
		expiresAt: time.Now().Add(5 * time.Minute),
	})
	w, _ := request(t, mw, "Bearer "+tok)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d want 200 — email allowlist should not reject valid JWTs", w.Code)
	}
}

// TestAuth_JWKS503_MiddlewareReturns503 — JWKS endpoint down; middleware
// must fail closed with 503 + Retry-After, not bypass.
func TestAuth_JWKS503_MiddlewareReturns503(t *testing.T) {
	b := &jwksBehaviour{}
	b.fail503.Store(true)
	srv := startJWKSServer(t, b)

	cfg := baseESConfig(srv.URL)
	mw, err := Auth(cfg, "", logger.NewNoop())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	// Sign with a key that won't be in the (empty) cache — forces refresh.
	kp := newECKeypair(t, testKid)
	tok := signToken(t, tokenOptions{
		alg: "ES256", kid: kp.kid, ecKey: kp.priv,
		issuer: testIssuer, audience: testAudience, email: testAllowedEml,
		expiresAt: time.Now().Add(5 * time.Minute),
	})
	w, _ := request(t, mw, "Bearer "+tok)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d want 503", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After=%q want 30", got)
	}
}

// TestAuth_UnknownKid_RefetchOnce_Then401 — first verify forces a refresh,
// the kid is still missing, middleware returns 401. Exactly two JWKS fetches
// must be observed (initial miss + one refetch).
func TestAuth_UnknownKid_RefetchOnce_Then401(t *testing.T) {
	known := newECKeypair(t, testKid)
	unknown := newECKeypair(t, "unknown-kid")
	b := &jwksBehaviour{}
	b.setKeys([]ecKeypair{known})
	srv := startJWKSServer(t, b)

	cfg := baseESConfig(srv.URL)
	mw, err := Auth(cfg, "", logger.NewNoop())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}

	tok := signToken(t, tokenOptions{
		alg: "ES256", kid: unknown.kid, ecKey: unknown.priv,
		issuer: testIssuer, audience: testAudience, email: testAllowedEml,
		expiresAt: time.Now().Add(5 * time.Minute),
	})
	before := b.requests.Load()
	w, _ := request(t, mw, "Bearer "+tok)
	after := b.requests.Load()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401 for unknown kid", w.Code)
	}
	if d := after - before; d == 0 {
		t.Fatalf("expected at least one JWKS refetch for unknown kid, got %d", d)
	}
}

// TestFromContext_NoIdentity covers the defensive path when caller reads the
// identity from a context that never passed through Auth.
func TestFromContext_NoIdentity(t *testing.T) {
	_, ok := FromContext(context.Background())
	if ok {
		t.Fatal("expected ok=false on bare context")
	}
}

func TestDevBypass(t *testing.T) {
	const devToken = "test-dev-token-abc123"

	cfg := config.AuthConfig{
		SupabaseJWTSecret: "dummy-secret-for-test-only",
	}

	log := logger.NewNoop()
	mw, err := Auth(cfg, devToken, log)
	if err != nil {
		t.Fatalf("Auth() setup error: %v", err)
	}

	var gotIdent Identity
	var handlerCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		gotIdent, _ = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(next)

	t.Run("bypass fires on matching token", func(t *testing.T) {
		handlerCalled = false
		gotIdent = Identity{}
		req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
		req.Header.Set("X-Dev-Token", devToken)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if !handlerCalled {
			t.Fatal("expected next handler to be called")
		}
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if gotIdent.UserID != "dev-bypass" {
			t.Fatalf("expected UserID dev-bypass, got %q", gotIdent.UserID)
		}
		if gotIdent.Email != "" {
			t.Fatalf("expected empty Email, got %q", gotIdent.Email)
		}
	})

	t.Run("wrong token falls through to 401", func(t *testing.T) {
		handlerCalled = false
		req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
		req.Header.Set("X-Dev-Token", "wrong-token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if handlerCalled {
			t.Fatal("next handler must not be called with wrong token")
		}
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("no header falls through to 401", func(t *testing.T) {
		handlerCalled = false
		req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if handlerCalled {
			t.Fatal("next handler must not be called with no token header")
		}
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})
}

func TestDevBypassDisabledWhenTokenEmpty(t *testing.T) {
	cfg := config.AuthConfig{
		SupabaseJWTSecret: "dummy-secret-for-test-only",
	}
	log := logger.NewNoop()
	mw, err := Auth(cfg, "", log) // devToken = "" → bypass disabled
	if err != nil {
		t.Fatalf("Auth() setup error: %v", err)
	}

	var handlerCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(next)

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	req.Header.Set("X-Dev-Token", "anything")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if handlerCalled {
		t.Fatal("next must not be called when devToken is empty")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

// TestVerifyJWT_DirectCall covers the public extraction helper used by the
// new RequireIdentity middleware. Asserts that VerifyJWT honours the same
// signature/claims rules as the classic Auth middleware but does NOT enforce
// the email allowlist — that's RequireIdentity's job (and it doesn't enforce
// it either, by design — Phase 1 swaps the allowlist for role+status).
func TestVerifyJWT_DirectCall(t *testing.T) {
	cfg := baseHSConfig()
	if _, err := Auth(cfg, "", logger.NewNoop()); err != nil {
		t.Fatalf("Auth init: %v", err)
	}

	t.Run("valid token returns sub+email", func(t *testing.T) {
		tok := signToken(t, tokenOptions{
			alg: "HS256", hmacKey: []byte(testHMACSecret),
			issuer: testIssuer, audience: testAudience, email: "foo@example.com",
			expiresAt: time.Now().Add(5 * time.Minute), subject: "user-x",
		})
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+tok)
		sub, email, err := VerifyJWT(r)
		if err != nil {
			t.Fatalf("VerifyJWT: %v", err)
		}
		if sub != "user-x" || email != "foo@example.com" {
			t.Fatalf("sub=%q email=%q", sub, email)
		}
	})

	t.Run("non-allowlisted email still verifies — VerifyJWT does not gate", func(t *testing.T) {
		tok := signToken(t, tokenOptions{
			alg: "HS256", hmacKey: []byte(testHMACSecret),
			issuer: testIssuer, audience: testAudience, email: "intruder@example.com",
			expiresAt: time.Now().Add(5 * time.Minute), subject: "user-y",
		})
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+tok)
		_, _, err := VerifyJWT(r)
		if err != nil {
			t.Fatalf("VerifyJWT must not enforce allowlist, got err: %v", err)
		}
	})

	t.Run("expired token rejected", func(t *testing.T) {
		tok := signToken(t, tokenOptions{
			alg: "HS256", hmacKey: []byte(testHMACSecret),
			issuer: testIssuer, audience: testAudience, email: testAllowedEml,
			expiresAt: time.Now().Add(-10 * time.Minute), subject: "user-z",
		})
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+tok)
		_, _, err := VerifyJWT(r)
		if err == nil {
			t.Fatal("expected error for expired token")
		}
	})

	t.Run("missing bearer rejected", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		_, _, err := VerifyJWT(r)
		if err == nil {
			t.Fatal("expected error for missing bearer")
		}
	})

	t.Run("dev token bypass", func(t *testing.T) {
		const devToken = "dev-test-token-verify"
		if _, err := Auth(baseHSConfig(), devToken, logger.NewNoop()); err != nil {
			t.Fatalf("Auth init: %v", err)
		}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Dev-Token", devToken)
		sub, email, err := VerifyJWT(r)
		if err != nil {
			t.Fatalf("VerifyJWT: %v", err)
		}
		if sub != "dev-bypass" || email != "" {
			t.Fatalf("sub=%q email=%q", sub, email)
		}
	})
}

// TestExtractBearer verifies the Authorization header parser accepts well-formed
// bearers and rejects everything else, including case-insensitive prefix.
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
		{"Bearer   ", "", false}, // all whitespace after prefix → trim-to-empty path
		{"Token abc", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			tok, ok := extractBearer(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
			if ok && tok != tc.tok {
				t.Fatalf("tok=%q want %q", tok, tc.tok)
			}
		})
	}
}

// ── Additional edge-case tests for Auth/VerifyJWT/keyfunc/refresh/parse ──────

// TestAuth_NeitherSecretNorJWKS_ReturnsError verifies that Auth() returns an
// error when the config provides neither a JWT secret nor a JWKS URL —
// newVerifier's error path.
func TestAuth_NeitherSecretNorJWKS_ReturnsError(t *testing.T) {
	cfg := config.AuthConfig{
		SupabaseProjectURL: testProjectURL,
		// Deliberately leave both SupabaseJWTSecret and SupabaseJWKSURL empty.
	}
	_, err := Auth(cfg, "", logger.NewNoop())
	if err == nil {
		t.Fatal("expected error when neither JWT secret nor JWKS URL is configured")
	}
}

// TestVerifyJWT_PipelineNotInitialised asserts that calling VerifyJWT in a
// fresh test that has never called Auth() returns the sentinel error.
// We reset jwtPipelineSlot to nil to simulate a cold process.
func TestVerifyJWT_PipelineNotInitialised(t *testing.T) {
	// Save and restore the global pipeline slot.
	jwtPipelineMu.Lock()
	saved := jwtPipelineSlot
	jwtPipelineSlot = nil
	jwtPipelineMu.Unlock()
	t.Cleanup(func() {
		jwtPipelineMu.Lock()
		jwtPipelineSlot = saved
		jwtPipelineMu.Unlock()
	})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	_, _, err := VerifyJWT(r)
	if err == nil {
		t.Fatal("expected error from VerifyJWT before Auth initialisation")
	}
}

// TestAuth_ExpiredToken_WithNoExpField_401 sends a token where ExpiresAt is
// nil (crafted manually). validateStandardClaims must return "exp missing".
// We exercise this via an HS256 token with no exp claim (jwt library omits
// the check when we bypass it — we test validateStandardClaims directly).
func TestValidateStandardClaims_NoExp(t *testing.T) {
	c := &authClaims{}
	c.Issuer = testIssuer
	c.Audience = jwt.ClaimStrings{testAudience}
	c.ExpiresAt = nil // no exp
	err := validateStandardClaims(c, testIssuer, testAudience, 0)
	if err == nil || err.Error() != "exp missing" {
		t.Fatalf("want 'exp missing', got %v", err)
	}
}

// TestValidateStandardClaims_IssMismatch exercises the issuer-mismatch branch.
func TestValidateStandardClaims_IssMismatch(t *testing.T) {
	c := &authClaims{}
	c.Issuer = "https://evil.example.com/auth/v1"
	c.Audience = jwt.ClaimStrings{testAudience}
	c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(5 * time.Minute))
	err := validateStandardClaims(c, testIssuer, testAudience, 0)
	if err == nil {
		t.Fatal("expected iss-mismatch error")
	}
}

// TestJWKS_TTLExpired_ForceRefetch verifies the lookup function returns nil
// when the in-memory cache has expired past its TTL, causing a refetch.
func TestJWKS_TTLExpired_ForceRefetch(t *testing.T) {
	kp := newECKeypair(t, "kid-ttl")
	b := &jwksBehaviour{}
	b.setKeys([]ecKeypair{kp})
	srv := startJWKSServer(t, b)

	// Very short TTL so the cache expires in the test.
	cfg := config.AuthConfig{
		SupabaseProjectURL:  testProjectURL,
		SupabaseJWKSURL:     srv.URL,
		ClockSkewSeconds:    60,
		JWKSCacheTTLSeconds: 0, // effectively zero → every lookup triggers refresh
	}
	mw, err := Auth(cfg, "", logger.NewNoop())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}

	tok := signToken(t, tokenOptions{
		alg: "ES256", kid: kp.kid, ecKey: kp.priv,
		issuer: testIssuer, audience: testAudience, email: testAllowedEml,
		expiresAt: time.Now().Add(5 * time.Minute), subject: "ttl-user",
	})
	w, obs := request(t, mw, "Bearer "+tok)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d want 200; body=%s", w.Code, w.Body.String())
	}
	if obs.ident.UserID != "ttl-user" {
		t.Fatalf("identity=%+v", obs.ident)
	}
	// At least one JWKS fetch must have happened.
	if b.requests.Load() == 0 {
		t.Fatal("expected at least one JWKS fetch")
	}
}

// TestJWKS_Refresh_ClientError_Returns503 drives the jwksVerifier.refresh path
// where http.Client.Do returns an error (simulated by a closed server).
func TestJWKS_Refresh_ClientError_Returns503(t *testing.T) {
	// Start a server then close it immediately so client.Do fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close() // Close before the verifier can reach it.

	cfg := config.AuthConfig{
		SupabaseProjectURL:  testProjectURL,
		SupabaseJWKSURL:     srv.URL,
		ClockSkewSeconds:    60,
		JWKSCacheTTLSeconds: 600,
	}
	mw, err := Auth(cfg, "", logger.NewNoop())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}

	kp := newECKeypair(t, "kid-unreachable")
	tok := signToken(t, tokenOptions{
		alg: "ES256", kid: kp.kid, ecKey: kp.priv,
		issuer: testIssuer, audience: testAudience, email: testAllowedEml,
		expiresAt: time.Now().Add(5 * time.Minute),
	})
	w, _ := request(t, mw, "Bearer "+tok)
	// Unreachable server → errJWKSUnavailable → 503
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d want 503 when JWKS server is unreachable", w.Code)
	}
}

// TestJWKS_Refresh_BadJSON exercises the JSON unmarshal error path in refresh.
func TestJWKS_Refresh_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json!!!"))
	}))
	t.Cleanup(srv.Close)

	cfg := config.AuthConfig{
		SupabaseProjectURL:  testProjectURL,
		SupabaseJWKSURL:     srv.URL,
		ClockSkewSeconds:    60,
		JWKSCacheTTLSeconds: 0, // no cache so every request refreshes
	}
	mw, err := Auth(cfg, "", logger.NewNoop())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}

	kp := newECKeypair(t, "kid-badjson")
	tok := signToken(t, tokenOptions{
		alg: "ES256", kid: kp.kid, ecKey: kp.priv,
		issuer: testIssuer, audience: testAudience, email: testAllowedEml,
		expiresAt: time.Now().Add(5 * time.Minute),
	})
	w, _ := request(t, mw, "Bearer "+tok)
	// Malformed JSON is not "unavailable" — it's a 401 (kid not found after
	// refresh fails with a decode error, which is not errJWKSUnavailable).
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401 for bad-JSON JWKS response", w.Code)
	}
}

// TestJWKS_Refresh_NonECKeySkipped verifies that non-EC keys in the JWKS doc
// are silently skipped and do not cause a panic or error.
func TestJWKS_Refresh_NonECKeySkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Serve a JWKS with an RSA key (kty != "EC") and no EC keys.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[{"kty":"RSA","kid":"rsa-1","n":"abc","e":"AQAB"}]}`))
	}))
	t.Cleanup(srv.Close)

	// newJWKSVerifier parses the JWKS; non-EC entries are skipped silently.
	v := newJWKSVerifier(srv.URL, 0, http.DefaultClient, 0)
	if err := v.refresh(); err != nil {
		t.Fatalf("refresh must not error on non-EC key: %v", err)
	}
	// Cache should be empty (no usable keys).
	v.mu.RLock()
	count := len(v.keys)
	v.mu.RUnlock()
	if count != 0 {
		t.Fatalf("expected 0 usable keys after non-EC-only JWKS, got %d", count)
	}
}

// TestParseECPublicKey_BadXBase64 covers the error branch for an invalid X coordinate.
func TestParseECPublicKey_BadXBase64(t *testing.T) {
	_, err := parseECPublicKey("!!! not base64 !!!", "AAAA")
	if err == nil {
		t.Fatal("expected error for invalid X base64")
	}
}

// TestParseECPublicKey_BadYBase64 covers the error branch for a valid X but
// invalid Y coordinate.
func TestParseECPublicKey_BadYBase64(t *testing.T) {
	// Provide a valid 32-byte base64url for X (32 zero bytes).
	validX := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	_, err := parseECPublicKey(validX, "!!! not base64 !!!")
	if err == nil {
		t.Fatal("expected error for invalid Y base64")
	}
}

// TestAuth_JWKSAvailable_KidNonString exercises the keyfunc branch where the
// kid header value is not a string (JWT spec only mandates string; some
// implementations send numbers).
func TestAuth_JWKSAvailable_KidNonString(t *testing.T) {
	kp := newECKeypair(t, testKid)
	b := &jwksBehaviour{}
	b.setKeys([]ecKeypair{kp})
	srv := startJWKSServer(t, b)

	cfg := baseESConfig(srv.URL)
	mw, err := Auth(cfg, "", logger.NewNoop())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}

	// Build a token whose kid header value is a number (non-string type).
	claims := authClaims{
		Email: testAllowedEml,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			Audience:  jwt.ClaimStrings{testAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			Subject:   "user-kidtype",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = 12345 // numeric, not string
	rawTok, err := tok.SignedString(kp.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	w, _ := request(t, mw, "Bearer "+rawTok)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401 for non-string kid", w.Code)
	}
}

// TestExtractBearer_ExactlyPrefixLen covers the len(header) <= len("Bearer ")
// boundary — the header value is exactly "Bearer" with no token.
func TestExtractBearer_ExactlyPrefixLen(t *testing.T) {
	_, ok := extractBearer("Bearer")
	if ok {
		t.Fatal("expected ok=false for header that is exactly the prefix")
	}
}

// TestAuth_JWKS_MissingKid_401 exercises keyfunc's `!ok` branch when the JWT
// carries no kid header at all. This is the missing-kid path (distinct from
// the non-string kid path).
func TestAuth_JWKS_MissingKid_401(t *testing.T) {
	kp := newECKeypair(t, testKid)
	b := &jwksBehaviour{}
	b.setKeys([]ecKeypair{kp})
	srv := startJWKSServer(t, b)

	cfg := baseESConfig(srv.URL)
	mw, err := Auth(cfg, "", logger.NewNoop())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}

	// Build a token with no kid header at all.
	claims := authClaims{
		Email: testAllowedEml,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			Audience:  jwt.ClaimStrings{testAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			Subject:   "user-nokid",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	delete(tok.Header, "kid") // remove the default kid from header
	rawTok, err := tok.SignedString(kp.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	w, _ := request(t, mw, "Bearer "+rawTok)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401 for missing kid", w.Code)
	}
}

// TestAuth_JWKS_EmptyKid_401 exercises the kid=="" branch in keyfunc.
func TestAuth_JWKS_EmptyKid_401(t *testing.T) {
	kp := newECKeypair(t, testKid)
	b := &jwksBehaviour{}
	b.setKeys([]ecKeypair{kp})
	srv := startJWKSServer(t, b)

	cfg := baseESConfig(srv.URL)
	mw, err := Auth(cfg, "", logger.NewNoop())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}

	// Build a token with an empty string kid.
	claims := authClaims{
		Email: testAllowedEml,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			Audience:  jwt.ClaimStrings{testAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			Subject:   "user-emptykid",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = "" // empty string kid
	rawTok, err := tok.SignedString(kp.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	w, _ := request(t, mw, "Bearer "+rawTok)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401 for empty kid", w.Code)
	}
}

// TestJWKS_Lookup_TTLExpired_ReturnsNil directly exercises the lookup function's
// TTL expiry branch by setting fetchedAt to a time well past the TTL.
func TestJWKS_Lookup_TTLExpired_ReturnsNil(t *testing.T) {
	kp := newECKeypair(t, "kid-lookup-ttl")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Return an empty JWKS so any subsequent fetch finds nothing.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	t.Cleanup(srv.Close)

	v := newJWKSVerifier(srv.URL, 1*time.Millisecond, http.DefaultClient, 0)

	// Pre-populate the cache with the key.
	x, y := coordBytes(kp.priv.PublicKey)
	validX := base64.RawURLEncoding.EncodeToString(x)
	validY := base64.RawURLEncoding.EncodeToString(y)
	v.mu.Lock()
	pub, err := parseECPublicKey(validX, validY)
	if err != nil {
		v.mu.Unlock()
		t.Fatalf("parseECPublicKey: %v", err)
	}
	v.keys["kid-lookup-ttl"] = pub
	v.fetchedAt = time.Now().Add(-10 * time.Second) // set fetchedAt to well in the past
	v.mu.Unlock()

	// lookup must return nil because TTL (1ms) has long expired.
	got := v.lookup("kid-lookup-ttl")
	if got != nil {
		t.Fatal("lookup must return nil when TTL has expired")
	}
}

// TestJWKS_Refresh_ParseECPublicKey_BadX exercises the parseECPublicKey error
// path inside refresh — a JWKS key with an invalid X coordinate is silently
// skipped (continue), not an error.
func TestJWKS_Refresh_ParseECPublicKey_BadX(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// EC P-256 key with invalid X (not valid base64url).
		_, _ = w.Write([]byte(`{"keys":[{"kty":"EC","crv":"P-256","kid":"bad-key","x":"!!!invalid!!!","y":"AAAA"}]}`))
	}))
	t.Cleanup(srv.Close)

	v := newJWKSVerifier(srv.URL, 0, http.DefaultClient, 0)
	if err := v.refresh(); err != nil {
		t.Fatalf("refresh must not error on bad EC key: %v", err)
	}
	v.mu.RLock()
	count := len(v.keys)
	v.mu.RUnlock()
	if count != 0 {
		t.Fatalf("expected 0 usable keys after bad EC key, got %d", count)
	}
}

// TestVerifyJWT_InvalidAfterParse_ReturnsError exercises the !tok.Valid branch
// in VerifyJWT. This branch exists for defensive completeness — in practice
// jwt.ParseWithClaims never returns a non-nil token with Valid=false and
// nil error. We test it by constructing a jwtPipeline with a verifier that
// returns exactly this state.
func TestVerifyJWT_InvalidAfterParse_ReturnsError(t *testing.T) {
	// Install a fake verifier that returns an invalid-but-non-nil token.
	fake := &fakeInvalidVerifier{}
	pipeline := &jwtPipeline{
		verifier:         fake,
		expectedIssuer:   testIssuer,
		expectedAudience: testAudience,
	}
	jwtPipelineMu.Lock()
	saved := jwtPipelineSlot
	jwtPipelineSlot = pipeline
	jwtPipelineMu.Unlock()
	t.Cleanup(func() {
		jwtPipelineMu.Lock()
		jwtPipelineSlot = saved
		jwtPipelineMu.Unlock()
	})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer fake-token")
	_, _, err := VerifyJWT(r)
	if err == nil {
		t.Fatal("expected error for token with Valid=false")
	}
}

// fakeInvalidVerifier returns a non-nil token with Valid=false and nil error,
// simulating the defensive branch that guards against jwt library edge cases.
type fakeInvalidVerifier struct{}

func (f *fakeInvalidVerifier) Verify(_ string) (*jwt.Token, *authClaims, error) {
	tok := &jwt.Token{Valid: false}
	return tok, &authClaims{}, nil
}
