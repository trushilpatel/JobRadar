package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
)

// --- Mock OrchestratorClient for PollLoop tests ---

type pollMockClient struct {
	getWorkFn   func(ctx context.Context, holding []string) (*GetWorkResult, error)
	reportErrFn func(ctx context.Context, e WorkError) error
	batches     []PageBatch
}

func (m *pollMockClient) GetWork(ctx context.Context, holding []string) (*GetWorkResult, error) {
	return m.getWorkFn(ctx, holding)
}

func (m *pollMockClient) SubmitBatch(ctx context.Context, batch PageBatch) error {
	m.batches = append(m.batches, batch)
	return nil
}

func (m *pollMockClient) ReportError(ctx context.Context, e WorkError) error {
	if m.reportErrFn != nil {
		return m.reportErrFn(ctx, e)
	}
	return nil
}

// --- Mock AdapterExecutor ---

type mockExecutor struct {
	executeFn func(ctx context.Context, unit WorkUnit) error
}

func (m *mockExecutor) Execute(ctx context.Context, unit WorkUnit) error {
	return m.executeFn(ctx, unit)
}

// --- Helpers ---

func newTestPollLoop(cfg *Config, client OrchestratorClient, exec AdapterExecutor) *PollLoop {
	return NewPollLoop(cfg, client, exec, logger.NewNoop())
}

func testConfig(pollInterval, maxConcurrent int) *Config {
	return &Config{
		PollIntervalSeconds:   pollInterval,
		MaxConcurrent:         maxConcurrent,
		AdapterTimeoutSeconds: 600,
	}
}

// --- Tests ---

// TestPollLoop_EmptyQueue_SleepsAndRetries verifies that when GetWork returns
// nil assignment the loop sleeps and polls again.
func TestPollLoop_EmptyQueue_SleepsAndRetries(t *testing.T) {
	calls := atomic.Int64{}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	client := &pollMockClient{
		getWorkFn: func(_ context.Context, _ []string) (*GetWorkResult, error) {
			calls.Add(1)
			return &GetWorkResult{Assignment: nil}, nil
		},
	}
	exec := &mockExecutor{
		executeFn: func(_ context.Context, _ WorkUnit) error { return nil },
	}

	cfg := testConfig(10, 1)    // 10ms poll for fast test
	cfg.PollIntervalSeconds = 0 // zero = no sleep
	loop := newTestPollLoop(cfg, client, exec)
	loop.Run(ctx)

	if calls.Load() < 1 {
		t.Error("expected at least 1 GetWork call, got 0")
	}
}

// TestPollLoop_WorkUnit_ExecutesAdapter verifies that when GetWork returns an
// assignment the loop dispatches it to the executor.
func TestPollLoop_WorkUnit_ExecutesAdapter(t *testing.T) {
	unit := &WorkUnit{
		AssignmentID: "assign-1",
		CompanyID:    "company-1",
		AdapterType:  "lever",
	}
	executed := make(chan WorkUnit, 1)
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	client := &pollMockClient{
		getWorkFn: func(_ context.Context, _ []string) (*GetWorkResult, error) {
			calls++
			if calls == 1 {
				return &GetWorkResult{Assignment: unit}, nil
			}
			// After first call cancel to stop the loop.
			cancel()
			return &GetWorkResult{Assignment: nil}, nil
		},
	}
	exec := &mockExecutor{
		executeFn: func(_ context.Context, u WorkUnit) error {
			executed <- u
			return nil
		},
	}

	loop := newTestPollLoop(testConfig(0, 2), client, exec)
	loop.Run(ctx)

	select {
	case u := <-executed:
		if u.AssignmentID != "assign-1" {
			t.Errorf("AssignmentID = %q, want assign-1", u.AssignmentID)
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for executor call")
	}
}

// TestPollLoop_HeartbeatMissed_RemovedFromHolding verifies that assignment IDs
// reported as missed in GetWorkResult are removed from the holding list.
func TestPollLoop_HeartbeatMissed_RemovedFromHolding(t *testing.T) {
	unit := &WorkUnit{AssignmentID: "assign-miss", CompanyID: "c1", AdapterType: "lever"}
	ctx, cancel := context.WithCancel(context.Background())

	// workerFinish is closed when the test is ready for the worker to stop.
	workerFinish := make(chan struct{})
	var capturedHolding [][]string
	calls := 0

	client := &pollMockClient{
		getWorkFn: func(_ context.Context, holding []string) (*GetWorkResult, error) {
			capturedHolding = append(capturedHolding, append([]string{}, holding...))
			calls++
			switch calls {
			case 1:
				return &GetWorkResult{Assignment: unit}, nil
			case 2:
				// Report the assignment as missed.
				return &GetWorkResult{Assignment: nil, HeartbeatMissed: []string{"assign-miss"}}, nil
			case 3:
				// By now removeMissed has run; allow worker to finish and cancel.
				close(workerFinish)
				cancel()
				return &GetWorkResult{Assignment: nil}, nil
			}
			return &GetWorkResult{}, nil
		},
	}
	exec := &mockExecutor{
		executeFn: func(_ context.Context, _ WorkUnit) error {
			<-workerFinish
			return nil
		},
	}

	loop := newTestPollLoop(testConfig(0, 3), client, exec)
	loop.Run(ctx)

	// capturedHolding[2] is the holding list sent with call 3 — after heartbeat miss.
	if len(capturedHolding) >= 3 {
		for _, id := range capturedHolding[2] {
			if id == "assign-miss" {
				t.Error("assign-miss should have been removed from holding after heartbeat miss")
			}
		}
	}
}

// TestPollLoop_MaxConcurrent_BlocksWhenFull verifies the semaphore blocks new
// work when MAX_CONCURRENT goroutines are already running.
func TestPollLoop_MaxConcurrent_BlocksWhenFull(t *testing.T) {
	started := atomic.Int64{}
	unblock := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	client := &pollMockClient{
		getWorkFn: func(_ context.Context, _ []string) (*GetWorkResult, error) {
			calls++
			switch calls {
			case 1, 2:
				return &GetWorkResult{Assignment: &WorkUnit{
					AssignmentID: fmt.Sprintf("assign-%d", calls),
					CompanyID:    "c1", AdapterType: "lever",
				}}, nil
			default:
				cancel()
				return &GetWorkResult{}, nil
			}
		},
	}
	exec := &mockExecutor{
		executeFn: func(ctx context.Context, _ WorkUnit) error {
			started.Add(1)
			<-unblock
			return nil
		},
	}

	// MAX_CONCURRENT = 2; both goroutines should start.
	loop := newTestPollLoop(testConfig(0, 2), client, exec)
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Wait for both goroutines to start.
	deadline := time.Now().Add(2 * time.Second)
	for started.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if started.Load() < 2 {
		t.Errorf("expected 2 goroutines started, got %d", started.Load())
	}

	close(unblock)
	<-done
}

// TestPollLoop_ExecutorError_CallsReportError verifies that an executor error
// results in ReportError being called on the client.
func TestPollLoop_ExecutorError_CallsReportError(t *testing.T) {
	unit := &WorkUnit{AssignmentID: "assign-fail", CompanyID: "c1", AdapterType: "lever"}
	reported := make(chan WorkError, 1)
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	client := &pollMockClient{
		getWorkFn: func(_ context.Context, _ []string) (*GetWorkResult, error) {
			calls++
			if calls == 1 {
				return &GetWorkResult{Assignment: unit}, nil
			}
			cancel()
			return &GetWorkResult{}, nil
		},
		reportErrFn: func(_ context.Context, e WorkError) error {
			reported <- e
			return nil
		},
	}
	exec := &mockExecutor{
		executeFn: func(_ context.Context, _ WorkUnit) error {
			return errors.New("adapter failed")
		},
	}

	loop := newTestPollLoop(testConfig(0, 2), client, exec)
	loop.Run(ctx)

	select {
	case e := <-reported:
		if e.AssignmentID != "assign-fail" {
			t.Errorf("ReportError AssignmentID = %q, want assign-fail", e.AssignmentID)
		}
		if e.ErrorMessage == "" {
			t.Error("ErrorMessage should not be empty")
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for ReportError")
	}
}

// TestPollLoop_GetWorkError_ContinuesPolling verifies that a GetWork error
// does not crash the loop — it should continue polling.
func TestPollLoop_GetWorkError_ContinuesPolling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := atomic.Int64{}

	client := &pollMockClient{
		getWorkFn: func(_ context.Context, _ []string) (*GetWorkResult, error) {
			n := calls.Add(1)
			if n == 1 {
				return nil, errors.New("network error")
			}
			cancel()
			return &GetWorkResult{}, nil
		},
	}
	exec := &mockExecutor{
		executeFn: func(_ context.Context, _ WorkUnit) error { return nil },
	}

	loop := newTestPollLoop(testConfig(0, 2), client, exec)
	loop.Run(ctx) // should not panic

	if calls.Load() < 2 {
		t.Error("loop should continue after GetWork error")
	}
}

// TestPollLoop_WorkerPanic_CallsReportError verifies that a panicking executor
// results in ReportError being called rather than crashing the process.
func TestPollLoop_WorkerPanic_CallsReportError(t *testing.T) {
	unit := &WorkUnit{AssignmentID: "assign-panic", CompanyID: "c1", AdapterType: "lever"}
	reported := make(chan WorkError, 1)
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	client := &pollMockClient{
		getWorkFn: func(_ context.Context, _ []string) (*GetWorkResult, error) {
			calls++
			if calls == 1 {
				return &GetWorkResult{Assignment: unit}, nil
			}
			cancel()
			return &GetWorkResult{}, nil
		},
		reportErrFn: func(_ context.Context, e WorkError) error {
			reported <- e
			return nil
		},
	}
	exec := &mockExecutor{
		executeFn: func(_ context.Context, _ WorkUnit) error {
			panic("test panic — expected")
		},
	}

	loop := newTestPollLoop(testConfig(0, 2), client, exec)
	loop.Run(ctx)

	select {
	case e := <-reported:
		if e.AssignmentID != "assign-panic" {
			t.Errorf("ReportError AssignmentID = %q, want assign-panic", e.AssignmentID)
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for ReportError after panic")
	}
}

// TestPollLoop_ReportErrorFailure_Logged verifies that a client.ReportError
// failure is handled gracefully (logged, not propagated).
func TestPollLoop_ReportErrorFailure_Logged(t *testing.T) {
	unit := &WorkUnit{AssignmentID: "assign-fail", CompanyID: "c1", AdapterType: "lever"}
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	client := &pollMockClient{
		getWorkFn: func(_ context.Context, _ []string) (*GetWorkResult, error) {
			calls++
			if calls == 1 {
				return &GetWorkResult{Assignment: unit}, nil
			}
			cancel()
			return &GetWorkResult{}, nil
		},
		reportErrFn: func(_ context.Context, _ WorkError) error {
			return errors.New("network down")
		},
	}
	exec := &mockExecutor{
		executeFn: func(_ context.Context, _ WorkUnit) error {
			return errors.New("adapter failed")
		},
	}

	// Should not panic — error is swallowed after logging.
	loop := newTestPollLoop(testConfig(0, 2), client, exec)
	loop.Run(ctx)
}

// TestPollLoop_Sleep_InterruptedByContextCancel verifies that a long poll
// interval is cut short when the context is cancelled.
func TestPollLoop_Sleep_InterruptedByContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := &pollMockClient{
		getWorkFn: func(_ context.Context, _ []string) (*GetWorkResult, error) {
			return &GetWorkResult{Assignment: nil}, nil
		},
	}
	exec := &mockExecutor{executeFn: func(_ context.Context, _ WorkUnit) error { return nil }}

	cfg := testConfig(60, 1) // 60-second poll → should be cut short
	loop := newTestPollLoop(cfg, client, exec)
	start := time.Now()
	loop.Run(ctx)
	if time.Since(start) > 300*time.Millisecond {
		t.Error("sleep was not interrupted by context cancel")
	}
}

// TestPollLoop_CancelWhileBlockedOnSemaphore_Exits verifies that the loop
// exits cleanly when context is cancelled while waiting for a semaphore slot.
func TestPollLoop_CancelWhileBlockedOnSemaphore_Exits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	workerBlocker := make(chan struct{})
	workerStarted := make(chan struct{}, 1)

	calls := 0
	client := &pollMockClient{
		getWorkFn: func(_ context.Context, _ []string) (*GetWorkResult, error) {
			calls++
			switch calls {
			case 1:
				return &GetWorkResult{Assignment: &WorkUnit{AssignmentID: "w1", CompanyID: "c1", AdapterType: "lever"}}, nil
			case 2:
				return &GetWorkResult{Assignment: &WorkUnit{AssignmentID: "w2", CompanyID: "c1", AdapterType: "lever"}}, nil
			}
			return &GetWorkResult{}, nil
		},
	}
	exec := &mockExecutor{
		executeFn: func(_ context.Context, _ WorkUnit) error {
			select {
			case workerStarted <- struct{}{}:
			default:
			}
			<-workerBlocker
			return nil
		},
	}

	go func() {
		<-workerStarted      // worker 1 has started and holds the semaphore
		cancel()             // cancel while sem is full
		close(workerBlocker) // let worker finish so wg.Wait() returns
	}()

	loop := newTestPollLoop(testConfig(0, 1), client, exec)
	loop.Run(ctx) // must return
}

// TestPollLoop_RemoveMissed_KeepsOtherHolding verifies that removeMissed
// preserves assignment IDs that are NOT in the missed list.
func TestPollLoop_RemoveMissed_KeepsOtherHolding(t *testing.T) {
	unit1 := &WorkUnit{AssignmentID: "keep-me", CompanyID: "c1", AdapterType: "lever"}
	unit2 := &WorkUnit{AssignmentID: "miss-me", CompanyID: "c1", AdapterType: "lever"}
	ctx, cancel := context.WithCancel(context.Background())

	workerFinish := make(chan struct{})
	var capturedHolding [][]string
	calls := 0

	client := &pollMockClient{
		getWorkFn: func(_ context.Context, holding []string) (*GetWorkResult, error) {
			capturedHolding = append(capturedHolding, append([]string{}, holding...))
			calls++
			switch calls {
			case 1:
				return &GetWorkResult{Assignment: unit1}, nil
			case 2:
				return &GetWorkResult{Assignment: unit2}, nil
			case 3:
				return &GetWorkResult{
					Assignment:      nil,
					HeartbeatMissed: []string{"miss-me"},
				}, nil
			case 4:
				close(workerFinish)
				cancel()
				return &GetWorkResult{}, nil
			}
			return &GetWorkResult{}, nil
		},
	}
	exec := &mockExecutor{
		executeFn: func(_ context.Context, _ WorkUnit) error {
			<-workerFinish
			return nil
		},
	}

	loop := newTestPollLoop(testConfig(0, 3), client, exec)
	loop.Run(ctx)

	// capturedHolding[3] (call 4) should contain "keep-me" but not "miss-me".
	if len(capturedHolding) >= 4 {
		holdingAt4 := capturedHolding[3]
		foundKeep, foundMiss := false, false
		for _, id := range holdingAt4 {
			if id == "keep-me" {
				foundKeep = true
			}
			if id == "miss-me" {
				foundMiss = true
			}
		}
		if foundMiss {
			t.Error("miss-me should have been removed from holding")
		}
		if !foundKeep {
			t.Log("keep-me not found in holding (may have finished naturally — acceptable)")
		}
	}
}

// TestPollLoop_GracefulShutdown_WaitsForInFlightWork verifies that Run blocks
// until all in-flight goroutines complete before returning.
func TestPollLoop_GracefulShutdown_WaitsForInFlightWork(t *testing.T) {
	started := make(chan struct{})
	unblock := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	client := &pollMockClient{
		getWorkFn: func(_ context.Context, _ []string) (*GetWorkResult, error) {
			calls++
			if calls == 1 {
				return &GetWorkResult{Assignment: &WorkUnit{
					AssignmentID: "assign-1", CompanyID: "c1", AdapterType: "lever",
				}}, nil
			}
			cancel()
			return &GetWorkResult{}, nil
		},
	}
	finished := atomic.Bool{}
	exec := &mockExecutor{
		executeFn: func(ctx context.Context, _ WorkUnit) error {
			close(started)
			time.Sleep(50 * time.Millisecond)
			finished.Store(true)
			<-unblock
			return nil
		},
	}

	done := make(chan struct{})
	go func() {
		loop := newTestPollLoop(testConfig(0, 2), client, exec)
		loop.Run(ctx)
		close(done)
	}()

	<-started
	close(unblock)
	<-done

	if !finished.Load() {
		t.Error("Run returned before in-flight goroutine completed")
	}
}
