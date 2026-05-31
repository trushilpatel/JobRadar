package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
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

	// UpdateHeartbeat refreshes the heartbeat timestamp for an in-progress row.
	// Returns ErrQueueRowMissing when no row matched — signals the row was
	// reassigned by ResetStuck and the crawler must abort the scan.
	UpdateHeartbeat(ctx context.Context, id string) error

	// ListQueue returns queue entries matching filter, ordered by priority DESC, queued_at ASC.
	ListQueue(ctx context.Context, filter dto.QueueFilter) ([]dto.ScanQueueItem, error)
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
	var ids []string
	if err = s.db.WithContext(ctx).Raw(`
		INSERT INTO company_scan_queue (company_id, source, adapter_type)
		SELECT s.company_id, ?, s.adapter_type
		FROM company_scan_settings s
		LEFT JOIN adapter_configs ac ON ac.adapter_type = s.adapter_type
		WHERE s.active = true
		  AND NOT EXISTS (
		    SELECT 1 FROM company_scan_queue q
		    WHERE q.company_id = s.company_id AND q.status IN ('queued','running')
		  )
		  AND (
		    s.last_scan_at IS NULL
		    OR s.last_scan_at + COALESCE(
		        s.scan_interval_override,
		        ac.scan_interval_minutes,
		        ?
		    ) * INTERVAL '1 minute' <= NOW()
		  )
		ORDER BY s.last_scan_at ASC NULLS FIRST
		ON CONFLICT DO NOTHING
		RETURNING company_id
	`, source, intervalMinutes).Scan(&ids).Error; err != nil {
		return EnqueueDueResult{}, err
	}

	// Count companies that exist but have no scan_settings row — surfaced
	// once per scheduler tick so admins know they still need to onboard them.
	var skipped int64
	if err = s.db.WithContext(ctx).Raw(`
		SELECT COUNT(*)
		FROM companies c
		LEFT JOIN company_scan_settings s ON s.company_id = c.id
		WHERE s.company_id IS NULL
	`).Scan(&skipped).Error; err != nil {
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
			FROM adapter_configs
			WHERE adapter_type = ?
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
		return tx.Exec(`UPDATE adapter_configs SET last_scheduled_at = NOW() WHERE adapter_type = ?`, adapterType).Error
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
	// active=false OR the scan_settings row is absent entirely.
	result := s.db.WithContext(ctx).Exec(`
		DELETE FROM company_scan_queue
		WHERE status = 'queued'
		  AND company_id IN (
		    SELECT c.id
		    FROM companies c
		    LEFT JOIN company_scan_settings s ON s.company_id = c.id
		    WHERE s.company_id IS NULL OR s.active = false
		  )
	`)
	return int(result.RowsAffected), result.Error
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
	res := s.db.WithContext(ctx).Exec(`
		UPDATE company_scan_queue
		SET status = 'queued', started_at = NULL, heartbeat_at = NULL
		WHERE status = 'running'
		  AND COALESCE(heartbeat_at, started_at) < NOW() - INTERVAL '1 second' * ?
	`, int(staleDur.Seconds()))
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
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

// --- repositoryService: terminal Service implementation ---

type repositoryService struct {
	queue   ScanQueueStore
	scanLog ScanLogReader
	config  Config
}

// NewRepositoryService constructs the terminal Service implementation backed by DB stores.
func NewRepositoryService(queue ScanQueueStore, scanLog ScanLogReader, cfg Config) Service {
	return &repositoryService{queue: queue, scanLog: scanLog, config: cfg}
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
