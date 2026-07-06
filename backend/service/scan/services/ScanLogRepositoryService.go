package services

import (
	"context"
	"fmt"
	"time"

	"github.com/trushilpatel/jobradar/common/tracing"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	scanmodel "github.com/trushilpatel/jobradar/service/scan/models"
	"gorm.io/gorm"
)

// recentRunsLimit is the maximum number of recent scan runs returned per company.
const recentRunsLimit = 10

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
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("scan.store.InsertCompanyScanLog: %w", err)
	}
	return nil
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
		return stats, fmt.Errorf("scan.store.GetScanStats: aggregate: %w", err)
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
		Limit(recentRunsLimit).
		Select("company_id, scanned_at, jobs_fetched, jobs_new, error, duration_ms").
		Scan(&recent).Error; err != nil {
		return stats, fmt.Errorf("scan.store.GetScanStats: recent runs: %w", err)
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
	if txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
	}); txErr != nil {
		return fmt.Errorf("scan.store.RecordScanResult: %w", txErr)
	}
	return nil
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
		return nil, fmt.Errorf("scan.store.GetScanHistories: %w", err)
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
	if err = s.db.WithContext(ctx).Model(&scanmodel.CompanyScanLog{}).
		Where("DATE(scanned_at) = CURRENT_DATE").
		Select("COALESCE(SUM(jobs_fetched), 0)").
		Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("scan.store.GetJobsScannedToday: %w", err)
	}
	return count, nil
}

func (s *scanLogStore) GetScansByDay(ctx context.Context, days int) (results []dto.DailyScanCount, err error) {
	ctx, span := tracing.Start(ctx, "scan.store.GetScansByDay")
	defer tracing.End(span, &err)
	if err = s.db.WithContext(ctx).Model(&scanmodel.CompanyScanLog{}).
		Select("DATE(scanned_at) as date, SUM(jobs_fetched) as scanned, SUM(jobs_new) as new_jobs").
		Where("scanned_at >= NOW() - ? * INTERVAL '1 day'", days).
		Group("DATE(scanned_at)").
		Order("date ASC").
		Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("scan.store.GetScansByDay: %w", err)
	}
	return results, nil
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
		return dto.CompanyAnalytics{}, fmt.Errorf("scan.store.GetCompanyAnalytics: monthly new: %w", err)
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
		return dto.CompanyAnalytics{}, fmt.Errorf("scan.store.GetCompanyAnalytics: monthly removed: %w", err)
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
		return dto.CompanyAnalytics{}, fmt.Errorf("scan.store.GetCompanyAnalytics: velocity: %w", err)
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
