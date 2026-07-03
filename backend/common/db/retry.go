package db

import (
	"context"
	stderrors "errors"
	"io"
	"net"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
)

// Retry-loop tuning. These are correctness floors, not per-deploy knobs: three
// attempts with a short exponential backoff covers a single Supavisor idle-drop
// (the connection is re-established on the immediate next attempt) without
// letting a genuinely-down database spin the caller. Kept as consts, never
// magic literals.
const (
	dbRetryMaxAttempts = 3
	dbRetryBaseDelay   = 50 * time.Millisecond
	dbRetryFactor      = 2
)

// retryLog holds the package-level logger used by WithRetry, defaulting to a
// noop so the db package has no mandatory logging dependency and callers
// (stores) can invoke WithRetry as a free function without threading a logger
// through every constructor. Workers may call SetLogger in main() to surface
// the WARN lines in Loki. Stored in an atomic so SetLogger and WithRetry's read
// are race-free even if a stray SetLogger races a live retry.
var retryLog atomic.Pointer[logger.Logger]

func init() {
	noop := logger.NewNoop()
	retryLog.Store(&noop)
}

// currentRetryLog returns the installed logger (never nil).
func currentRetryLog() logger.Logger {
	if p := retryLog.Load(); p != nil {
		return *p
	}
	return logger.NewNoop()
}

// SetLogger installs the process logger used for WithRetry's transient-retry
// WARN lines. Call once from main() before serving traffic. A nil argument is
// ignored so a mis-wire can never blank the logger.
func SetLogger(l logger.Logger) {
	if l != nil {
		retryLog.Store(&l)
	}
}

// IsTransientConnError reports whether err is a transient connection-drop that
// is safe to retry for an idempotent operation. It returns true ONLY for the
// connection-drop class:
//
//   - io.EOF / io.ErrUnexpectedEOF (Supavisor closed an idle connection
//     mid-flight — the pgx-marked-not-safe-to-retry case this whole fix exists
//     for),
//   - *net.OpError and the underlying ECONNRESET / EPIPE syscall errors,
//   - anything pgconn itself marks SafeToRetry (sent before the server read it).
//
// It returns false for context cancellation/deadline and, belt-and-suspenders,
// for the domain row-missing sentinels (ErrLedgerRowMissing / ErrQueueRowMissing)
// — those are business outcomes, never connection drops, and must never be
// retried. Every other error (constraint violations, business errors) is false.
func IsTransientConnError(err error) bool {
	if err == nil {
		return false
	}
	// Never retry cancellation/deadline — the caller asked us to stop.
	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Never retry the row-count sentinels — they are business outcomes that must
	// pass straight through even if wrapped alongside a transient-looking error.
	if stderrors.Is(err, domainerrs.ErrLedgerRowMissing) || stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
		return false
	}
	// Connection-drop EOFs — the core Supavisor idle-drop symptom.
	if stderrors.Is(err, io.EOF) || stderrors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	// Reset-by-peer / broken-pipe at the syscall layer.
	if stderrors.Is(err, syscall.ECONNRESET) || stderrors.Is(err, syscall.EPIPE) {
		return true
	}
	// Any network op error (dial/read/write on a dropped socket).
	var opErr *net.OpError
	if stderrors.As(err, &opErr) {
		return true
	}
	// pgconn's own safe-to-retry marker (query never reached the server).
	if pgconn.SafeToRetry(err) {
		return true
	}
	return false
}

// WithRetry runs op, retrying up to dbRetryMaxAttempts times when op returns a
// transient connection-drop error (IsTransientConnError) AND the context is not
// yet cancelled. Between attempts it sleeps dbRetryBaseDelay, doubling each
// time, and aborts the sleep early if the context is cancelled. It returns nil
// on success, the last error on exhaustion, and the error immediately (no
// retry) for any non-transient error — so business errors such as
// ErrLedgerRowMissing are never retried. Each retry increments
// DBTransientRetriesTotal and emits a WARN line via the package logger.
//
// op MUST be idempotent: WithRetry may execute it multiple times. Only wrap the
// raw Exec/Update surface; evaluate row-count sentinels on the final success.
func WithRetry(ctx context.Context, op func() error) error {
	delay := dbRetryBaseDelay
	var lastErr error
	for attempt := 1; attempt <= dbRetryMaxAttempts; attempt++ {
		err := op()
		if err == nil {
			return nil
		}
		lastErr = err
		// Stop immediately on non-transient errors or if the context is done.
		if !IsTransientConnError(err) || ctx.Err() != nil {
			return err
		}
		// No point sleeping after the final attempt.
		if attempt == dbRetryMaxAttempts {
			break
		}
		metrics.DBTransientRetriesTotal.Inc()
		currentRetryLog().Warn("db: retrying after transient connection error",
			logger.Int(logger.FieldAttempt, attempt),
			logger.Int(logger.FieldMaxAttempts, dbRetryMaxAttempts),
			logger.Err(err),
		)
		// Context-aware backoff sleep.
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return err
		case <-timer.C:
		}
		delay *= dbRetryFactor
	}
	return lastErr
}
