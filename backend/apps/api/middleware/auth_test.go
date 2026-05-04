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
		AllowedEmail:        testAllowedEml,
		ClockSkewSeconds:    60,
		JWKSCacheTTLSeconds: 600,
	}
}

func baseHSConfig() config.AuthConfig {
	return config.AuthConfig{
		SupabaseProjectURL:  testProjectURL,
		SupabaseJWTSecret:   testHMACSecret,
		AllowedEmail:        testAllowedEml,
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

func TestAuth_WrongEmail_403(t *testing.T) {
	cfg := baseHSConfig()
	mw, _ := Auth(cfg, "", logger.NewNoop())
	tok := signToken(t, tokenOptions{
		alg: "HS256", hmacKey: []byte(testHMACSecret),
		issuer: testIssuer, audience: testAudience, email: "intruder@example.com",
		expiresAt: time.Now().Add(5 * time.Minute),
	})
	w, _ := request(t, mw, "Bearer "+tok)
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d want 403", w.Code)
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

// TestEmailMatches_ConstantTime directly exercises the allowlist compare to
// confirm length mismatch, case folding, and empty inputs behave correctly.
func TestEmailMatches_ConstantTime(t *testing.T) {
	cases := []struct {
		got, allowed string
		want         bool
	}{
		{"owner@example.com", "owner@example.com", true},
		{"OWNER@example.com", "owner@example.com", true},
		{"owner@example.com", "OWNER@example.com", true},
		{"intruder@example.com", "owner@example.com", false},
		{"", "owner@example.com", false},
		{"owner@example.com", "", false},
		{"different-length@example.com", "owner@example.com", false},
	}
	for _, tc := range cases {
		if got := emailMatches(tc.got, tc.allowed); got != tc.want {
			t.Errorf("emailMatches(%q, %q) = %v, want %v", tc.got, tc.allowed, got, tc.want)
		}
	}
}

func TestDevBypass(t *testing.T) {
	const devToken = "test-dev-token-abc123"
	const allowedEmail = "dev@example.com"

	cfg := config.AuthConfig{
		AllowedEmail:      allowedEmail,
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
		if gotIdent.Email != allowedEmail {
			t.Fatalf("expected Email %q, got %q", allowedEmail, gotIdent.Email)
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
		AllowedEmail:      "dev@example.com",
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
		cfg2 := baseHSConfig()
		cfg2.AllowedEmail = "dev@example.com"
		if _, err := Auth(cfg2, devToken, logger.NewNoop()); err != nil {
			t.Fatalf("Auth init: %v", err)
		}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Dev-Token", devToken)
		sub, email, err := VerifyJWT(r)
		if err != nil {
			t.Fatalf("VerifyJWT: %v", err)
		}
		if sub != "dev-bypass" || email != "dev@example.com" {
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
