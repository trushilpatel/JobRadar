package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/platform/metrics"
)

// Metrics returns a middleware that records Prometheus HTTP metrics after each request.
// It uses chi's route pattern (e.g. "/jobs/{id}") as the label — not the raw path —
// to keep label cardinality bounded.
func Metrics() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)

			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = r.URL.Path
			}
			elapsed := time.Since(start)
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, route, strconv.Itoa(ww.status)).Inc()
			metrics.HTTPRequestDuration.WithLabelValues(r.Method, route).Observe(elapsed.Seconds())
		})
	}
}
