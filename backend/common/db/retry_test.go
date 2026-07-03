package db_test

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/trushilpatel/jobradar/common/db"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
)

// errPGSafeToRetry is a pgconn error that reports SafeToRetry()==true. pgconn's
// SafeToRetry unwraps looking for an error whose SafeToRetry() method returns
// true; a *pgconn.PgError before any write qualifies via safeToRetry wrapper,
// but the simplest deterministic case is a custom error implementing the
// interface pgconn checks.
type errSafeToRetry struct{}

func (errSafeToRetry) Error() string     { return "safe to retry" }
func (errSafeToRetry) SafeToRetry() bool { return true }

func TestIsTransientConnError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"io.EOF", io.EOF, true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"wrapped io.EOF", fmt.Errorf("read failed: %w", io.EOF), true},
		{"wrapped ErrUnexpectedEOF", fmt.Errorf("exec: %w", io.ErrUnexpectedEOF), true},
		{"ECONNRESET", syscall.ECONNRESET, true},
		{"EPIPE", syscall.EPIPE, true},
		{"wrapped ECONNRESET", fmt.Errorf("conn: %w", syscall.ECONNRESET), true},
		{"net.OpError", &net.OpError{Op: "read", Err: syscall.ECONNRESET}, true},
		{"wrapped net.OpError", fmt.Errorf("dial: %w", &net.OpError{Op: "dial", Err: io.EOF}), true},
		// A net.OpError whose inner error is neither EOF nor a known syscall —
		// only the *net.OpError As branch classifies it.
		{"net.OpError generic inner", &net.OpError{Op: "write", Err: stderrors.New("connection timed out")}, true},
		{"pgconn SafeToRetry", errSafeToRetry{}, true},
		{"wrapped pgconn SafeToRetry", fmt.Errorf("pg: %w", errSafeToRetry{}), true},
		{"context.Canceled", context.Canceled, false},
		{"context.DeadlineExceeded", context.DeadlineExceeded, false},
		{"ErrLedgerRowMissing", domainerrs.ErrLedgerRowMissing, false},
		{"ErrLedgerRowMissing via WithMessage", domainerrs.ErrLedgerRowMissing.WithMessage("x"), false},
		{"ErrQueueRowMissing", domainerrs.ErrQueueRowMissing, false},
		{"plain business error", stderrors.New("constraint violation"), false},
		{"not found sentinel", domainerrs.ErrNotFound, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := db.IsTransientConnError(tc.err); got != tc.want {
				t.Fatalf("IsTransientConnError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestIsTransientConnError_PgConnRealError confirms a genuine pgconn write-error
// wrapper is classified. pgconn.SafeToRetry over a *PgError wrapped in a
// safe-to-retry marker returns true; here we assert our predicate agrees with
// pgconn on a plain PgError (which is NOT safe to retry — server saw it).
func TestIsTransientConnError_PgErrorNotSafe(t *testing.T) {
	t.Parallel()
	pgErr := &pgconn.PgError{Code: "23505", Message: "duplicate key"}
	if db.IsTransientConnError(pgErr) {
		t.Fatal("a constraint PgError must not be classified transient")
	}
}

func TestWithRetry_SucceedsFirstTry(t *testing.T) {
	t.Parallel()
	calls := 0
	err := db.WithRetry(context.Background(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("WithRetry err = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (no retry on success)", calls)
	}
}

func TestWithRetry_TransientThenSucceeds(t *testing.T) {
	t.Parallel()
	calls := 0
	err := db.WithRetry(context.Background(), func() error {
		calls++
		if calls < 3 {
			return io.ErrUnexpectedEOF
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithRetry err = %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (retried twice then succeeded)", calls)
	}
}

func TestWithRetry_ExhaustsReturnsLastErr(t *testing.T) {
	t.Parallel()
	calls := 0
	sentinel := fmt.Errorf("attempt-%d: %w", 0, io.EOF)
	err := db.WithRetry(context.Background(), func() error {
		calls++
		return sentinel
	})
	if !stderrors.Is(err, io.EOF) {
		t.Fatalf("want wrapped io.EOF returned on exhaustion, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (max attempts)", calls)
	}
}

func TestWithRetry_NonTransientReturnsImmediately(t *testing.T) {
	t.Parallel()
	calls := 0
	businessErr := domainerrs.ErrLedgerRowMissing.WithMessage("no row")
	err := db.WithRetry(context.Background(), func() error {
		calls++
		return businessErr
	})
	if !stderrors.Is(err, domainerrs.ErrLedgerRowMissing) {
		t.Fatalf("want ErrLedgerRowMissing passed straight through, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (business error must NOT be retried)", calls)
	}
}

func TestWithRetry_CtxCancelledMidRetryStops(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := db.WithRetry(ctx, func() error {
		calls++
		// Cancel during the first attempt so the backoff sleep observes it.
		cancel()
		return io.EOF
	})
	if err == nil {
		t.Fatal("want error when ctx cancelled mid-retry")
	}
	// The op ran once; the cancellation aborts before a second attempt.
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (ctx cancel stops retry)", calls)
	}
}

func TestWithRetry_AlreadyCancelledCtxDoesNotRetry(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := db.WithRetry(ctx, func() error {
		calls++
		return io.EOF
	})
	if err == nil {
		t.Fatal("want error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (already-cancelled ctx: single attempt, no retry)", calls)
	}
}

// TestWithRetry_RespectsDeadlineBudget confirms the backoff sleeps are bounded
// and the loop completes quickly on exhaustion (three ~50ms/100ms sleeps).
func TestWithRetry_ExhaustionIsBounded(t *testing.T) {
	t.Parallel()
	start := time.Now()
	_ = db.WithRetry(context.Background(), func() error { return io.EOF })
	// Two inter-attempt sleeps: 50ms + 100ms = 150ms. Generous upper bound.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("exhaustion took %v, expected well under 2s", elapsed)
	}
}

// TestSetLogger_NilIgnored confirms SetLogger(nil) is a safe no-op and WithRetry
// still functions afterward.
func TestSetLogger_NilIgnored(t *testing.T) {
	db.SetLogger(nil)
	if err := db.WithRetry(context.Background(), func() error { return nil }); err != nil {
		t.Fatalf("WithRetry after SetLogger(nil) err = %v", err)
	}
}

// TestSetLogger_InstallsLogger confirms a non-nil logger is installed (the WARN
// line emitted on retry uses it). Using a noop keeps the test silent; the point
// is exercising the non-nil install branch. Restore the default afterward so
// test ordering is unaffected.
func TestSetLogger_InstallsLogger(t *testing.T) {
	db.SetLogger(logger.NewNoop())
	t.Cleanup(func() { db.SetLogger(logger.NewNoop()) })
	// One transient-then-succeed run so the retry WARN path (which touches the
	// installed logger) executes.
	attempts := 0
	err := db.WithRetry(context.Background(), func() error {
		attempts++
		if attempts < 2 {
			return io.EOF
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithRetry err = %v", err)
	}
}

// TestWithRetry_CtxCancelledDuringBackoffSleep drives the select-on-ctx.Done
// branch inside the backoff sleep: the op returns a transient error (entering
// the sleep), and the context is cancelled while the backoff timer is pending,
// so WithRetry returns before the next attempt.
func TestWithRetry_CtxCancelledDuringBackoffSleep(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	// Cancel shortly after the first attempt returns, while the ~50ms backoff
	// timer is still pending.
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	err := db.WithRetry(ctx, func() error {
		attempts++
		return io.ErrUnexpectedEOF // transient → enters backoff sleep
	})
	if err == nil {
		t.Fatal("want error when ctx cancelled during backoff sleep")
	}
	// Only the first attempt ran; cancellation aborted the sleep before a retry.
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (sleep aborted by ctx cancel)", attempts)
	}
}
