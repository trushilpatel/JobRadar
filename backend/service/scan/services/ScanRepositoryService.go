package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	scanmodel "github.com/trushilpatel/jobradar/service/scan/models"
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

// ScanLogWriter records scan summaries.
//
// Phase 2: per-company operational state lives on `company_scan_settings`,
// not `companies`. RecordScanResult writes the company_scan_log row and the
// scan_settings state update in one transaction. consecutive_errors is reset
// to 0 on success and incremented on error.
type ScanLogWriter interface {
	InsertCompanyScanLog(ctx context.Context, companyID string, jobsFetched, jobsNew, durationMs int, errMsg *string) error
	// RecordScanResult atomically writes the scan log row and updates the
	// company's `company_scan_settings` row (last_scan_at + last_success_at /
	// last_error_at + last_error_message + consecutive_errors).
	// status must be "ok" or "error".
	RecordScanResult(ctx context.Context, companyID, status string, jobsFetched, jobsNew, durationMs int, errMsg *string) error
}

// ScanLogReader reads scan history for a company.
type ScanLogReader interface {
	GetScanStats(ctx context.Context, companyID string) (dto.ScanStatsView, error)
	// GetScanHistories returns the last ≤10 scan outcomes for each company in companyIDs,
	// ordered oldest→newest. true=success, false=error.
	GetScanHistories(ctx context.Context, companyIDs []string) (map[string][]bool, error)
	GetJobsScannedToday(ctx context.Context) (int64, error)
	GetScansByDay(ctx context.Context, days int) ([]dto.DailyScanCount, error)
	GetCompanyAnalytics(ctx context.Context, companyID string) (dto.CompanyAnalytics, error)
}

// ScanLogStore is the full interface for scan log read+write.
type ScanLogStore interface {
	ScanLogWriter
	ScanLogReader
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

// --- Scan log store ---

type scanLogStore struct {
	db *gorm.DB
}

// NewLogStore constructs a ScanLogStore implementation.
func NewLogStore(db *gorm.DB) ScanLogStore {
	return &scanLogStore{db: db}
}

func (s *scanLogStore) InsertCompanyScanLog(ctx context.Context, companyID string, jobsFetched, jobsNew, durationMs int, errMsg *string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.store.InsertCompanyScanLog")
	defer tracing.End(span, &err)
	row := scanmodel.CompanyScanLog{
		CompanyID:   companyID,
		JobsFetched: jobsFetched,
		JobsNew:     jobsNew,
		DurationMs:  &durationMs,
		Error:       errMsg,
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *scanLogStore) GetScanStats(ctx context.Context, companyID string) (_ dto.ScanStatsView, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.GetScanStats")
	defer tracing.End(span, &err)

	var stats dto.ScanStatsView

	// Single aggregation query collapses the previous 3 separate queries into one
	// using conditional aggregation — avoids 3 round-trips for a single stats card.
	type agg struct {
		TotalScans    int64
		TotalFetched  int64
		TotalNew      int64
		ErrorCount    int64
		AvgDurationMs *float64
		NewToday      int64
		NewThisMonth  int64
	}
	var a agg
	if err := s.db.WithContext(ctx).Raw(`
		SELECT
		    COUNT(*)                                                                                              AS total_scans,
		    COALESCE(SUM(jobs_fetched), 0)                                                                       AS total_fetched,
		    COALESCE(SUM(jobs_new), 0)                                                                           AS total_new,
		    COUNT(*) FILTER (WHERE error IS NOT NULL)                                                            AS error_count,
		    COALESCE(AVG(duration_ms), 0)                                                                        AS avg_duration_ms,
		    COALESCE(SUM(jobs_new) FILTER (WHERE DATE(scanned_at) = CURRENT_DATE), 0)                           AS new_today,
		    COALESCE(SUM(jobs_new) FILTER (WHERE DATE_TRUNC('month', scanned_at) = DATE_TRUNC('month', CURRENT_DATE)), 0) AS new_this_month
		FROM company_scan_log
		WHERE company_id = ?
	`, companyID).Scan(&a).Error; err != nil {
		return stats, err
	}
	stats.TotalScans = a.TotalScans
	stats.TotalFetched = a.TotalFetched
	stats.TotalNew = a.TotalNew
	stats.ErrorCount = a.ErrorCount
	stats.AvgDurationMs = a.AvgDurationMs
	stats.NewToday = a.NewToday
	stats.NewThisMonth = a.NewThisMonth

	// Recent runs must remain a separate query — it returns multiple rows.
	var recent []dto.ScanRun
	if err := s.db.WithContext(ctx).Model(&scanmodel.CompanyScanLog{}).
		Where("company_id = ?", companyID).
		Order("scanned_at DESC").
		Limit(10).
		Select("company_id, scanned_at, jobs_fetched, jobs_new, error, duration_ms").
		Scan(&recent).Error; err != nil {
		return stats, err
	}
	stats.RecentRuns = recent

	return stats, nil
}

func (s *scanLogStore) RecordScanResult(ctx context.Context, companyID, status string, jobsFetched, jobsNew, durationMs int, errMsg *string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.store.RecordScanResult")
	defer tracing.End(span, &err)
	// Phase 2: write the scan log row AND patch company_scan_settings (the
	// new home for last_scan_at / last_success_at / last_error_at /
	// last_error_message / consecutive_errors) atomically.
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := scanmodel.CompanyScanLog{
			CompanyID:   companyID,
			JobsFetched: jobsFetched,
			JobsNew:     jobsNew,
			DurationMs:  &durationMs,
			Error:       errMsg,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}

		fields := map[string]any{
			"last_scan_at": gorm.Expr("NOW()"),
		}
		if status == scancommon.ScanStatusOK {
			fields["last_success_at"] = gorm.Expr("NOW()")
			fields["consecutive_errors"] = 0
			fields["last_error_message"] = nil
		} else {
			fields["last_error_at"] = gorm.Expr("NOW()")
			if errMsg != nil {
				fields["last_error_message"] = *errMsg
			}
			fields["consecutive_errors"] = gorm.Expr("consecutive_errors + 1")
		}

		// If no scan_settings row exists (worker should have skipped already)
		// the UPDATE is a no-op — never an error. The companyScanLog row is
		// still committed so the operational history is preserved.
		return tx.Model(&companymodel.CompanyScanSettings{}).
			Where("company_id = ?", companyID).
			Updates(fields).Error
	})
}

func (s *scanLogStore) GetScanHistories(ctx context.Context, companyIDs []string) (_ map[string][]bool, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.GetScanHistories")
	defer tracing.End(span, &err)

	if len(companyIDs) == 0 {
		return map[string][]bool{}, nil
	}

	type row struct {
		CompanyID string
		Success   bool
	}

	var rows []row
	err = s.db.WithContext(ctx).Raw(`
		SELECT company_id, (error IS NULL) AS success
		FROM (
			SELECT company_id, error,
			       ROW_NUMBER() OVER (PARTITION BY company_id ORDER BY scanned_at DESC) AS rn
			FROM company_scan_log
			WHERE company_id IN ?
		) ranked
		WHERE rn <= 10
		ORDER BY company_id, rn DESC
	`, companyIDs).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string][]bool, len(companyIDs))
	for _, r := range rows {
		result[r.CompanyID] = append(result[r.CompanyID], r.Success)
	}
	return result, nil
}

func (s *scanLogStore) GetJobsScannedToday(ctx context.Context) (_ int64, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.GetJobsScannedToday")
	defer tracing.End(span, &err)
	var count int64
	err = s.db.WithContext(ctx).Model(&scanmodel.CompanyScanLog{}).
		Where("DATE(scanned_at) = CURRENT_DATE").
		Select("COALESCE(SUM(jobs_fetched), 0)").
		Scan(&count).Error
	return count, err
}

func (s *scanLogStore) GetScansByDay(ctx context.Context, days int) (results []dto.DailyScanCount, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.GetScansByDay")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Model(&scanmodel.CompanyScanLog{}).
		Select("DATE(scanned_at) as date, SUM(jobs_fetched) as scanned, SUM(jobs_new) as new_jobs").
		Where("scanned_at >= NOW() - ? * INTERVAL '1 day'", days).
		Group("DATE(scanned_at)").
		Order("date ASC").
		Scan(&results).Error
	return results, err
}

func (s *scanLogStore) GetCompanyAnalytics(ctx context.Context, companyID string) (_ dto.CompanyAnalytics, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.GetCompanyAnalytics")
	defer tracing.End(span, &err)

	// Monthly new jobs from job_analytics
	var newRows []struct {
		MonthStart time.Time
		Month      string
		Count      int
	}
	if err = s.db.WithContext(ctx).Raw(`
		SELECT
			DATE_TRUNC('month', snapshotted_at)                                  AS month_start,
			TO_CHAR(DATE_TRUNC('month', snapshotted_at), 'Mon YYYY')             AS month,
			COUNT(*)                                                              AS count
		FROM job_analytics
		WHERE company_id = ?
		  AND snapshotted_at >= DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '11 months'
		GROUP BY DATE_TRUNC('month', snapshotted_at)
		ORDER BY month_start
	`, companyID).Scan(&newRows).Error; err != nil {
		return dto.CompanyAnalytics{}, err
	}

	// Monthly removed jobs from job_pipeline_ledger
	var removedRows []struct {
		MonthStart time.Time
		Month      string
		Count      int
	}
	if err = s.db.WithContext(ctx).Raw(`
		SELECT
			DATE_TRUNC('month', removed_at)                                      AS month_start,
			TO_CHAR(DATE_TRUNC('month', removed_at), 'Mon YYYY')                 AS month,
			COUNT(*)                                                              AS count
		FROM job_pipeline_ledger
		WHERE company_id = ?
		  AND removed_at IS NOT NULL
		  AND removed_at >= DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '11 months'
		GROUP BY DATE_TRUNC('month', removed_at)
		ORDER BY month_start
	`, companyID).Scan(&removedRows).Error; err != nil {
		return dto.CompanyAnalytics{}, err
	}

	// Velocity: this week vs last week (from job_analytics)
	var vel struct {
		ThisWeek int
		LastWeek int
	}
	if err = s.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(*) FILTER (WHERE snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE))                                                             AS this_week,
			COUNT(*) FILTER (WHERE snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE) - INTERVAL '7 days'
			                   AND snapshotted_at  < DATE_TRUNC('week', CURRENT_DATE))                                                             AS last_week
		FROM job_analytics
		WHERE company_id = ?
		  AND snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE) - INTERVAL '7 days'
	`, companyID).Scan(&vel).Error; err != nil {
		return dto.CompanyAnalytics{}, err
	}

	// Build ordered month index covering all 12 months
	type monthKey struct {
		start time.Time
		label string
	}
	now := time.Now()
	months := make([]monthKey, 12)
	for i := 11; i >= 0; i-- {
		t := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		months[11-i] = monthKey{
			start: time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC),
			label: t.Format("Jan 2006"),
		}
	}

	newByMonth := make(map[string]int, len(newRows))
	for _, r := range newRows {
		newByMonth[r.Month] = r.Count
	}
	removedByMonth := make(map[string]int, len(removedRows))
	for _, r := range removedRows {
		removedByMonth[r.Month] = r.Count
	}

	points := make([]dto.MonthlyJobPoint, 0, 12)
	for _, m := range months {
		points = append(points, dto.MonthlyJobPoint{
			Month:   m.label,
			New:     newByMonth[m.label],
			Removed: removedByMonth[m.label],
		})
	}

	return dto.CompanyAnalytics{
		Monthly:  points,
		Velocity: dto.CompanyVelocity{ThisWeek: vel.ThisWeek, LastWeek: vel.LastWeek},
	}, nil
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

func (s *repositoryService) ResetStuck(ctx context.Context, staleDur time.Duration) (err error) {
	ctx, span := tracing.Start(ctx, "scan.repository.ResetStuck")
	defer tracing.End(span, &err)
	if _, err := s.queue.ResetStuck(ctx, staleDur); err != nil {
		return errors.ErrInternal.WithMessage("failed to reset stuck queue entries")
	}
	return nil
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
