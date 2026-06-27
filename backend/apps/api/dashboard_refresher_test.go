package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
	dashboardsvc "github.com/trushilpatel/jobradar/service/dashboard/services"
)

// fakeStatsRefresher records each refresh call and signals on a channel so
// tests can synchronise without sleeps. Optional error funcs let a test drive
// the error-handling branches.
type fakeStatsRefresher struct {
	mu          sync.Mutex
	pipelineN   int
	heatmapN    int
	pipelineErr func(n int) error
	heatmapErr  func(n int) error
	pipelineCh  chan struct{}
	heatmapCh   chan struct{}
}

func newFakeRefresher() *fakeStatsRefresher {
	return &fakeStatsRefresher{
		pipelineCh: make(chan struct{}, 64),
		heatmapCh:  make(chan struct{}, 64),
	}
}

func (f *fakeStatsRefresher) RefreshPipelineStats(_ context.Context) error {
	f.mu.Lock()
	f.pipelineN++
	n := f.pipelineN
	errFn := f.pipelineErr
	f.mu.Unlock()
	select {
	case f.pipelineCh <- struct{}{}:
	default:
	}
	if errFn != nil {
		return errFn(n)
	}
	return nil
}

func (f *fakeStatsRefresher) RefreshHeatmap(_ context.Context) error {
	f.mu.Lock()
	f.heatmapN++
	n := f.heatmapN
	errFn := f.heatmapErr
	f.mu.Unlock()
	select {
	case f.heatmapCh <- struct{}{}:
	default:
	}
	if errFn != nil {
		return errFn(n)
	}
	return nil
}

func (f *fakeStatsRefresher) pipelineCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pipelineN
}

func (f *fakeStatsRefresher) heatmapCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.heatmapN
}

var _ dashboardsvc.StatsRefresher = (*fakeStatsRefresher)(nil)

// withIntervals temporarily overrides the package refresh cadences and
// restores them on cleanup. Keeps tests from racing on the globals.
func withIntervals(t *testing.T, pipeline, heatmap time.Duration) {
	t.Helper()
	origP, origH := dashboardPipelineRefreshInterval, dashboardHeatmapRefreshInterval
	dashboardPipelineRefreshInterval = pipeline
	dashboardHeatmapRefreshInterval = heatmap
	t.Cleanup(func() {
		dashboardPipelineRefreshInterval = origP
		dashboardHeatmapRefreshInterval = origH
	})
}

// TestRunRefresher_InitialRefreshThenStopsOnCancel verifies both views are
// refreshed once immediately on startup and that cancelling the context makes
// the loop return promptly via the ctx.Done() branch.
func TestRunRefresher_InitialRefreshThenStopsOnCancel(t *testing.T) {
	// Long intervals so only the initial (pre-loop) refresh runs before cancel.
	withIntervals(t, time.Hour, time.Hour)

	f := newFakeRefresher()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		runRefresher(ctx, logger.NewNoop(), f)
		close(done)
	}()

	// Wait for the initial pipeline + heatmap refresh to land.
	select {
	case <-f.pipelineCh:
	case <-time.After(2 * time.Second):
		t.Fatal("initial pipeline refresh did not run")
	}
	select {
	case <-f.heatmapCh:
	case <-time.After(2 * time.Second):
		t.Fatal("initial heatmap refresh did not run")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runRefresher did not return after ctx cancel")
	}

	if got := f.pipelineCount(); got != 1 {
		t.Fatalf("pipeline refreshed %d times, want exactly 1 (initial only)", got)
	}
	if got := f.heatmapCount(); got != 1 {
		t.Fatalf("heatmap refreshed %d times, want exactly 1 (initial only)", got)
	}
}

// TestRunRefresher_TickerFiresBothViews drives the in-loop ticker branches:
// with sub-millisecond intervals both tickers fire repeatedly until cancel.
func TestRunRefresher_TickerFiresBothViews(t *testing.T) {
	withIntervals(t, time.Millisecond, time.Millisecond)

	f := newFakeRefresher()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		runRefresher(ctx, logger.NewNoop(), f)
		close(done)
	}()

	// Consume the initial refreshes plus at least one ticker-driven refresh of
	// each view (count > 1 proves the select tick branch executed).
	waitForCount := func(name string, get func() int) {
		deadline := time.After(3 * time.Second)
		for {
			if get() > 1 {
				return
			}
			select {
			case <-deadline:
				t.Fatalf("%s did not tick past the initial refresh; count=%d", name, get())
			case <-time.After(time.Millisecond):
			}
		}
	}
	waitForCount("pipeline", f.pipelineCount)
	waitForCount("heatmap", f.heatmapCount)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runRefresher did not return after ctx cancel")
	}
}

// TestRunRefresher_InitialErrorsAreLoggedNotFatal drives the error branches of
// the immediate startup refresh — both refreshers return errors, but the loop
// must still proceed (and then stop cleanly on cancel).
func TestRunRefresher_InitialErrorsAreLoggedNotFatal(t *testing.T) {
	withIntervals(t, time.Hour, time.Hour)

	f := newFakeRefresher()
	f.pipelineErr = func(int) error { return errors.New("pipeline boom") }
	f.heatmapErr = func(int) error { return errors.New("heatmap boom") }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runRefresher(ctx, logger.NewNoop(), f)
		close(done)
	}()

	select {
	case <-f.heatmapCh:
	case <-time.After(2 * time.Second):
		t.Fatal("initial refresh did not run under error conditions")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runRefresher did not return after ctx cancel (error path)")
	}

	if f.pipelineCount() != 1 || f.heatmapCount() != 1 {
		t.Fatalf("expected one refresh each despite errors, got pipeline=%d heatmap=%d",
			f.pipelineCount(), f.heatmapCount())
	}
}

// TestRunRefresher_TickerErrorsKeepLoopAlive drives the in-loop error branches:
// every refresh returns an error, yet the tickers keep firing (the loop never
// exits on error) until the context is cancelled.
func TestRunRefresher_TickerErrorsKeepLoopAlive(t *testing.T) {
	withIntervals(t, time.Millisecond, time.Millisecond)

	f := newFakeRefresher()
	f.pipelineErr = func(int) error { return errors.New("pipeline boom") }
	f.heatmapErr = func(int) error { return errors.New("heatmap boom") }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		runRefresher(ctx, logger.NewNoop(), f)
		close(done)
	}()

	deadline := time.After(3 * time.Second)
	for f.pipelineCount() <= 1 || f.heatmapCount() <= 1 {
		select {
		case <-deadline:
			t.Fatalf("tickers stopped on error: pipeline=%d heatmap=%d",
				f.pipelineCount(), f.heatmapCount())
		case <-time.After(time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runRefresher did not return after ctx cancel (ticker error path)")
	}
}
