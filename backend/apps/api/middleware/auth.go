package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	keyfunc "github.com/MicahParks/keyfunc/v3"
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

// authClaims holds the Clerk JWT claim set. email and role are custom claims
// injected by the Clerk JWT template. A single "role" claim is used because
// Clerk publicMetadata stores one role string; multi-role would require a []string claim.
type authClaims struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

// VerifiedClaims is returned by VerifyJWT — the full verified identity from the token.
type VerifiedClaims struct {
	Subject string
	Email   string
	Role    string
}

// jwtPipeline holds the JWKS keyfunc + per-instance config used by both the
// Auth middleware and the public VerifyJWT helper. It is built once at boot
// via Auth() and read concurrently — never mutated after init.
type jwtPipeline struct {
	jwks     keyfunc.Keyfunc
	devToken string
	issuer   string
	// clockSkew is the JWT exp tolerance; set to jwtClockSkew at construction.
	clockSkew time.Duration
}

// pipelineMu / pipelineSlot stores the most recent pipeline initialised in
// this process. Auth() closes over its own copy; this slot is used by
// VerifyJWT, which has no closure to capture config in.
var (
	pipelineMu   sync.RWMutex
	pipelineSlot *jwtPipeline
)

func storePipeline(p *jwtPipeline) {
	pipelineMu.Lock()
	defer pipelineMu.Unlock()
	pipelineSlot = p
}

func loadPipeline() *jwtPipeline {
	pipelineMu.RLock()
	defer pipelineMu.RUnlock()
	return pipelineSlot
}

// jwtClockSkew is the tolerance applied to JWT exp validation.
// Configure in Clerk JWT template if a different value is needed.
const jwtClockSkew = 60 * time.Second

// Auth builds the authentication middleware. Fetches Clerk JWKS once at
// startup and refreshes automatically via MicahParks/keyfunc. Validates
// RS256 (or ES256) signature, iss, and exp with 60s clock skew.
func Auth(cfg config.AuthConfig, devToken string, log logger.Logger) (func(http.Handler) http.Handler, error) {
	jwks, err := keyfunc.NewDefaultCtx(context.Background(), []string{cfg.ClerkJWKSURL})
	if err != nil {
		return nil, fmt.Errorf("auth: fetch JWKS from %s: %w", cfg.ClerkJWKSURL, err)
	}
	p := &jwtPipeline{
		jwks:      jwks,
		devToken:  devToken,
		issuer:    cfg.ClerkIssuerURL,
		clockSkew: jwtClockSkew,
	}
	storePipeline(p)
	return authHandler(p, log), nil
}

// errPipelineNotInit is returned by VerifyJWT when no Auth middleware has been
// built in this process — a programmer error in wiring.
var errPipelineNotInit = errors.New("auth: VerifyJWT called before Auth was initialised")

// VerifyJWT runs the same JWT verification the Auth middleware does — bearer
// extraction, signature, iss, and exp (with skew) — and returns VerifiedClaims.
//
// The dev-token bypass mirrors Auth: a request carrying a matching X-Dev-Token
// header short-circuits to (Subject="dev-bypass", Role="admin") without
// touching the JWKS verifier.
//
// Auth must have been called once before VerifyJWT — typically at process boot.
func VerifyJWT(r *http.Request) (VerifiedClaims, error) {
	p := loadPipeline()
	if p == nil {
		return VerifiedClaims{}, errPipelineNotInit
	}
	if p.devToken != "" && r.Header.Get("X-Dev-Token") == p.devToken {
		return VerifiedClaims{Subject: "dev-bypass", Email: "", Role: "admin"}, nil
	}
	raw, ok := extractBearer(r.Header.Get("Authorization"))
	if !ok {
		return VerifiedClaims{}, errors.New("auth: missing or malformed bearer")
	}
	claims, err := verifyToken(p, raw)
	if err != nil {
		return VerifiedClaims{}, err
	}
	return VerifiedClaims{Subject: claims.Subject, Email: claims.Email, Role: claims.Role}, nil
}

func authHandler(p *jwtPipeline, log logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p.devToken != "" && r.Header.Get("X-Dev-Token") == p.devToken {
				ctx := context.WithValue(r.Context(), identityCtxKey{}, Identity{UserID: "dev-bypass"})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			raw, ok := extractBearer(r.Header.Get("Authorization"))
			if !ok {
				log.Warn("auth: missing or malformed bearer")
				handler.RespondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
				return
			}
			claims, err := verifyToken(p, raw)
			if err != nil {
				log.Warn("auth: token verification failed", logger.Err(err))
				handler.RespondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token")
				return
			}
			ctx := context.WithValue(r.Context(), identityCtxKey{}, Identity{
				UserID: claims.Subject,
				Email:  claims.Email,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func verifyToken(p *jwtPipeline, raw string) (*authClaims, error) {
	claims := &authClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, p.jwks.Keyfunc,
		jwt.WithValidMethods([]string{"RS256", "ES256"}),
		jwt.WithLeeway(p.clockSkew),
		jwt.WithIssuedAt(),
	)
	if err != nil {
		return nil, err
	}
	if claims.Issuer != p.issuer {
		return nil, fmt.Errorf("iss mismatch: got %q want %q", claims.Issuer, p.issuer)
	}
	if claims.ExpiresAt == nil {
		return nil, errors.New("token missing exp")
	}
	return claims, nil
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
