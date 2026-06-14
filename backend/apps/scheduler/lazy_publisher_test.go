package main

// lazy_publisher_test.go covers the lazyPublisher (AC-2 / RC-2).

import (
	"context"
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// stubSweeperPublisher is an in-process stub for SweeperPublisher used by
// lazyPublisher tests. It records calls and can inject errors.
type stubSweeperPublisher struct {
	publishErr     error
	publishCalls   int
	withHeadersErr error
	withHeadersMap []map[string]string
}

func (s *stubSweeperPublisher) Publish(_ context.Context, _ string, _ []byte) error {
	s.publishCalls++
	return s.publishErr
}

func (s *stubSweeperPublisher) PublishWithHeaders(_ context.Context, _ string, _ []byte, headers map[string]string) error {
	s.withHeadersMap = append(s.withHeadersMap, headers)
	return s.withHeadersErr
}

var _ scansvc.SweeperPublisher = (*stubSweeperPublisher)(nil)

// connectFn helpers ---------------------------------------------------------

// connectOK returns a stubSweeperPublisher on every call (simulates NATS up).
func connectOK(stub *stubSweeperPublisher) func(context.Context) (scansvc.SweeperPublisher, error) {
	return func(_ context.Context) (scansvc.SweeperPublisher, error) {
		return stub, nil
	}
}

// connectFail returns an error on every call (simulates NATS down).
func connectFail(err error) func(context.Context) (scansvc.SweeperPublisher, error) {
	return func(_ context.Context) (scansvc.SweeperPublisher, error) {
		return nil, err
	}
}

// TestLazyPublisher_PublishWithHeaders_NATSUp verifies that when NATS is
// available, PublishWithHeaders succeeds and the inner publisher is called.
func TestLazyPublisher_PublishWithHeaders_NATSUp(t *testing.T) {
	inner := &stubSweeperPublisher{}
	lp := newLazyPublisher(connectOK(inner), logger.NewNoop())

	err := lp.PublishWithHeaders(context.Background(), "jobs.raw.greenhouse", []byte(`{}`), map[string]string{"X-Retry-Count": "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inner.withHeadersMap) != 1 {
		t.Errorf("expected 1 inner PublishWithHeaders call, got %d", len(inner.withHeadersMap))
	}
}

// TestLazyPublisher_Publish_NATSUp verifies the basic Publish path when NATS is up.
func TestLazyPublisher_Publish_NATSUp(t *testing.T) {
	inner := &stubSweeperPublisher{}
	lp := newLazyPublisher(connectOK(inner), logger.NewNoop())

	err := lp.Publish(context.Background(), "jobs.raw.greenhouse", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.publishCalls != 1 {
		t.Errorf("expected 1 inner Publish call, got %d", inner.publishCalls)
	}
}

// TestLazyPublisher_NATSDown_ReturnsError verifies that when NATS is
// unavailable, PublishWithHeaders returns an error so the sweeper can
// re-queue and try again next cycle (AC-2).
func TestLazyPublisher_NATSDown_ReturnsError(t *testing.T) {
	lp := newLazyPublisher(connectFail(errors.New("dial tcp: refused")), logger.NewNoop())

	err := lp.PublishWithHeaders(context.Background(), "jobs.raw.greenhouse", []byte(`{}`), nil)
	if err == nil {
		t.Fatal("expected error when NATS is down, got nil")
	}
}

// TestLazyPublisher_NATSDown_IncrementsMetric verifies that a NATS connection
// failure increments SweeperNATSUnavailableTotal (AC-2, AC-7 metrics).
func TestLazyPublisher_NATSDown_IncrementsMetric(t *testing.T) {
	before := testutil.ToFloat64(metrics.SweeperNATSUnavailableTotal)

	lp := newLazyPublisher(connectFail(errors.New("nats down")), logger.NewNoop())
	_ = lp.PublishWithHeaders(context.Background(), "jobs.raw.greenhouse", []byte(`{}`), nil)

	after := testutil.ToFloat64(metrics.SweeperNATSUnavailableTotal)
	if after-before < 1 {
		t.Errorf("SweeperNATSUnavailableTotal delta = %.0f, want ≥1", after-before)
	}
}

// TestLazyPublisher_Reconnects verifies that after a NATS outage the lazy
// publisher re-connects on the next call (no cached nil publisher).
func TestLazyPublisher_Reconnects(t *testing.T) {
	callCount := 0
	inner := &stubSweeperPublisher{}
	connectFn := func(_ context.Context) (scansvc.SweeperPublisher, error) {
		callCount++
		if callCount < 3 {
			return nil, errors.New("transient error")
		}
		return inner, nil
	}
	lp := newLazyPublisher(connectFn, logger.NewNoop())

	// First two calls fail (NATS down).
	_ = lp.PublishWithHeaders(context.Background(), "subject", []byte(`{}`), nil)
	_ = lp.PublishWithHeaders(context.Background(), "subject", []byte(`{}`), nil)

	// Third call succeeds (NATS recovered).
	err := lp.PublishWithHeaders(context.Background(), "subject", []byte(`{}`), map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("expected success after reconnect, got: %v", err)
	}
	if len(inner.withHeadersMap) != 1 {
		t.Errorf("expected 1 successful publish after reconnect, got %d", len(inner.withHeadersMap))
	}
}

// TestLazyPublisher_CachesConnection verifies that a successful connect is
// cached: connectFn is only called once even if publish is called many times.
func TestLazyPublisher_CachesConnection(t *testing.T) {
	connectCount := 0
	inner := &stubSweeperPublisher{}
	connectFn := func(_ context.Context) (scansvc.SweeperPublisher, error) {
		connectCount++
		return inner, nil
	}
	lp := newLazyPublisher(connectFn, logger.NewNoop())

	for i := 0; i < 5; i++ {
		_ = lp.PublishWithHeaders(context.Background(), "subject", []byte(`{}`), nil)
	}

	if connectCount != 1 {
		t.Errorf("expected 1 connect call (cached), got %d", connectCount)
	}
}

// TestLazyPublisher_Publish_NATSDown verifies that Publish (the plain path)
// returns an error and increments SweeperNATSUnavailableTotal when NATS is down.
func TestLazyPublisher_Publish_NATSDown(t *testing.T) {
	before := testutil.ToFloat64(metrics.SweeperNATSUnavailableTotal)

	lp := newLazyPublisher(connectFail(errors.New("nats down")), logger.NewNoop())

	err := lp.Publish(context.Background(), "jobs.raw.greenhouse", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error when NATS is down, got nil")
	}

	after := testutil.ToFloat64(metrics.SweeperNATSUnavailableTotal)
	if after-before < 1 {
		t.Errorf("SweeperNATSUnavailableTotal delta = %.0f, want ≥1", after-before)
	}
}

// TestLazyPublisher_Publish_EvictsOnPublishError verifies that when the inner
// Publish call fails, the cached publisher is evicted so the next call re-dials.
func TestLazyPublisher_Publish_EvictsOnPublishError(t *testing.T) {
	connectCount := 0
	failOnce := true
	connectFn := func(_ context.Context) (scansvc.SweeperPublisher, error) {
		connectCount++
		if failOnce {
			return &stubSweeperPublisher{publishErr: errors.New("publish boom")}, nil
		}
		return &stubSweeperPublisher{}, nil
	}
	lp := newLazyPublisher(connectFn, logger.NewNoop())

	// First call: connects, Publish fails, publisher evicted.
	err := lp.Publish(context.Background(), "subject", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error on first call (inner publish fails), got nil")
	}

	// Second call: re-connects, now succeeds.
	failOnce = false
	err = lp.Publish(context.Background(), "subject", []byte(`{}`))
	if err != nil {
		t.Fatalf("expected success after reconnect, got: %v", err)
	}
	if connectCount != 2 {
		t.Errorf("expected 2 connect calls (initial + reconnect), got %d", connectCount)
	}
}

// TestLazyPublisher_EvictsOnPublishError verifies that when the inner publisher
// itself returns an error (e.g. JetStream stream not found), the cached
// publisher is evicted so the next call re-dials rather than retrying the
// same broken connection.
func TestLazyPublisher_EvictsOnPublishError(t *testing.T) {
	connectCount := 0
	failOnce := true
	connectFn := func(_ context.Context) (scansvc.SweeperPublisher, error) {
		connectCount++
		if failOnce {
			// Return a publisher that always fails.
			return &stubSweeperPublisher{withHeadersErr: errors.New("stream not found")}, nil
		}
		return &stubSweeperPublisher{}, nil
	}
	lp := newLazyPublisher(connectFn, logger.NewNoop())

	// First call: connects, gets a failing publisher, publish fails, publisher evicted.
	err := lp.PublishWithHeaders(context.Background(), "subject", []byte(`{}`), nil)
	if err == nil {
		t.Fatal("expected error on first call (publisher fails), got nil")
	}

	// Second call: re-connects, now succeeds.
	failOnce = false
	err = lp.PublishWithHeaders(context.Background(), "subject", []byte(`{}`), nil)
	if err != nil {
		t.Fatalf("expected success after reconnect, got: %v", err)
	}

	if connectCount != 2 {
		t.Errorf("expected 2 connect calls (initial + reconnect after eviction), got %d", connectCount)
	}
}
