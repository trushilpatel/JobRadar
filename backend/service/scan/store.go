package scan

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
)

// ScanQueueStore manages the company scan work queue.
type ScanQueueStore interface {
	// Enqueue inserts a company into the queue. Silently no-ops if the company
	// already has an active entry (queued or processing).
	Enqueue(companyID, source string) error

	// ClaimNext atomically claims the next queued item and marks it processing.
	// Returns nil, nil if the queue is empty.
	ClaimNext(ctx context.Context) (*ScanQueueItem, error)

	// Complete removes the queue entry. Call after scan succeeds or fails.
	Complete(id string) error

	// Remove deletes a queued entry by ID. No-ops if entry is processing or missing.
	Remove(id string) error

	// MoveUp raises this item one position in the queue by increasing its priority.
	MoveUp(id string) error

	// MoveDown lowers this item one position in the queue.
	MoveDown(id string) error

	// ResetStuck resets processing rows that started more than staleDur ago.
	ResetStuck(staleDur time.Duration) error

	// ListQueue returns all active queue entries ordered by priority DESC, queued_at ASC.
	ListQueue() ([]ScanQueueItem, error)
}

// ScanLogWriter records scan summaries and updates company scan status.
type ScanLogWriter interface {
	InsertCompanyScanLog(companyID string, jobsFetched, jobsNew, durationMs int, errMsg *string) error
	UpdateCompanyScanStatus(companyID, status string) error
}

// ScanLogReader reads scan history for a company.
type ScanLogReader interface {
	GetScanStats(companyID string) (ScanStatsView, error)
	GetJobsScannedToday() (int64, error)
	GetScansByDay(days int) ([]DailyScanCount, error)
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

func (s *scanQueueStore) Enqueue(companyID, source string) error {
	return s.db.Exec(`
		INSERT INTO company_scan_queue (company_id, source)
		VALUES (?, ?)
		ON CONFLICT DO NOTHING
	`, companyID, source).Error
}

func (s *scanQueueStore) ClaimNext(ctx context.Context) (*ScanQueueItem, error) {
	var item ScanQueueItem
	err := s.db.WithContext(ctx).Raw(`
		UPDATE company_scan_queue
		SET status = 'processing', started_at = NOW()
		WHERE id = (
			SELECT id FROM company_scan_queue
			WHERE status = 'queued'
			ORDER BY priority DESC, queued_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, company_id, status, source, priority, queued_at, started_at
	`).Scan(&item).Error
	if err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func (s *scanQueueStore) Complete(id string) error {
	return s.db.Exec(`DELETE FROM company_scan_queue WHERE id = ?`, id).Error
}

func (s *scanQueueStore) Remove(id string) error {
	return s.db.Exec(`DELETE FROM company_scan_queue WHERE id = ? AND status = 'queued'`, id).Error
}

func (s *scanQueueStore) MoveUp(id string) error {
	items, err := s.listOrdered()
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
	return s.db.Exec(`UPDATE company_scan_queue SET priority = ? WHERE id = ?`, newPriority, id).Error
}

func (s *scanQueueStore) MoveDown(id string) error {
	items, err := s.listOrdered()
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
	this := items[pos]
	below := items[pos+1]
	newPriority := this.Priority + 1
	return s.db.Exec(`UPDATE company_scan_queue SET priority = ? WHERE id = ?`, newPriority, below.ID).Error
}

func (s *scanQueueStore) ResetStuck(staleDur time.Duration) error {
	return s.db.Exec(`
		UPDATE company_scan_queue
		SET status = 'queued', started_at = NULL
		WHERE status = 'processing'
		  AND started_at < NOW() - INTERVAL '1 second' * ?
	`, int(staleDur.Seconds())).Error
}

func (s *scanQueueStore) ListQueue() ([]ScanQueueItem, error) {
	return s.listOrdered()
}

func (s *scanQueueStore) listOrdered() ([]ScanQueueItem, error) {
	var items []ScanQueueItem
	err := s.db.Raw(`
		SELECT id, company_id, status, source, priority, queued_at, started_at
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

func (s *scanLogStore) InsertCompanyScanLog(companyID string, jobsFetched, jobsNew, durationMs int, errMsg *string) error {
	row := model.CompanyScanLog{
		CompanyID:   companyID,
		JobsFetched: jobsFetched,
		JobsNew:     jobsNew,
		DurationMs:  &durationMs,
		Error:       errMsg,
	}
	return s.db.Create(&row).Error
}

func (s *scanLogStore) GetScanStats(companyID string) (ScanStatsView, error) {
	var stats ScanStatsView

	type agg struct {
		TotalScans    int64
		TotalFetched  int64
		TotalNew      int64
		ErrorCount    int64
		AvgDurationMs *float64
	}
	var a agg
	err := s.db.Model(&model.CompanyScanLog{}).
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
	s.db.Model(&model.CompanyScanLog{}).
		Where("company_id = ? AND DATE(scanned_at) = CURRENT_DATE", companyID).
		Select("COALESCE(SUM(jobs_new), 0)").Scan(&newToday)
	s.db.Model(&model.CompanyScanLog{}).
		Where("company_id = ? AND DATE_TRUNC('month', scanned_at) = DATE_TRUNC('month', CURRENT_DATE)", companyID).
		Select("COALESCE(SUM(jobs_new), 0)").Scan(&newThisMonth)
	stats.NewToday = newToday
	stats.NewThisMonth = newThisMonth

	var recent []ScanRun
	if err := s.db.Model(&model.CompanyScanLog{}).
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

func (s *scanLogStore) GetJobsScannedToday() (int64, error) {
	var count int64
	err := s.db.Model(&model.CompanyScanLog{}).
		Where("DATE(scanned_at) = CURRENT_DATE").
		Select("COALESCE(SUM(jobs_fetched), 0)").
		Scan(&count).Error
	return count, err
}

func (s *scanLogStore) GetScansByDay(days int) ([]DailyScanCount, error) {
	var results []DailyScanCount
	err := s.db.Model(&model.CompanyScanLog{}).
		Select("DATE(scanned_at) as date, SUM(jobs_fetched) as scanned, SUM(jobs_new) as new_jobs").
		Where("scanned_at >= NOW() - ? * INTERVAL '1 day'", days).
		Group("DATE(scanned_at)").
		Order("date ASC").
		Scan(&results).Error
	return results, err
}

func (s *scanLogStore) UpdateCompanyScanStatus(companyID, status string) error {
	return s.db.Model(&model.Company{}).
		Where("id = ?", companyID).
		Updates(map[string]any{
			"last_scan_at":     gorm.Expr("NOW()"),
			"last_scan_status": status,
		}).Error
}
