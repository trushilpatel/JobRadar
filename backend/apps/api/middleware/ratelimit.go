// Package middleware — ratelimit.go
//
// RateLimit throttles requests per (key, scope) using a Limiter. The scope is
// derived from the HTTP method via ratelimit.ScopeFromMethod; the key is the
// caller's user ID when an Identity is on the context, falling back to
// r.RemoteAddr when the request is unauthenticated (so anonymous probes still
// get throttled — they just share a bucket per source IP).
//
// Failure policy:
//   - Limiter denies → 429 + Retry-After header (seconds, rounded up).
//   - Limiter returns an error (Redis outage) → fail open. The middleware
//     logs and lets the request through. This matches the spec: rate-limiting
//     is a best-effort defence, not a correctness boundary.
package middleware

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/ratelimit"
)

// RateLimit returns middleware that consults limiter once per request. nil
// limiter is treated as "no limiting" — useful for tests and local dev.
func RateLimit(limiter ratelimit.Limiter, log logger.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = logger.NewNoop()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil {
				next.ServeHTTP(w, r)
				return
			}

			key := identity.UserIDFrom(r.Context())
			if key == "" {
				key = r.RemoteAddr
			}
			scope := ratelimit.ScopeFromMethod(r.Method)

			allowed, retryAfter, err := limiter.Allow(r.Context(), key, scope)
			if err != nil {
				// Fail-open: never block traffic on a Redis outage.
				log.Warn("ratelimit: limiter error, failing open",
					logger.Err(err),
					logger.Str(logger.FieldMethod, r.Method),
					logger.Str(logger.FieldPath, r.URL.Path),
				)
				next.ServeHTTP(w, r)
				return
			}
			if !allowed {
				w.Header().Set("Retry-After", retryAfterSeconds(retryAfter))
				handler.RespondError(w, http.StatusTooManyRequests, "RATE_LIMITED", "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// retryAfterSeconds renders a Retry-After header value in whole seconds,
// rounded up so a 1.2s wait becomes "2". Always reports at least "1" when
// the limiter signalled a positive wait — clients should never see "0".
func retryAfterSeconds(d time.Duration) string {
	if d <= 0 {
		return "1"
	}
	secs := int64(math.Ceil(d.Seconds()))
	if secs < 1 {
		secs = 1
	}
	return strconv.FormatInt(secs, 10)
}
