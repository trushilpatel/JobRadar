package middleware

import "net/http"

// BodyLimit wraps r.Body with http.MaxBytesReader so oversize request bodies
// fail fast with a 413-style error at the first read instead of buffering the
// whole payload. max must be positive; the caller owns the default.
func BodyLimit(max int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if max > 0 && r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, max)
			}
			next.ServeHTTP(w, r)
		})
	}
}
