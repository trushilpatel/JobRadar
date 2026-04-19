package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/infra/tracing"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SeenLedgerReader checks if a job has already been processed.
type SeenLedgerReader interface {
	HasSeen(ctx context.Context, jobID string) (bool, error)
	HasSeenBatch(ctx context.Context, jobIDs []string) (map[string]bool, error)
}

// SeenLedgerWriter marks jobs as seen or updates their reason.
type SeenLedgerWriter interface {
	MarkSeen(ctx context.Context, jobID, companyID, reason string) error
	MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) error
	UpdateSeenReason(ctx context.Context, jobID, reason string) error
	UpdateSeenReasonBatch(ctx context.Context, jobIDs []string, reason string) error
	MarkRemovedBatch(ctx context.Context, jobIDs []string) error
}

// SeenLedgerCompanyReader provides company-scoped reads on the ledger.
type SeenLedgerCompanyReader interface {
	GetActiveJobIDsByCompany(ctx context.Context, companyID string) ([]string, error)
}

// SeenLedgerStore composes all ledger interfaces.
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

func (s *seenLedgerStore) HasSeen(ctx context.Context, jobID string) (_ bool, err error) {
	ctx, span := tracing.Start(ctx, "job.store.HasSeen")
	defer tracing.End(span, &err)
	var count int64
	err = s.db.WithContext(ctx).Model(&jobmodel.JobSeenLedger{}).
		Where("job_id = ?", jobID).
		Count(&count).Error
	return count > 0, err
}

func (s *seenLedgerStore) HasSeenBatch(ctx context.Context, jobIDs []string) (_ map[string]bool, err error) {
	ctx, span := tracing.Start(ctx, "job.store.HasSeenBatch")
	defer tracing.End(span, &err)
	if len(jobIDs) == 0 {
		return map[string]bool{}, nil
	}
	var found []string
	err = s.db.WithContext(ctx).Model(&jobmodel.JobSeenLedger{}).
		Where("job_id IN ?", jobIDs).
		Pluck("job_id", &found).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(jobIDs))
	for _, id := range found {
		result[id] = true
	}
	return result, nil
}

func (s *seenLedgerStore) MarkSeen(ctx context.Context, jobID, companyID, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.MarkSeen")
	defer tracing.End(span, &err)
	row := jobmodel.JobSeenLedger{
		JobID:     jobID,
		CompanyID: companyID,
		Reason:    reason,
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *seenLedgerStore) MarkSeenBatch(ctx context.Context, jobIDs []string, companyID, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.MarkSeenBatch")
	defer tracing.End(span, &err)
	if len(jobIDs) == 0 {
		return nil
	}
	rows := make([]jobmodel.JobSeenLedger, len(jobIDs))
	for i, id := range jobIDs {
		rows[i] = jobmodel.JobSeenLedger{JobID: id, CompanyID: companyID, Reason: reason}
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(rows, 500).Error
}

func (s *seenLedgerStore) UpdateSeenReason(ctx context.Context, jobID, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSeenReason")
	defer tracing.End(span, &err)
	return s.db.WithContext(ctx).Model(&jobmodel.JobSeenLedger{}).
		Where("job_id = ?", jobID).
		Update("reason", reason).Error
}

func (s *seenLedgerStore) UpdateSeenReasonBatch(ctx context.Context, jobIDs []string, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSeenReasonBatch")
	defer tracing.End(span, &err)
	if len(jobIDs) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Model(&jobmodel.JobSeenLedger{}).
		Where("job_id IN ?", jobIDs).
		Update("reason", reason).Error
}

func (s *seenLedgerStore) MarkRemovedBatch(ctx context.Context, jobIDs []string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.MarkRemovedBatch")
	defer tracing.End(span, &err)
	if len(jobIDs) == 0 {
		return nil
	}
	now := time.Now()
	return s.db.WithContext(ctx).Model(&jobmodel.JobSeenLedger{}).
		Where("job_id IN ?", jobIDs).
		Where("removed_at IS NULL").
		Update("removed_at", now).Error
}

func (s *seenLedgerStore) GetActiveJobIDsByCompany(ctx context.Context, companyID string) (ids []string, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetActiveJobIDsByCompany")
	defer tracing.End(span, &err)
	err = s.db.WithContext(ctx).Model(&jobmodel.JobSeenLedger{}).
		Where("company_id = ? AND removed_at IS NULL", companyID).
		Pluck("job_id", &ids).Error
	return ids, err
}
