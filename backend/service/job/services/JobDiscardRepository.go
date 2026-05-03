package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/tracing"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/gorm"
)

// DiscardStore writes dismissed jobs to the job_dismissal_log table.
type DiscardStore interface {
	InsertDiscard(ctx context.Context, jobResultID, reason, note string) error
	InsertDiscardBatch(ctx context.Context, ids []string, reason string) error
}

type discardStore struct {
	db *gorm.DB
}

func NewDiscardStore(db *gorm.DB) DiscardStore {
	return &discardStore{db: db}
}

func (s *discardStore) InsertDiscard(ctx context.Context, jobResultID, reason, note string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertDiscard")
	defer tracing.End(span, &err)
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	row := jobmodel.Discarded{
		JobResultID: jobResultID,
		Reason:      reason,
		Note:        notePtr,
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *discardStore) InsertDiscardBatch(ctx context.Context, ids []string, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertDiscardBatch")
	defer tracing.End(span, &err)
	if len(ids) == 0 {
		return nil
	}
	rows := make([]jobmodel.Discarded, len(ids))
	for i, id := range ids {
		rows[i] = jobmodel.Discarded{JobResultID: id, Reason: reason}
	}
	return s.db.WithContext(ctx).CreateInBatches(rows, 500).Error
}
