package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	commondb "github.com/trushilpatel/jobradar/common/db"
	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	"gorm.io/gorm"
)

// EnqueueDueResult is returned by EnqueueDue. Enqueued is the number of
// companies inserted into the queue this tick. SkippedNoSettings is the
// number of `companies` rows that were considered but skipped because they
// have no matching `company_scan_settings` row (Phase 2: post-cutover
// companies start un-onboarded for scanning). Workers/scheduler log
// SkippedNoSettings once per cycle so admins can see which companies still
// need configuration.
type EnqueueDueResult struct {
	Enqueued          int
	SkippedNoSettings int
}

// ScanQueueStore manages the company scan work queue.
type ScanQueueStore interface {
	// Enqueue inserts a company into the queue. Silently no-ops if the company
	// already has an active entry (queued or running). Returns NOT_FOUND when
	// the company has no `company_scan_settings` row — manual triggers must
	// reject un-onboarded companies with a clear error.
	Enqueue(ctx context.Context, companyID, source string) error

	// EnqueueDue atomically enqueues all companies currently due for a scan
	// (scan_settings exists and active=true, last_scan_at past the effective
	// interval, not already queued) in one query. Returns the number enqueued
	// and the number of companies skipped because they have no scan_settings
	// row yet. Safe for concurrent callers — uses ON CONFLICT DO NOTHING so
	// two instances never double-enqueue.
	EnqueueDue(ctx context.Context, intervalMinutes int, source string) (EnqueueDueResult, error)

	// EnqueueAll immediately enqueues every active company (scan_settings
	// exists and active=true) that is not already queued or running. Used by
	// manual "scan all" — bypasses the scan-interval cooldown. Returns the
	// number of rows inserted.
	EnqueueAll(ctx context.Context, source string) (int, error)

	// ClaimNext atomically claims the next queued item for the given adapterType,
	// enforcing the inter-company delay from adapter_configs via a FOR UPDATE lock.
	// Returns nil, nil if the queue is empty or the delay has not elapsed.
	ClaimNext(ctx context.Context, adapterType string) (*dto.ScanQueueItem, error)

	// Complete removes the queue entry. Call after scan succeeds or fails.
	// Returns ErrQueueRowMissing when no row matched (already removed/reassigned).
	Complete(ctx context.Context, id string) error

	// Remove deletes a queued entry by ID. Returns ErrQueueRowMissing when no
	// queued row matched (entry running or already gone) — the admin API maps
	// this to HTTP 404.
	Remove(ctx context.Context, id string) error

	// ResetStuck resets running rows whose heartbeat (or started_at if no heartbeat yet) is older than staleDur.
	// Returns the number of rows reclaimed (purely additive observability — zero is not an error).
	ResetStuck(ctx context.Context, staleDur time.Duration) (int, error)

	// PurgeInactive deletes all queued (not running) entries whose company
	// is inactive. Returns the number of rows removed.
	PurgeInactive(ctx context.Context) (int, error)

	// AutoPauseChronicFailures sets company_scan_settings.auto_paused=true for
	// every company whose consecutive_errors >= threshold and is not already
	// paused, returning the IDs newly paused (RETURNING company_id). Returning 0
	// rows is NOT an error — the sweep is idempotent and no-ops when nothing
	// crosses the threshold (mirrors the ReclaimStuckFetchRows convention).
	AutoPauseChronicFailures(ctx context.Context, threshold int) ([]string, error)

	// UpdateHeartbeat refreshes the heartbeat timestamp for an in-progress row.
	// Returns ErrQueueRowMissing when no row matched — signals the row was
	// reassigned by ResetStuck and the crawler must abort the scan.
	UpdateHeartbeat(ctx context.Context, id string) error

	// ListQueue returns queue entries matching filter, ordered by priority DESC, queued_at ASC.
	ListQueue(ctx context.Context, filter dto.QueueFilter) ([]dto.ScanQueueItem, error)

	// CountQueued returns the current number of company_scan_queue rows in
	// status='queued'. Read-only snapshot — used by the scheduler's gauge
	// refresh, not the enqueue/claim path.
	CountQueued(ctx context.Context) (int, error)

	// OldestQueuedAgeSeconds returns the age in seconds of the oldest
	// status='queued' row, or 0 when the queue is empty.
	OldestQueuedAgeSeconds(ctx context.Context) (float64, error)
}

// --- GORM implementations ---

type scanQueueStore struct {
	db *gorm.DB
}

// NewQueueStore constructs a ScanQueueStore implementation.
func NewQueueStore(db *gorm.DB) ScanQueueStore {
	return &scanQueueStore{db: db}
}

func (s *scanQueueStore) Enqueue(ctx context.Context, companyID, source string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.store.Enqueue")
	defer tracing.End(span, &err)
	// Phase 2: adapter_type lives on company_scan_settings, not companies.
	// If the company has no settings row, the SELECT returns 0 rows and the
	// INSERT is a no-op — caller (ImplService.TriggerScan) checks for the
	// scan_settings row first and returns a clear error to the admin.
	return s.db.WithContext(ctx).Exec(`
		INSERT INTO company_scan_queue (company_id, source, adapter_type)
		SELECT ?, ?, adapter_type FROM company_scan_settings WHERE company_id = ?
		ON CONFLICT DO NOTHING
	`, companyID, source, companyID).Error
}

func (s *scanQueueStore) EnqueueDue(ctx context.Context, intervalMinutes int, source string) (_ EnqueueDueResult, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.EnqueueDue")
	defer tracing.End(span, &err)

	// Phase 2 enqueue source is `company_scan_settings`, not `companies`.
	// Companies without a scan_settings row are silently skipped here; the
	// scheduler logs the count via the SkippedNoSettings field on the result.
	// Fix 1 (AC1.5): auto_paused companies are never enqueued (in addition to
	// the existing active=true filter). Fix 3 (AC3.7): the INSERT ... SELECT is
	// idempotent (ON CONFLICT DO NOTHING) so it is safe to retry on a transient
	// connection-drop; wrap only the Raw exec in WithRetry and evaluate the
	// result (ids) after the retried closure succeeds.
	var ids []string
	if err = commondb.WithRetry(ctx, func() error {
		ids = nil
		return s.db.WithContext(ctx).Raw(`
			INSERT INTO company_scan_queue (company_id, source, adapter_type)
			SELECT s.company_id, ?, s.adapter_type
			FROM company_scan_settings s
			LEFT JOIN ats a ON a.name = s.adapter_type
			WHERE s.active = true
			  AND s.auto_paused = false
			  AND NOT EXISTS (
			    SELECT 1 FROM company_scan_queue q
			    WHERE q.company_id = s.company_id AND q.status IN ('queued','running')
			  )
			  AND (
			    s.last_scan_at IS NULL
			    OR s.last_scan_at + COALESCE(
			        s.scan_interval_override,
			        a.scan_interval_min,
			        ?
			    ) * INTERVAL '1 minute' <= NOW()
			  )
			ORDER BY s.last_scan_at ASC NULLS FIRST
			ON CONFLICT DO NOTHING
			RETURNING company_id
		`, source, intervalMinutes).Scan(&ids).Error
	}); err != nil {
		return EnqueueDueResult{}, err
	}

	// Count companies that exist but have no scan_settings row — surfaced
	// once per scheduler tick so admins know they still need to onboard them.
	// Read-only COUNT is idempotent; retry on transient connection drops.
	var skipped int64
	if err = commondb.WithRetry(ctx, func() error {
		return s.db.WithContext(ctx).Raw(`
			SELECT COUNT(*)
			FROM companies c
			LEFT JOIN company_scan_settings s ON s.company_id = c.id
			WHERE s.company_id IS NULL
		`).Scan(&skipped).Error
	}); err != nil {
		return EnqueueDueResult{}, err
	}

	return EnqueueDueResult{Enqueued: len(ids), SkippedNoSettings: int(skipped)}, nil
}

func (s *scanQueueStore) EnqueueAll(ctx context.Context, source string) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.EnqueueAll")
	defer tracing.End(span, &err)
	var ids []string
	err = s.db.WithContext(ctx).Raw(`
		INSERT INTO company_scan_queue (company_id, source, adapter_type)
		SELECT s.company_id, ?, s.adapter_type
		FROM company_scan_settings s
		WHERE s.active = true
		  AND s.auto_paused = false
		  AND NOT EXISTS (
		    SELECT 1 FROM company_scan_queue q
		    WHERE q.company_id = s.company_id AND q.status IN ('queued','running')
		  )
		ON CONFLICT DO NOTHING
		RETURNING company_id
	`, source).Scan(&ids).Error
	return len(ids), err
}

func (s *scanQueueStore) ClaimNext(ctx context.Context, adapterType string) (_ *dto.ScanQueueItem, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.ClaimNext")
	defer tracing.End(span, &err)

	var item dto.ScanQueueItem
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Step 1: Lock this adapter's config row and check if the inter-company delay
		// has elapsed. FOR UPDATE serialises concurrent pod claims for this adapter type.
		// If the row is absent or the delay has not passed, no row is returned and we
		// return nil (worker sleeps and retries on the next poll interval).
		var delayS int
		res := tx.Raw(`
			SELECT inter_company_delay_s
			FROM ats
			WHERE name = ?
			  AND (
			    last_scheduled_at IS NULL
			    OR NOW() - last_scheduled_at >= inter_company_delay_s * INTERVAL '1 second'
			  )
			FOR UPDATE
		`, adapterType).Scan(&delayS)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Delay not elapsed or adapter unknown — signal nil return via sentinel.
			return errNoEligibleAdapter
		}

		// Step 2: Claim the oldest queued item for this adapter type.
		if err := tx.Raw(`
			UPDATE company_scan_queue
			SET status = 'running', started_at = NOW(), heartbeat_at = NOW()
			WHERE id = (
			    SELECT id FROM company_scan_queue
			    WHERE status = 'queued' AND adapter_type = ?
			      AND COALESCE(next_attempt_at, '-infinity'::timestamptz) <= NOW()
			    ORDER BY priority DESC, queued_at ASC
			    LIMIT 1
			    FOR UPDATE SKIP LOCKED
			)
			RETURNING id, company_id, adapter_type, source
		`, adapterType).Scan(&item).Error; err != nil {
			return err
		}

		// Step 3: Record when this adapter was last claimed so the next claim
		// waits for inter_company_delay_s before proceeding.
		return tx.Exec(`UPDATE ats SET last_scheduled_at = NOW() WHERE name = ?`, adapterType).Error
	})

	if txErr == errNoEligibleAdapter {
		return nil, nil
	}
	if txErr != nil {
		return nil, txErr
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

// errNoEligibleAdapter is a sentinel used inside the ClaimNext transaction to
// signal that no adapter_configs row was eligible (delay not elapsed or unknown
// adapter). It is never returned to callers — ClaimNext returns nil, nil instead.
var errNoEligibleAdapter = fmt.Errorf("no eligible adapter row")

func (s *scanQueueStore) PurgeInactive(ctx context.Context) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.PurgeInactive")
	defer tracing.End(span, &err)
	// Phase 2: `active` lives on company_scan_settings. A company is
	// considered inactive (queue-purgable) when its scan_settings row is
	// active=false, auto_paused=true (system decision — Fix 1, AC1.11), OR the
	// scan_settings row is absent entirely.
	result := s.db.WithContext(ctx).Exec(`
		DELETE FROM company_scan_queue
		WHERE status = 'queued'
		  AND company_id IN (
		    SELECT c.id
		    FROM companies c
		    LEFT JOIN company_scan_settings s ON s.company_id = c.id
		    WHERE s.company_id IS NULL OR s.active = false OR s.auto_paused = true
		  )
	`)
	return int(result.RowsAffected), result.Error
}

// AutoPauseChronicFailures flips company_scan_settings.auto_paused=true for
// every company whose consecutive_errors has crossed threshold and is not yet
// paused, returning the IDs newly paused. The WHERE auto_paused=false guard
// makes the sweep idempotent — an already-paused company is never re-selected,
// so it produces no duplicate audit rows. Returning 0 rows is NOT an error
// (mirrors the ReclaimStuckFetchRows no-sentinel convention). The UPDATE ...
// RETURNING is idempotent for the same threshold, so it is wrapped in WithRetry
// for transient-connection-drop resilience (Fix 3 parity).
func (s *scanQueueStore) AutoPauseChronicFailures(ctx context.Context, threshold int) (_ []string, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.AutoPauseChronicFailures")
	defer tracing.End(span, &err)
	var ids []string
	if err = commondb.WithRetry(ctx, func() error {
		ids = nil
		return s.db.WithContext(ctx).Raw(`
			UPDATE company_scan_settings
			SET auto_paused = true, updated_at = NOW()
			WHERE auto_paused = false AND consecutive_errors >= ?
			RETURNING company_id
		`, threshold).Scan(&ids).Error
	}); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *scanQueueStore) Complete(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.store.Complete")
	defer tracing.End(span, &err)
	res := s.db.WithContext(ctx).Exec(`DELETE FROM company_scan_queue WHERE id = ?`, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.ErrQueueRowMissing.WithMessage("Complete: no scan queue row for id=" + id)
	}
	return nil
}

func (s *scanQueueStore) Remove(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.store.Remove")
	defer tracing.End(span, &err)
	res := s.db.WithContext(ctx).Exec(`DELETE FROM company_scan_queue WHERE id = ? AND status = 'queued'`, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.ErrQueueRowMissing.WithMessage("Remove: no queued scan queue row for id=" + id)
	}
	return nil
}

func (s *scanQueueStore) ResetStuck(ctx context.Context, staleDur time.Duration) (rowsReset int, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.ResetStuck")
	defer tracing.End(span, &err)
	// Fix 3 (AC3.7): the reset is idempotent (re-running the same UPDATE on
	// already-queued rows is a no-op), so retry the raw Exec on a transient
	// connection-drop. RowsAffected is captured inside the closure and read only
	// after WithRetry succeeds.
	var rowsAffected int64
	if err = commondb.WithRetry(ctx, func() error {
		res := s.db.WithContext(ctx).Exec(`
			UPDATE company_scan_queue
			SET status = 'queued', started_at = NULL, heartbeat_at = NULL
			WHERE status = 'running'
			  AND COALESCE(heartbeat_at, started_at) < NOW() - INTERVAL '1 second' * ?
		`, int(staleDur.Seconds()))
		if res.Error != nil {
			return res.Error
		}
		rowsAffected = res.RowsAffected
		return nil
	}); err != nil {
		return 0, err
	}
	return int(rowsAffected), nil
}

func (s *scanQueueStore) UpdateHeartbeat(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.store.UpdateHeartbeat")
	defer tracing.End(span, &err)
	res := s.db.WithContext(ctx).Exec(`UPDATE company_scan_queue SET heartbeat_at = NOW() WHERE id = ?`, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.ErrQueueRowMissing.WithMessage("UpdateHeartbeat: no scan queue row for id=" + id)
	}
	return nil
}

func (s *scanQueueStore) ListQueue(ctx context.Context, filter dto.QueueFilter) (_ []dto.ScanQueueItem, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.ListQueue")
	defer tracing.End(span, &err)
	query := `SELECT id, company_id, adapter_type, status, source, priority, queued_at, started_at, heartbeat_at FROM company_scan_queue WHERE 1=1`
	var args []any
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if filter.Source != "" {
		query += ` AND source = ?`
		args = append(args, filter.Source)
	}
	query += ` ORDER BY priority DESC, queued_at ASC`
	var items []dto.ScanQueueItem
	err = s.db.WithContext(ctx).Raw(query, args...).Scan(&items).Error
	return items, err
}

func (s *scanQueueStore) CountQueued(ctx context.Context) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.CountQueued")
	defer tracing.End(span, &err)
	var count int64
	err = s.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM company_scan_queue WHERE status = 'queued'`).Scan(&count).Error
	return int(count), err
}

func (s *scanQueueStore) OldestQueuedAgeSeconds(ctx context.Context) (_ float64, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.OldestQueuedAgeSeconds")
	defer tracing.End(span, &err)
	var seconds float64
	err = s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(queued_at))), 0)
		FROM company_scan_queue WHERE status = 'queued'
	`).Scan(&seconds).Error
	return seconds, err
}

// --- repositoryService: terminal Service implementation ---

type repositoryService struct {
	queue   ScanQueueStore
	scanLog ScanLogReader
	config  Config
	db      *gorm.DB
	atsSvc  atssvc.AtsService
}

// NewRepositoryService constructs the terminal Service implementation backed by DB stores.
// atsSvc is used by GetEffectiveScanConfig to resolve the adapter's scan_interval_min from
// the ats table without a direct DB query in the service layer.
func NewRepositoryService(queue ScanQueueStore, scanLog ScanLogReader, cfg Config, db *gorm.DB, atsSvc atssvc.AtsService) Service {
	return &repositoryService{queue: queue, scanLog: scanLog, config: cfg, db: db, atsSvc: atsSvc}
}

func (s *repositoryService) EnqueueDue(ctx context.Context, intervalMinutes int) (_ SchedulerCycleResult, err error) {
	ctx, span := tracing.Start(ctx, "scan.repository.EnqueueDue")
	defer tracing.End(span, &err)
	res, err := s.queue.EnqueueDue(ctx, intervalMinutes, scancommon.QueueSourceScheduler)
	if err != nil {
		return SchedulerCycleResult{}, errors.ErrInternal.WithMessage("failed to enqueue due companies")
	}
	return SchedulerCycleResult(res), nil
}

func (s *repositoryService) ResetStuck(ctx context.Context, staleDur time.Duration) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.repository.ResetStuck")
	defer tracing.End(span, &err)
	n, err := s.queue.ResetStuck(ctx, staleDur)
	if err != nil {
		return 0, errors.ErrInternal.WithMessage("failed to reset stuck queue entries")
	}
	return n, nil
}

// TriggerScan enqueues a manual scan by company ID. No existence check — that's ImplService's job.
func (s *repositoryService) TriggerScan(ctx context.Context, companyID string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.repository.TriggerScan")
	defer tracing.End(span, &err)
	if err := s.queue.Enqueue(ctx, companyID, scancommon.QueueSourceManual); err != nil {
		return errors.ErrInternal.WithMessage("failed to trigger scan")
	}
	return nil
}

func (s *repositoryService) TriggerScanAll(ctx context.Context) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.repository.TriggerScanAll")
	defer tracing.End(span, &err)
	n, err := s.queue.EnqueueAll(ctx, scancommon.QueueSourceManual)
	if err != nil {
		return 0, errors.ErrInternal.WithMessage("failed to enqueue all active companies")
	}
	return n, nil
}

func (s *repositoryService) GetStats(ctx context.Context, companyID string) (_ *dto.ScanStatsView, err error) {
	ctx, span := tracing.Start(ctx, "scan.repository.GetStats")
	defer tracing.End(span, &err)
	stats, err := s.scanLog.GetScanStats(ctx, companyID)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch scan stats")
	}
	return &stats, nil
}

// ListQueue returns raw queue entries without company enrichment — ImplService overrides this.
func (s *repositoryService) ListQueue(ctx context.Context, filter dto.QueueFilter) (_ []dto.QueueEntryView, err error) {
	ctx, span := tracing.Start(ctx, "scan.repository.ListQueue")
	defer tracing.End(span, &err)
	items, err := s.queue.ListQueue(ctx, filter)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch queue")
	}
	views := make([]dto.QueueEntryView, 0, len(items))
	for _, item := range items {
		views = append(views, dto.QueueEntryView{
			ID:        item.ID,
			CompanyID: item.CompanyID,
			Status:    item.Status,
			Source:    item.Source,
			Priority:  item.Priority,
			QueuedAt:  item.QueuedAt,
			StartedAt: item.StartedAt,
		})
	}
	return views, nil
}

func (s *repositoryService) PurgeInactive(ctx context.Context) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "scan.repository.PurgeInactive")
	defer tracing.End(span, &err)
	n, err := s.queue.PurgeInactive(ctx)
	if err != nil {
		return 0, errors.ErrInternal.WithMessage("failed to purge inactive queue entries")
	}
	return n, nil
}

func (s *repositoryService) RemoveFromQueue(ctx context.Context, entryID string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.repository.RemoveFromQueue")
	defer tracing.End(span, &err)
	if err := s.queue.Remove(ctx, entryID); err != nil {
		// Propagate ErrQueueRowMissing so the admin API renders 404; wrap any
		// other store failure as a 500.
		if stderrors.Is(err, errors.ErrQueueRowMissing) {
			return errors.ErrQueueRowMissing.WithMessage("remove queue entry: " + err.Error())
		}
		return errors.ErrInternal.WithMessage("failed to remove queue entry")
	}
	return nil
}

func (s *repositoryService) GetAnalytics(ctx context.Context, companyID string) (_ *dto.CompanyAnalytics, err error) {
	ctx, span := tracing.Start(ctx, "scan.repository.GetAnalytics")
	defer tracing.End(span, &err)
	a, err := s.scanLog.GetCompanyAnalytics(ctx, companyID)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch company analytics")
	}
	return &a, nil
}

func (s *repositoryService) GetEffectiveScanConfig(ctx context.Context, companyID string) (_ dto.EffectiveScanConfigDTO, err error) {
	ctx, span := tracing.Start(ctx, "scan.repository.GetEffectiveScanConfig")
	defer tracing.End(span, &err)

	var settings companymodel.CompanyScanSettings
	if err = s.db.WithContext(ctx).First(&settings, "company_id = ?", companyID).Error; err != nil {
		if errors.IsNotFound(err) {
			return dto.EffectiveScanConfigDTO{}, errors.ErrNotFound.WithMessage("company has no scan settings configured")
		}
		return dto.EffectiveScanConfigDTO{}, errors.ErrInternal.WithMessage("failed to fetch company scan settings")
	}

	ats, err := s.atsSvc.GetATS(ctx, settings.AdapterType)
	if err != nil {
		return dto.EffectiveScanConfigDTO{}, err
	}

	effective := ats.ScanIntervalMin
	if settings.ScanIntervalOverride != nil {
		effective = *settings.ScanIntervalOverride
	}
	return dto.EffectiveScanConfigDTO{
		CompanyID:                    companyID,
		AdapterType:                  settings.AdapterType,
		AdapterScanIntervalMinutes:   ats.ScanIntervalMin,
		EffectiveScanIntervalMinutes: effective,
		ScanIntervalMinutesOverride:  settings.ScanIntervalOverride,
		InterCompanyDelayS:           ats.InterCompanyDelayS,
	}, nil
}

// RetryDueJobs is a no-op on the repository service. The sweeper is implemented
// directly on schedulerService, which never routes through the REST decorator
// chain. This satisfies the Service interface so the REST + validation
// decorators compile without adding sweeper logic to the HTTP path.
func (s *repositoryService) RetryDueJobs(_ context.Context) error {
	return nil
}

// EscalateStuckRetryJobs is a no-op on the repository service for the same
// reason as RetryDueJobs — the scheduler-only sweeper path is never wired
// through the REST decorator chain.
func (s *repositoryService) EscalateStuckRetryJobs(_ context.Context) (int64, error) {
	return 0, nil
}

// ReclaimStuckFetchRows is a no-op on the repository service for the same
// reason as RetryDueJobs/EscalateStuckRetryJobs — the scheduler-only reclaimer
// path is never wired through the REST decorator chain.
func (s *repositoryService) ReclaimStuckFetchRows(_ context.Context) (int64, error) {
	return 0, nil
}

// AutoPauseChronicFailures is a no-op on the repository service for the same
// reason as the other scheduler-only sweeps — the auto-pause path is never
// wired through the REST decorator chain.
func (s *repositoryService) AutoPauseChronicFailures(_ context.Context) (int, error) {
	return 0, nil
}

// RefreshGaugeMetrics is a no-op on the repository service for the same
// reason as the other scheduler-only sweeps — the gauge-refresh path is
// never wired through the REST decorator chain.
func (s *repositoryService) RefreshGaugeMetrics(_ context.Context) error {
	return nil
}

func (s *repositoryService) UpdateScanIntervalOverride(ctx context.Context, companyID string, override *int) (err error) {
	ctx, span := tracing.Start(ctx, "scan.repository.UpdateScanIntervalOverride")
	defer tracing.End(span, &err)
	result := s.db.WithContext(ctx).Model(&companymodel.CompanyScanSettings{}).
		Where("company_id = ?", companyID).
		Update("scan_interval_override", override)
	if result.Error != nil {
		return errors.ErrInternal.WithMessage("failed to update scan interval override")
	}
	if result.RowsAffected == 0 {
		return errors.ErrNotFound.WithMessage("company has no scan settings configured")
	}
	return nil
}
