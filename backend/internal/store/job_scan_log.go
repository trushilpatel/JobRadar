package store

import (
	"github.com/trushilpatel/jobradar/internal/model"
	"gorm.io/gorm"
)

// JobScanLogWriter records scan summaries and updates company scan status.
type JobScanLogWriter interface {
	InsertJobScanLog(companyID string, jobsFetched, jobsNew, durationMs int, errMsg *string) error
	UpdateCompanyScanStatus(companyID, status string) error
}

type jobScanLogStore struct {
	db *gorm.DB
}

func NewJobScanLogStore(db *gorm.DB) JobScanLogWriter {
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

func (s *jobScanLogStore) UpdateCompanyScanStatus(companyID, status string) error {
	return s.db.Model(&model.Company{}).
		Where("id = ?", companyID).
		Updates(map[string]any{
			"last_scan_at":     gorm.Expr("NOW()"),
			"last_scan_status": status,
		}).Error
}
