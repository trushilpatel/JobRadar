package middleware

import (
	"net/http"
	"time"

	"github.com/trushilpatel/jobradar/internal/logger"
)

// Logger returns an HTTP middleware that logs each request's method, path,
// status code, and duration. Accepts a Logger so the service name and format
// are inherited from the process-wide logger.
func Logger(log logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)
			log.Info("request",
				logger.Str(logger.FieldMethod, r.Method),
				logger.Str(logger.FieldPath, r.URL.Path),
				logger.Int(logger.FieldStatus, ww.status),
				logger.Dur(logger.FieldDuration, time.Since(start).Round(time.Millisecond)),
			)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture the written status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
