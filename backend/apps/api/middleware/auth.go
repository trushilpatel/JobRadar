package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
)

// identityCtxKey is an unexported type so only this package can read/write
// the identity on a request context — prevents forgery via raw string keys.
type identityCtxKey struct{}

// Identity is the authenticated caller surfaced on the request context.
type Identity struct {
	UserID string
	Email  string
}

// FromContext returns the Identity stored on ctx by the Auth middleware.
// ok is false when no identity is attached (middleware skipped or not run).
func FromContext(ctx context.Context) (Identity, bool) {
	v, ok := ctx.Value(identityCtxKey{}).(Identity)
	return v, ok
}

// authClaims is the subset of Supabase JWT claims the middleware verifies.
// Extra claims (role, app_metadata, etc.) are ignored.
type authClaims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// tokenVerifier abstracts the signature-verification strategy so the middleware
// body is algorithm-agnostic. At boot exactly one verifier is selected by
// config — never both, per spec §6.
type tokenVerifier interface {
	Verify(tokenString string) (*jwt.Token, *authClaims, error)
}

// errJWKSUnavailable is a sentinel returned by the JWKS verifier when the
// remote JWKS endpoint cannot be reached. Maps to HTTP 503 Retry-After.
var errJWKSUnavailable = errors.New("auth: jwks unavailable")

// Auth builds the authentication middleware. Selects a single verifier at
// boot (ES256 via JWKS OR HS256 via shared secret — never both) and verifies
// signature, iss, aud, exp (with skew), then enforces the email allowlist via
// constant-time compare.
func Auth(cfg config.AuthConfig, log logger.Logger) (func(http.Handler) http.Handler, error) {
	skew := time.Duration(cfg.ClockSkewSeconds) * time.Second
	verifier, err := newVerifier(cfg, skew)
	if err != nil {
		return nil, fmt.Errorf("auth middleware: init verifier: %w", err)
	}
	return authHandler(cfg, verifier, log), nil
}

func authHandler(cfg config.AuthConfig, verifier tokenVerifier, log logger.Logger) func(http.Handler) http.Handler {
	expectedIssuer := strings.TrimRight(cfg.SupabaseProjectURL, "/") + "/auth/v1"
	const expectedAudience = "authenticated"
	skew := time.Duration(cfg.ClockSkewSeconds) * time.Second

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := extractBearer(r.Header.Get("Authorization"))
			if !ok {
				log.Warn("auth: missing or malformed bearer")
				handler.RespondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
				return
			}

			tok, claims, err := verifier.Verify(raw)
			if err != nil {
				if errors.Is(err, errJWKSUnavailable) {
					log.Error("auth: jwks unavailable", err)
					w.Header().Set("Retry-After", "30")
					handler.RespondError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "authentication service unavailable")
					return
				}
				log.Warn("auth: token verification failed", logger.Err(err))
				handler.RespondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token")
				return
			}
			if !tok.Valid {
				log.Warn("auth: token invalid after parse")
				handler.RespondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token")
				return
			}

			if err := validateStandardClaims(claims, expectedIssuer, expectedAudience, skew); err != nil {
				log.Warn("auth: claims rejected", logger.Err(err))
				handler.RespondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token")
				return
			}

			if !emailMatches(claims.Email, cfg.AllowedEmail) {
				log.Warn("auth: email not allowed", logger.Str(logger.FieldEmailDomain, domainOf(claims.Email)))
				handler.RespondError(w, http.StatusForbidden, "FORBIDDEN", "access denied")
				return
			}

			ident := Identity{UserID: claims.Subject, Email: claims.Email}
			ctx := context.WithValue(r.Context(), identityCtxKey{}, ident)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearer(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) {
		return "", false
	}
	if !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	tok := strings.TrimSpace(header[len(prefix):])
	if tok == "" {
		return "", false
	}
	return tok, true
}

func validateStandardClaims(c *authClaims, issuer, audience string, skew time.Duration) error {
	if c.Issuer != issuer {
		return fmt.Errorf("iss mismatch: got %q want %q", c.Issuer, issuer)
	}
	if !audienceContains(c.Audience, audience) {
		return fmt.Errorf("aud mismatch: got %v want %q", c.Audience, audience)
	}
	if c.ExpiresAt == nil {
		return errors.New("exp missing")
	}
	if time.Now().After(c.ExpiresAt.Add(skew)) {
		return errors.New("token expired")
	}
	return nil
}

func audienceContains(aud jwt.ClaimStrings, want string) bool {
	return slices.Contains(aud, want)
}

// emailMatches performs a constant-time compare of two emails after lowering.
// Returns false if either side is empty. Length mismatch short-circuits to a
// dummy compare so the timing profile stays flat regardless of input length.
func emailMatches(got, allowed string) bool {
	if got == "" || allowed == "" {
		return false
	}
	g := []byte(strings.ToLower(got))
	a := []byte(strings.ToLower(allowed))
	if len(g) != len(a) {
		_ = subtle.ConstantTimeCompare(a, a)
		return false
	}
	return subtle.ConstantTimeCompare(g, a) == 1
}

func domainOf(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return email[at+1:]
}

// newVerifier picks ES256-via-JWKS or HS256-via-secret based on cfg. The
// caller has already validated that exactly one of the two is set
// (config.AuthConfig.Validate). skew is applied as jwt.WithLeeway so the
// library's own exp check matches our explicit one.
func newVerifier(cfg config.AuthConfig, skew time.Duration) (tokenVerifier, error) {
	if cfg.SupabaseJWTSecret != "" {
		return newHMACVerifier(cfg.SupabaseJWTSecret, skew), nil
	}
	if cfg.SupabaseJWKSURL == "" {
		return nil, errors.New("neither SUPABASE_JWT_SECRET nor SUPABASE_JWKS_URL set")
	}
	ttl := time.Duration(cfg.JWKSCacheTTLSeconds) * time.Second
	return newJWKSVerifier(cfg.SupabaseJWKSURL, ttl, http.DefaultClient, skew), nil
}

// hmacVerifier verifies HS256 tokens using a shared secret. Rejects any other
// algorithm via jwt.WithValidMethods.
type hmacVerifier struct {
	secret []byte
	skew   time.Duration
}

func newHMACVerifier(secret string, skew time.Duration) *hmacVerifier {
	return &hmacVerifier{secret: []byte(secret), skew: skew}
}

func (v *hmacVerifier) Verify(tokenString string) (*jwt.Token, *authClaims, error) {
	claims := &authClaims{}
	tok, err := jwt.ParseWithClaims(tokenString, claims, func(_ *jwt.Token) (any, error) {
		return v.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithLeeway(v.skew))
	if err != nil {
		return nil, nil, err
	}
	return tok, claims, nil
}

// jwksVerifier verifies ES256 tokens using keys fetched from a JWKS endpoint.
// Rejects any other algorithm via jwt.WithValidMethods. Caches keys in-memory
// with a TTL; on unknown kid refetches once before rejecting.
type jwksVerifier struct {
	url    string
	ttl    time.Duration
	skew   time.Duration
	client *http.Client

	mu        sync.RWMutex
	keys      map[string]*ecdsa.PublicKey
	fetchedAt time.Time
}

func newJWKSVerifier(url string, ttl time.Duration, client *http.Client, skew time.Duration) *jwksVerifier {
	return &jwksVerifier{url: url, ttl: ttl, skew: skew, client: client, keys: map[string]*ecdsa.PublicKey{}}
}

func (v *jwksVerifier) Verify(tokenString string) (*jwt.Token, *authClaims, error) {
	claims := &authClaims{}
	tok, err := jwt.ParseWithClaims(tokenString, claims, v.keyfunc, jwt.WithValidMethods([]string{"ES256"}), jwt.WithLeeway(v.skew))
	if err != nil {
		return nil, nil, err
	}
	return tok, claims, nil
}

// keyfunc resolves the signing key by kid. Cache misses trigger a single
// refetch; if the kid is still unknown after the refetch, returns an error.
// Network failures on fetch surface as errJWKSUnavailable.
func (v *jwksVerifier) keyfunc(tok *jwt.Token) (any, error) {
	kidRaw, ok := tok.Header["kid"]
	if !ok {
		return nil, errors.New("jwks: token missing kid")
	}
	kid, ok := kidRaw.(string)
	if !ok || kid == "" {
		return nil, errors.New("jwks: token kid not a string")
	}

	if k := v.lookup(kid); k != nil {
		return k, nil
	}
	if err := v.refresh(); err != nil {
		return nil, err
	}
	if k := v.lookup(kid); k != nil {
		return k, nil
	}
	return nil, fmt.Errorf("jwks: unknown kid %q after refresh", kid)
}

func (v *jwksVerifier) lookup(kid string) *ecdsa.PublicKey {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.ttl > 0 && !v.fetchedAt.IsZero() && time.Since(v.fetchedAt) > v.ttl {
		return nil
	}
	return v.keys[kid]
}

// refresh fetches the JWKS document and replaces the in-memory cache. Network
// or non-2xx responses return errJWKSUnavailable so the middleware can fail
// closed with 503.
func (v *jwksVerifier) refresh() error {
	req, err := http.NewRequest(http.MethodGet, v.url, nil)
	if err != nil {
		return fmt.Errorf("jwks request: %w", err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return errJWKSUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errJWKSUnavailable
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errJWKSUnavailable
	}
	var doc jwksDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("jwks decode: %w", err)
	}

	newKeys := make(map[string]*ecdsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" || k.Kid == "" {
			continue
		}
		pub, err := parseECPublicKey(k.X, k.Y)
		if err != nil {
			continue
		}
		newKeys[k.Kid] = pub
	}

	v.mu.Lock()
	v.keys = newKeys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

type jwksDoc struct {
	Keys []jwksKey `json:"keys"`
}

type jwksKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	Alg string `json:"alg"`
}

func parseECPublicKey(xB64, yB64 string) (*ecdsa.PublicKey, error) {
	xBytes, err := base64.RawURLEncoding.DecodeString(xB64)
	if err != nil {
		return nil, fmt.Errorf("jwks: decode x: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(yB64)
	if err != nil {
		return nil, fmt.Errorf("jwks: decode y: %w", err)
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}, nil
}
