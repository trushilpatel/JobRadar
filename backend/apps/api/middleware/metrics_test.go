package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/trushilpatel/jobradar/common/metrics"
)

// TestMetrics_IncrementsCounterWithChiRoutePattern asserts that the middleware
// labels the counter with the chi route pattern (not the raw URL) when the
// request flows through chi. The label cardinality contract depends on this —
// using r.URL.Path would explode the metric series with one entry per ID.
func TestMetrics_IncrementsCounterWithChiRoutePattern(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Metrics())
	r.Get("/jobs/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	const method = http.MethodGet
	const route = "/jobs/{id}"
	const status = "200"

	before := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(method, route, status))

	req := httptest.NewRequest(method, "/jobs/abc-123", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rr.Code)
	}
	after := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(method, route, status))
	if delta := after - before; delta != 1 {
		t.Fatalf("counter delta=%v want 1 (label %s %s %s)", delta, method, route, status)
	}
}

// TestMetrics_RecordsStatusFromHandler asserts the wrapped writer's status is
// the label, not the default — so 500s and 404s appear under the right bucket.
func TestMetrics_RecordsStatusFromHandler(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Metrics())
	r.Post("/widgets", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	const method = http.MethodPost
	const route = "/widgets"
	const status = "500"

	before := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(method, route, status))

	req := httptest.NewRequest(method, "/widgets", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	after := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(method, route, status))
	if delta := after - before; delta != 1 {
		t.Fatalf("500 counter delta=%v want 1", delta)
	}
}

// TestMetrics_DefaultStatusWhenHandlerSilent asserts the wrapped writer
// defaults to 200 when the handler never calls WriteHeader.
func TestMetrics_DefaultStatusWhenHandlerSilent(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Metrics())
	r.Get("/silent", func(_ http.ResponseWriter, _ *http.Request) {
		// no WriteHeader, no Write
	})

	before := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(http.MethodGet, "/silent", "200"))
	req := httptest.NewRequest(http.MethodGet, "/silent", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	after := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(http.MethodGet, "/silent", "200"))
	if delta := after - before; delta != 1 {
		t.Fatalf("default-status counter delta=%v want 1", delta)
	}
}

// TestMetrics_FallbackToPathWhenNoChiContext asserts that when the middleware
// runs outside a chi router (no RouteContext), it falls back to r.URL.Path so
// the label is at least observable instead of being empty.
func TestMetrics_FallbackToPathWhenNoChiContext(t *testing.T) {
	// Build a stand-alone middleware-wrapped handler — no chi router involved.
	mw := Metrics()
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	// Even without chi.RouteContext, the chi library installs a default empty
	// context on calls through chi.RouteContext(); the middleware then checks
	// for empty pattern and falls back to URL.Path. Drive it with the raw path.
	const rawPath = "/bare/path"
	before := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(http.MethodGet, rawPath, "202"))

	req := httptest.NewRequest(http.MethodGet, rawPath, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	after := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(http.MethodGet, rawPath, "202"))
	if delta := after - before; delta != 1 {
		t.Fatalf("fallback-path counter delta=%v want 1 (route=%q)", delta, rawPath)
	}
}

// TestMetrics_RecordsDurationHistogram asserts the duration histogram is also
// incremented; sample count must increase by 1 per request.
func TestMetrics_RecordsDurationHistogram(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Metrics())
	r.Get("/timed", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// CollectAndCount counts samples across all metric families; we filter by
	// label by inspecting the histogram's sample count delta directly.
	hist := metrics.HTTPRequestDuration.WithLabelValues(http.MethodGet, "/timed")

	// Read current count via dto.Metric.
	beforeCount := histogramSampleCount(t, hist)

	req := httptest.NewRequest(http.MethodGet, "/timed", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	afterCount := histogramSampleCount(t, hist)
	if delta := afterCount - beforeCount; delta != 1 {
		t.Fatalf("histogram sample delta=%d want 1", delta)
	}
}

// histogramSampleCount reads the current sample count from a prom histogram.
// Uses the prometheus Metric interface so the helper accepts an Observer
// pulled from a HistogramVec.
func histogramSampleCount(t *testing.T, h prometheus.Observer) uint64 {
	t.Helper()
	m, ok := h.(prometheus.Metric)
	if !ok {
		t.Fatalf("histogram is not prometheus.Metric")
	}
	var pb dto.Metric
	if err := m.Write(&pb); err != nil {
		t.Fatalf("hist.Write: %v", err)
	}
	if pb.Histogram == nil {
		return 0
	}
	return pb.Histogram.GetSampleCount()
}
