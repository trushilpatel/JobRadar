package store

import (
	"github.com/trushilpatel/jobradar/internal/model"
	"gorm.io/gorm"
)

// SeenLedgerReader checks if a job has already been processed.
type SeenLedgerReader interface {
	HasSeen(jobID string) (bool, error)
}

// SeenLedgerWriter marks jobs as seen or updates their reason.
type SeenLedgerWriter interface {
	MarkSeen(jobID, companyID, reason string) error
	UpdateSeenReason(jobID, reason string) error
}

// SeenLedgerStore composes both for workers that need full access (e.g. scan engine).
type SeenLedgerStore interface {
	SeenLedgerReader
	SeenLedgerWriter
}

type seenLedgerStore struct {
	db *gorm.DB
}

func NewSeenLedgerStore(db *gorm.DB) SeenLedgerStore {
	return &seenLedgerStore{db: db}
}

func (s *seenLedgerStore) HasSeen(jobID string) (bool, error) {
	var count int64
	err := s.db.Model(&model.JobSeenLedger{}).
		Where("job_id = ?", jobID).
		Count(&count).Error
	return count > 0, err
}

func (s *seenLedgerStore) MarkSeen(jobID, companyID, reason string) error {
	row := model.JobSeenLedger{
		JobID:     jobID,
		CompanyID: companyID,
		Reason:    reason,
	}
	return s.db.Create(&row).Error
}

func (s *seenLedgerStore) UpdateSeenReason(jobID, reason string) error {
	return s.db.Model(&model.JobSeenLedger{}).
		Where("job_id = ?", jobID).
		Update("reason", reason).Error
}
