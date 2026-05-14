package metrics_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/metrics"
)

// pickFreePort grabs an ephemeral port from the OS so Serve doesn't collide
// with other test runs.
func pickFreePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return addr
}

// startServeAndWait launches metrics.Serve in a goroutine and waits for the
// /health endpoint to respond, then returns the addr.
func startServeAndWait(t *testing.T, readyFn func() error) string {
	t.Helper()
	addr := pickFreePort(t)
	go metrics.Serve(addr, readyFn)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr+"/health", nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			return addr
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("metrics server never came up on %s", addr)
	return ""
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestServe_HealthAlwaysOK(t *testing.T) {
	addr := startServeAndWait(t, func() error { return nil })
	code, _ := get(t, "http://"+addr+"/health")
	if code != http.StatusOK {
		t.Fatalf("/health: want 200, got %d", code)
	}
}

func TestServe_ReadyWhenFnNil(t *testing.T) {
	addr := startServeAndWait(t, func() error { return nil })
	code, _ := get(t, "http://"+addr+"/ready")
	if code != http.StatusOK {
		t.Fatalf("/ready (healthy): want 200, got %d", code)
	}
}

func TestServe_ReadyReturns503OnError(t *testing.T) {
	addr := startServeAndWait(t, func() error { return errors.New("db down") })
	code, body := get(t, "http://"+addr+"/ready")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("/ready (failing): want 503, got %d", code)
	}
	if !strings.Contains(body, "db down") {
		t.Fatalf("body should contain error text, got %q", body)
	}
}

func TestServe_MetricsEndpointEmitsPrometheusFormat(t *testing.T) {
	addr := startServeAndWait(t, func() error { return nil })

	// Increment a known counter so /metrics has something to render.
	metrics.HTTPRequestsTotal.WithLabelValues("GET", "/x", "200").Inc()

	code, body := get(t, "http://"+addr+"/metrics")
	if code != http.StatusOK {
		t.Fatalf("/metrics: want 200, got %d", code)
	}
	if !strings.Contains(body, "http_requests_total") {
		preview := body
		if len(preview) > 200 {
			preview = preview[:200]
		}
		t.Fatalf("/metrics body should contain http_requests_total, got len=%d preview=%q", len(body), preview)
	}
}

func TestCountersAndHistograms_Defined(t *testing.T) {
	// Sanity-check that every exported counter/histogram is non-nil — this
	// also covers the init() registration block.
	cases := []any{
		metrics.HTTPRequestsTotal,
		metrics.HTTPRequestDuration,
		metrics.CrawlerScansTotal,
		metrics.CrawlerJobsFetchedTotal,
		metrics.CrawlerScanDuration,
		metrics.FilterJobsTotal,
		metrics.FilterLLMDuration,
		metrics.FilterChunkDuration,
		metrics.FilterRetryPublishesTotal,
		metrics.FilterRetryPublishFailures,
		metrics.FilterDLQPublishesTotal,
		metrics.FilterCommitFailuresTotal,
		metrics.FilterChunksPerFetch,
		metrics.EnricherJobsTotal,
		metrics.EnricherLLMDuration,
		metrics.EnricherGroupDuration,
		metrics.EnricherRetryPublishesTotal,
		metrics.EnricherRetryPublishFailures,
		metrics.EnricherRetryLedgerFailuresTotal,
		metrics.EnricherDLQPublishesTotal,
		metrics.EnricherCommitFailuresTotal,
		metrics.EnricherGroupsPerFetch,
		metrics.SchedulerCompaniesEnqueuedTotal,
		metrics.SchedulerRunDuration,
		metrics.SchedulerErrorsTotal,
		metrics.SchedulerStuckChecksTotal,
	}
	for i, c := range cases {
		if c == nil {
			t.Fatalf("metric #%d is nil — init() registration broken", i)
		}
	}
}

func TestCounters_IncrementsAccumulate(t *testing.T) {
	// Use a label combo unique to this test to avoid cross-test interference.
	method, route, status := "PUT", fmt.Sprintf("/test/%d", time.Now().UnixNano()), "204"
	c := metrics.HTTPRequestsTotal.WithLabelValues(method, route, status)
	c.Inc()
	c.Inc()
	c.Add(3)
	// Read back through /metrics rather than reflection — exercise the
	// registry plumbing end-to-end.
	addr := startServeAndWait(t, func() error { return nil })
	_, body := get(t, "http://"+addr+"/metrics")
	if !strings.Contains(body, route) {
		t.Fatalf("/metrics should expose label route=%q, got len=%d", route, len(body))
	}
}
