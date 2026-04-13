package store

import (
	"time"

	"github.com/trushilpatel/jobradar/model"
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
	UpdateSeenReasonBatch(jobIDs []string, reason string) error
	MarkRemovedBatch(jobIDs []string) error
}

// SeenLedgerCompanyReader provides company-scoped reads on the ledger.
// Used by adapters that need to detect removed jobs (e.g. Google).
type SeenLedgerCompanyReader interface {
	GetActiveJobIDsByCompany(companyID string) ([]string, error)
}

// SeenLedgerStore composes all interfaces for workers that need full access (e.g. scan engine).
type SeenLedgerStore interface {
	SeenLedgerReader
	SeenLedgerWriter
	SeenLedgerCompanyReader
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

func (s *seenLedgerStore) UpdateSeenReasonBatch(jobIDs []string, reason string) error {
	if len(jobIDs) == 0 {
		return nil
	}
	return s.db.Model(&model.JobSeenLedger{}).
		Where("job_id IN ?", jobIDs).
		Update("reason", reason).Error
}

// MarkRemovedBatch sets removed_at = NOW() for the given job IDs.
// Used to track jobs that have disappeared from a company's careers page.
func (s *seenLedgerStore) MarkRemovedBatch(jobIDs []string) error {
	if len(jobIDs) == 0 {
		return nil
	}
	now := time.Now()
	return s.db.Model(&model.JobSeenLedger{}).
		Where("job_id IN ?", jobIDs).
		Where("removed_at IS NULL").
		Update("removed_at", now).Error
}

// GetActiveJobIDsByCompany returns all job IDs for a company that have NOT been
// marked as removed (removed_at IS NULL). Used to diff against current scan
// results for detecting removed jobs.
func (s *seenLedgerStore) GetActiveJobIDsByCompany(companyID string) ([]string, error) {
	var ids []string
	err := s.db.Model(&model.JobSeenLedger{}).
		Where("company_id = ? AND removed_at IS NULL", companyID).
		Pluck("job_id", &ids).Error
	return ids, err
}
