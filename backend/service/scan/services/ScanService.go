package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// SchedulerCycleResult is the per-tick summary returned to the scheduler
// binary. Enqueued counts companies inserted into the queue this tick;
// SkippedNoSettings counts companies the scheduler ignored because they
// have no `company_scan_settings` row yet (Phase 2: post-cutover companies
// start un-onboarded for scanning until an admin configures them).
type SchedulerCycleResult struct {
	Enqueued          int
	SkippedNoSettings int
}

// SchedulerService is the narrow interface the scheduler worker depends on.
type SchedulerService interface {
	EnqueueDue(ctx context.Context, intervalMinutes int) (SchedulerCycleResult, error)
	// ResetStuck resets queue rows stuck in 'running' state whose heartbeat has
	// expired. Returns the count of rows recovered so the scheduler can log and
	// metric the recovery event — zero is not an error.
	ResetStuck(ctx context.Context, staleDur time.Duration) (int, error)
	// PurgeInactive removes queued (not processing) entries for companies that
	// have been deactivated since they were enqueued. Returns number of rows removed.
	PurgeInactive(ctx context.Context) (int, error)
	// RetryDueJobs fetches ledger rows in reason='retry' whose next_attempt_at
	// is due, then re-publishes them to jobs.raw.{adapter} or escalates to
	// reason='error' when the retry budget is exhausted.
	RetryDueJobs(ctx context.Context) error
	// EscalateStuckRetryJobs finds reason='retry' rows that have not been
	// re-published within 4 hours of their next_attempt_at and sets them to
	// reason='error'. Returns the number of rows escalated.
	EscalateStuckRetryJobs(ctx context.Context) (int64, error)
	// ReclaimStuckFetchRows DELETEs reason='fetch' ledger rows (removed_at IS
	// NULL) older than 1 hour to clear the dedup block for jobs the crawler
	// marked seen but the filter never transitioned. Returns the number of rows
	// reclaimed.
	ReclaimStuckFetchRows(ctx context.Context) (int64, error)
	// AutoPauseChronicFailures sets company_scan_settings.auto_paused=true for
	// every company whose consecutive_errors has crossed AutoPauseThreshold and
	// is not already paused, writing one admin_audit_log row per newly paused
	// company. Idempotent: an already-paused company is neither re-paused nor
	// re-audited. Returns the number of companies newly paused — zero is not an
	// error. Audit writes are non-fatal (a failed audit never blocks the pause).
	AutoPauseChronicFailures(ctx context.Context) (int, error)
	// RefreshGaugeMetrics recomputes the current-state gauge metrics (queue
	// depth, oldest-pending age, online node count, auto-paused company
	// count, retry-ladder backlog). Each of the 5 reads is independent —
	// a failure on one is joined into the returned error but never blocks
	// the others.
	RefreshGaugeMetrics(ctx context.Context) error
}

// Service defines all business operations for the scan domain.
// It embeds SchedulerService so the scheduler worker can depend on the
// narrower interface while the REST decorator chain uses the full Service.
type Service interface {
	SchedulerService
	TriggerScan(ctx context.Context, companyID string) error
	TriggerScanAll(ctx context.Context) (int, error)
	GetStats(ctx context.Context, companyID string) (*dto.ScanStatsView, error)
	GetAnalytics(ctx context.Context, companyID string) (*dto.CompanyAnalytics, error)
	ListQueue(ctx context.Context, filter dto.QueueFilter) ([]dto.QueueEntryView, error)
	RemoveFromQueue(ctx context.Context, entryID string) error
	// GetEffectiveScanConfig returns the merged scan config for a company:
	// the adapter default (from the ats table) plus any company-level override.
	GetEffectiveScanConfig(ctx context.Context, companyID string) (dto.EffectiveScanConfigDTO, error)
	// UpdateScanIntervalOverride patches the per-company scan interval override
	// on company_scan_settings. Pass nil to clear the override.
	UpdateScanIntervalOverride(ctx context.Context, companyID string, override *int) error
}

// BaseService auto-delegates all Service methods to next.
// Embed this in decorators and override only the methods they own.
type BaseService struct {
	next Service
}

// NewBaseService constructs a BaseService wrapping next.
// Panics if next is nil to surface wiring mistakes early.
func NewBaseService(next Service) BaseService {
	if next == nil {
		panic("BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) EnqueueDue(ctx context.Context, intervalMinutes int) (SchedulerCycleResult, error) {
	return b.next.EnqueueDue(ctx, intervalMinutes)
}

func (b *BaseService) ResetStuck(ctx context.Context, staleDur time.Duration) (int, error) {
	return b.next.ResetStuck(ctx, staleDur)
}

func (b *BaseService) PurgeInactive(ctx context.Context) (int, error) {
	return b.next.PurgeInactive(ctx)
}

func (b *BaseService) RetryDueJobs(ctx context.Context) error {
	return b.next.RetryDueJobs(ctx)
}

func (b *BaseService) EscalateStuckRetryJobs(ctx context.Context) (int64, error) {
	return b.next.EscalateStuckRetryJobs(ctx)
}

func (b *BaseService) ReclaimStuckFetchRows(ctx context.Context) (int64, error) {
	return b.next.ReclaimStuckFetchRows(ctx)
}

func (b *BaseService) AutoPauseChronicFailures(ctx context.Context) (int, error) {
	return b.next.AutoPauseChronicFailures(ctx)
}

func (b *BaseService) RefreshGaugeMetrics(ctx context.Context) error {
	return b.next.RefreshGaugeMetrics(ctx)
}

func (b *BaseService) TriggerScan(ctx context.Context, companyID string) error {
	return b.next.TriggerScan(ctx, companyID)
}

func (b *BaseService) TriggerScanAll(ctx context.Context) (int, error) {
	return b.next.TriggerScanAll(ctx)
}

func (b *BaseService) GetStats(ctx context.Context, companyID string) (*dto.ScanStatsView, error) {
	return b.next.GetStats(ctx, companyID)
}

func (b *BaseService) GetAnalytics(ctx context.Context, companyID string) (*dto.CompanyAnalytics, error) {
	return b.next.GetAnalytics(ctx, companyID)
}

func (b *BaseService) ListQueue(ctx context.Context, filter dto.QueueFilter) ([]dto.QueueEntryView, error) {
	return b.next.ListQueue(ctx, filter)
}

func (b *BaseService) RemoveFromQueue(ctx context.Context, entryID string) error {
	return b.next.RemoveFromQueue(ctx, entryID)
}

func (b *BaseService) GetEffectiveScanConfig(ctx context.Context, companyID string) (dto.EffectiveScanConfigDTO, error) {
	return b.next.GetEffectiveScanConfig(ctx, companyID)
}

func (b *BaseService) UpdateScanIntervalOverride(ctx context.Context, companyID string, override *int) error {
	return b.next.UpdateScanIntervalOverride(ctx, companyID, override)
}
