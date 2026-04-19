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
