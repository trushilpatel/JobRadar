package store

import (
	"github.com/trushilpatel/jobradar/internal/model"
	"gorm.io/gorm"
)

// ScanStats is a read-only summary of a company's scan history.
type ScanStats struct {
	TotalScans    int64    `json:"total_scans"`
	TotalFetched  int64    `json:"total_fetched"`
	TotalNew      int64    `json:"total_new"`
	ErrorCount    int64    `json:"error_count"`
	AvgDurationMs *float64 `json:"avg_duration_ms"`
	NewToday      int64    `json:"new_today"`
	NewThisMonth  int64    `json:"new_this_month"`
	RecentRuns    []model.JobScanLog `json:"recent_runs"`
}

// JobScanLogWriter records scan summaries and updates company scan status.
type JobScanLogWriter interface {
	InsertJobScanLog(companyID string, jobsFetched, jobsNew, durationMs int, errMsg *string) error
	UpdateCompanyScanStatus(companyID, status string) error
}

// DailyScanCount holds aggregated scan totals for a given date.
type DailyScanCount struct {
	Date    string `json:"date"`
	Scanned int    `json:"scanned"`
	NewJobs int    `json:"new_jobs"`
}

// JobScanLogReader reads scan history for a company.
type JobScanLogReader interface {
	GetScanStats(companyID string) (ScanStats, error)
	GetJobsScannedToday() (int64, error)
	GetScansByDay(days int) ([]DailyScanCount, error)
}

type jobScanLogStore struct {
	db *gorm.DB
}

// JobScanLogStore is the full interface used by anything that reads + writes scan logs.
type JobScanLogStore interface {
	JobScanLogWriter
	JobScanLogReader
}

func NewJobScanLogStore(db *gorm.DB) JobScanLogStore {
	return &jobScanLogStore{db: db}
}

func (s *jobScanLogStore) InsertJobScanLog(companyID string, jobsFetched, jobsNew, durationMs int, errMsg *string) error {
	row := model.JobScanLog{
		CompanyID:   companyID,
		JobsFetched: jobsFetched,
		JobsNew:     jobsNew,
		DurationMs:  &durationMs,
		Error:       errMsg,
	}
	return s.db.Create(&row).Error
}

func (s *jobScanLogStore) GetScanStats(companyID string) (ScanStats, error) {
	var stats ScanStats

	type agg struct {
		TotalScans    int64
		TotalFetched  int64
		TotalNew      int64
		ErrorCount    int64
		AvgDurationMs *float64
	}
	var a agg
	err := s.db.Model(&model.JobScanLog{}).
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
	s.db.Model(&model.JobScanLog{}).
		Where("company_id = ? AND DATE(scanned_at) = CURRENT_DATE", companyID).
		Select("COALESCE(SUM(jobs_new), 0)").Scan(&newToday)
	s.db.Model(&model.JobScanLog{}).
		Where("company_id = ? AND DATE_TRUNC('month', scanned_at) = DATE_TRUNC('month', CURRENT_DATE)", companyID).
		Select("COALESCE(SUM(jobs_new), 0)").Scan(&newThisMonth)
	stats.NewToday = newToday
	stats.NewThisMonth = newThisMonth

	err = s.db.Where("company_id = ?", companyID).
		Order("scanned_at DESC").
		Limit(20).
		Find(&stats.RecentRuns).Error
	return stats, err
}

func (s *jobScanLogStore) GetJobsScannedToday() (int64, error) {
	var count int64
	err := s.db.Model(&model.JobScanLog{}).
		Where("DATE(scanned_at) = CURRENT_DATE").
		Select("COALESCE(SUM(jobs_fetched), 0)").
		Scan(&count).Error
	return count, err
}

func (s *jobScanLogStore) GetScansByDay(days int) ([]DailyScanCount, error) {
	var results []DailyScanCount
	err := s.db.Model(&model.JobScanLog{}).
		Select("DATE(scanned_at) as date, SUM(jobs_fetched) as scanned, SUM(jobs_new) as new_jobs").
		Where("scanned_at >= NOW() - ? * INTERVAL '1 day'", days).
		Group("DATE(scanned_at)").
		Order("date ASC").
		Scan(&results).Error
	return results, err
}

func (s *jobScanLogStore) UpdateCompanyScanStatus(companyID, status string) error {
	return s.db.Model(&model.Company{}).
		Where("id = ?", companyID).
		Updates(map[string]any{
			"last_scan_at":     gorm.Expr("NOW()"),
			"last_scan_status": status,
		}).Error
}
