package middleware

import "net/http"

// SecurityHeaders sets defence-in-depth security response headers at the API
// layer. HSTS and CSP are handled by Caddy — setting them here would either
// duplicate or conflict with the edge, so they are intentionally omitted.
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=(), interest-cohort=()")
			next.ServeHTTP(w, r)
		})
	}
}
