package scan

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
)

// ScanQueueStore manages the company scan work queue.
type ScanQueueStore interface {
	// Enqueue inserts a company into the queue. Silently no-ops if the company
	// already has an active entry (queued or processing).
	Enqueue(ctx context.Context, companyID, source string) error

	// ClaimNext atomically claims the next queued item and marks it processing.
	// Returns nil, nil if the queue is empty.
	ClaimNext(ctx context.Context) (*ScanQueueItem, error)

	// Complete removes the queue entry. Call after scan succeeds or fails.
	Complete(ctx context.Context, id string) error

	// Remove deletes a queued entry by ID. No-ops if entry is processing or missing.
	Remove(ctx context.Context, id string) error

	// MoveUp raises this item one position in the queue by increasing its priority.
	MoveUp(ctx context.Context, id string) error

	// MoveDown lowers this item one position in the queue.
	MoveDown(ctx context.Context, id string) error

	// ResetStuck resets processing rows whose heartbeat (or started_at if no heartbeat yet) is older than staleDur.
	ResetStuck(ctx context.Context, staleDur time.Duration) error

	// UpdateHeartbeat refreshes the heartbeat timestamp for an in-progress row.
	UpdateHeartbeat(ctx context.Context, id string) error

	// ListQueue returns all active queue entries ordered by priority DESC, queued_at ASC.
	ListQueue(ctx context.Context) ([]ScanQueueItem, error)
}

// ScanLogWriter records scan summaries.
type ScanLogWriter interface {
	InsertCompanyScanLog(ctx context.Context, companyID string, jobsFetched, jobsNew, durationMs int, errMsg *string) error
	// RecordScanResult atomically writes the scan log row and updates the company's
	// last_scan_at / last_scan_status in a single transaction.
	// status must be "ok" or "error".
	RecordScanResult(ctx context.Context, companyID, status string, jobsFetched, jobsNew, durationMs int, errMsg *string) error
}

// ScanLogReader reads scan history for a company.
type ScanLogReader interface {
	GetScanStats(ctx context.Context, companyID string) (ScanStatsView, error)
	// GetScanHistories returns the last ≤10 scan outcomes for each company in companyIDs,
	// ordered oldest→newest. true=success, false=error.
	GetScanHistories(ctx context.Context, companyIDs []string) (map[string][]bool, error)
	GetJobsScannedToday(ctx context.Context) (int64, error)
	GetScansByDay(ctx context.Context, days int) ([]DailyScanCount, error)
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
	return s.db.WithContext(ctx).Exec(`
		INSERT INTO company_scan_queue (company_id, source)
		VALUES (?, ?)
		ON CONFLICT DO NOTHING
	`, companyID, source).Error
}

func (s *scanQueueStore) ClaimNext(ctx context.Context) (_ *ScanQueueItem, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.ClaimNext")
	defer tracing.End(span, &err)
	var item ScanQueueItem
	err = s.db.WithContext(ctx).Raw(`
		UPDATE company_scan_queue
		SET status = 'processing', started_at = NOW()
		WHERE id = (
			SELECT id FROM company_scan_queue
			WHERE status = 'queued'
			ORDER BY priority DESC, queued_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, company_id, status, source, priority, queued_at, started_at, heartbeat_at
	`).Scan(&item).Error
	if err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func (s *scanQueueStore) Complete(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.store.Complete")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Exec(`DELETE FROM company_scan_queue WHERE id = ?`, id).Error
}

func (s *scanQueueStore) Remove(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.store.Remove")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Exec(`DELETE FROM company_scan_queue WHERE id = ? AND status = 'queued'`, id).Error
}

func (s *scanQueueStore) MoveUp(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.store.MoveUp")
	defer tracing.End(span, &err)
	items, err := s.listOrdered(ctx)
	if err != nil {
		return err
	}
	pos := -1
	for i, item := range items {
		if item.ID == id {
			pos = i
			break
		}
	}
	if pos <= 0 {
		return nil
	}
	above := items[pos-1]
	newPriority := above.Priority + 1
	return s.db.WithContext(ctx).Exec(`UPDATE company_scan_queue SET priority = ? WHERE id = ?`, newPriority, id).Error
}

func (s *scanQueueStore) MoveDown(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.store.MoveDown")
	defer tracing.End(span, &err)
	items, err := s.listOrdered(ctx)
	if err != nil {
		return err
	}
	pos := -1
	for i, item := range items {
		if item.ID == id {
			pos = i
			break
		}
	}
	if pos < 0 || pos >= len(items)-1 {
		return nil
	}
	below := items[pos+1]
	newPriority := below.Priority + 1
	return s.db.WithContext(ctx).Exec(`UPDATE company_scan_queue SET priority = ? WHERE id = ?`, newPriority, below.ID).Error
}

func (s *scanQueueStore) ResetStuck(ctx context.Context, staleDur time.Duration) (err error) {
	ctx, span := tracing.Start(ctx, "scan.store.ResetStuck")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Exec(`
		UPDATE company_scan_queue
		SET status = 'queued', started_at = NULL, heartbeat_at = NULL
		WHERE status = 'processing'
		  AND COALESCE(heartbeat_at, started_at) < NOW() - INTERVAL '1 second' * ?
	`, int(staleDur.Seconds())).Error
}

func (s *scanQueueStore) UpdateHeartbeat(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.store.UpdateHeartbeat")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Exec(`UPDATE company_scan_queue SET heartbeat_at = NOW() WHERE id = ?`, id).Error
}

func (s *scanQueueStore) ListQueue(ctx context.Context) (_ []ScanQueueItem, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.ListQueue")
	defer tracing.End(span, &err)
	return s.listOrdered(ctx)
}

func (s *scanQueueStore) listOrdered(ctx context.Context) ([]ScanQueueItem, error) {
	var items []ScanQueueItem
	err := s.db.WithContext(ctx).Raw(`
		SELECT id, company_id, status, source, priority, queued_at, started_at, heartbeat_at
		FROM company_scan_queue
		ORDER BY priority DESC, queued_at ASC
	`).Scan(&items).Error
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
	row := model.CompanyScanLog{
		CompanyID:   companyID,
		JobsFetched: jobsFetched,
		JobsNew:     jobsNew,
		DurationMs:  &durationMs,
		Error:       errMsg,
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *scanLogStore) GetScanStats(ctx context.Context, companyID string) (_ ScanStatsView, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.GetScanStats")
	defer tracing.End(span, &err)
	var stats ScanStatsView

	type agg struct {
		TotalScans    int64
		TotalFetched  int64
		TotalNew      int64
		ErrorCount    int64
		AvgDurationMs *float64
	}
	var a agg
	err = s.db.WithContext(ctx).Model(&model.CompanyScanLog{}).
		Where("company_id = ?", companyID).
		Select("COUNT(*) as total_scans, SUM(jobs_fetched) as total_fetched, SUM(jobs_new) as total_new, SUM(CASE WHEN error IS NOT NULL THEN 1 ELSE 0 END) as error_count, AVG(duration_ms) as avg_duration_ms").
		Scan(&a).Error
	if err != nil {
		return stats, err
	}

	stats.TotalScans = a.TotalScans
	stats.TotalFetched = a.TotalFetched
	stats.TotalNew = a.TotalNew
	stats.ErrorCount = a.ErrorCount
	stats.AvgDurationMs = a.AvgDurationMs

	var newToday, newThisMonth int64
	s.db.WithContext(ctx).Model(&model.CompanyScanLog{}).
		Where("company_id = ? AND DATE(scanned_at) = CURRENT_DATE", companyID).
		Select("COALESCE(SUM(jobs_new), 0)").Scan(&newToday)
	s.db.WithContext(ctx).Model(&model.CompanyScanLog{}).
		Where("company_id = ? AND DATE_TRUNC('month', scanned_at) = DATE_TRUNC('month', CURRENT_DATE)", companyID).
		Select("COALESCE(SUM(jobs_new), 0)").Scan(&newThisMonth)
	stats.NewToday = newToday
	stats.NewThisMonth = newThisMonth

	var recent []ScanRun
	if err := s.db.WithContext(ctx).Model(&model.CompanyScanLog{}).
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
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := model.CompanyScanLog{
			CompanyID:   companyID,
			JobsFetched: jobsFetched,
			JobsNew:     jobsNew,
			DurationMs:  &durationMs,
			Error:       errMsg,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return tx.Model(&model.Company{}).
			Where("id = ?", companyID).
			Updates(map[string]any{
				"last_scan_at":     gorm.Expr("NOW()"),
				"last_scan_status": status,
			}).Error
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
			WHERE company_id = ANY(?)
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
	err = s.db.WithContext(ctx).Model(&model.CompanyScanLog{}).
		Where("DATE(scanned_at) = CURRENT_DATE").
		Select("COALESCE(SUM(jobs_fetched), 0)").
		Scan(&count).Error
	return count, err
}

func (s *scanLogStore) GetScansByDay(ctx context.Context, days int) (results []DailyScanCount, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.GetScansByDay")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Model(&model.CompanyScanLog{}).
		Select("DATE(scanned_at) as date, SUM(jobs_fetched) as scanned, SUM(jobs_new) as new_jobs").
		Where("scanned_at >= NOW() - ? * INTERVAL '1 day'", days).
		Group("DATE(scanned_at)").
		Order("date ASC").
		Scan(&results).Error
	return results, err
}
