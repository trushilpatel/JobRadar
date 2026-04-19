package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TechTagWriter writes normalized technology tags for a job into job_tech_tags.
type TechTagWriter interface {
	InsertTechTags(ctx context.Context, jobID, companyID string, techs []string, taggedAt time.Time) error
}

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
	rows := make([]model.JobTechTag, 0, len(techs))
	for _, tech := range techs {
		rows = append(rows, model.JobTechTag{
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
