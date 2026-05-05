package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/common/tracing"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// techTagStore is the GORM-backed implementation of TechTagWriter
// (interface declared in JobService.go).
type techTagStore struct {
	db *gorm.DB
}

func NewTechTagStore(db *gorm.DB) TechTagWriter {
	return &techTagStore{db: db}
}

func (s *techTagStore) InsertTechTags(ctx context.Context, jobID, companyID string, techs []string, taggedAt time.Time) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertTechTags")
	defer tracing.End(span, &err)
	if len(techs) == 0 {
		return nil
	}
	rows := make([]jobmodel.JobTechTag, 0, len(techs))
	for _, tech := range techs {
		rows = append(rows, jobmodel.JobTechTag{
			JobID:     jobID,
			Tech:      tech,
			CompanyID: companyID,
			TaggedAt:  taggedAt,
		})
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&rows).Error
}
