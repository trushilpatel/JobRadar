package common

import "time"

// Back-off tuning for the crawler retry hot-loop fix. A company whose scan just
// failed must not be re-served instantly; its queue row carries a
// next_attempt_at = now() + Backoff(consecutive_errors). These are correctness
// floors tied to the queue's fairness contract, not per-deploy knobs, so they
// live as module constants (mirroring stuckRetryThreshold/stuckFetchThreshold
// in ScanSchedulerService.go) to keep the operator surface minimal.
const (
	// BackoffBase is the delay after the first failure.
	BackoffBase = 5 * time.Minute
	// BackoffFactor is the exponential multiplier per consecutive failure.
	BackoffFactor = 2
	// BackoffCap is the maximum back-off; a chronically-failing company is
	// retried at most this often (before AutoPauseThreshold pauses it entirely).
	BackoffCap = 6 * time.Hour
	// BackoffMaxShift clamps the 2^n shift so a large consecutive_errors count
	// can never overflow the exponent. BackoffBase << BackoffMaxShift is already
	// far larger than BackoffCap, so the cap dominates long before the clamp
	// bites — the clamp only prevents shift overflow.
	BackoffMaxShift = 12

	// AutoPauseThreshold is the consecutive-error count at which the scheduler
	// sweep auto-pauses a company (sets company_scan_settings.auto_paused=true).
	AutoPauseThreshold = 50
)

// Backoff returns the retry delay for a queue row after consecutiveErrors
// failures: min(BackoffCap, BackoffBase * BackoffFactor^min(consecutiveErrors,
// BackoffMaxShift)). It is a pure function (no DB, no clock) so it is unit
// testable in isolation and reusable by both the node store and the legacy
// serialized store. A non-positive consecutiveErrors clamps to BackoffBase.
func Backoff(consecutiveErrors int) time.Duration {
	if consecutiveErrors <= 0 {
		return BackoffBase
	}
	shift := consecutiveErrors
	if shift > BackoffMaxShift {
		shift = BackoffMaxShift
	}
	// BackoffFactor is 2, so 2^shift is a left-shift — clamped above so it never
	// overflows.
	d := BackoffBase * time.Duration(1<<uint(shift))
	if d > BackoffCap || d <= 0 {
		return BackoffCap
	}
	return d
}
