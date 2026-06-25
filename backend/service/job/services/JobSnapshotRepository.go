package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/gorm"
)

var _ SnapshotWriter = (*snapshotStore)(nil)

// snapshotStore is the GORM-backed implementation of SnapshotWriter
// (interface declared in JobService.go).
type snapshotStore struct {
	db *gorm.DB
}

func NewSnapshotStore(db *gorm.DB) SnapshotWriter {
	return &snapshotStore{db: db}
}

func (s *snapshotStore) InsertSnapshot(ctx context.Context, input dto.SnapshotInput) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertSnapshot")
	defer tracing.End(span, &err)
	row := jobmodel.JobSnapshot{
		ID:                input.ID,
		CompanyID:         input.CompanyID,
		SnapshottedAt:     time.Now(),
		Remote:            input.Remote,
		Sponsorship:       input.Sponsorship,
		RequiresCitizen:   input.RequiresCitizen,
		SecurityClearance: input.SecurityClearance,
		ExperienceYears:   input.ExperienceYears,
		TechStack:         input.TechStack,
	}
	if input.Country != "" {
		row.Country = &input.Country
	}
	if input.State != "" {
		row.State = &input.State
	}
	if input.EmployType != "" {
		row.EmployType = &input.EmployType
	}
	if input.RoleCategory != "" {
		row.RoleCategory = &input.RoleCategory
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *snapshotStore) UpdateSnapshotAction(ctx context.Context, id, action string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSnapshotAction")
	defer tracing.End(span, &err)
	now := time.Now()
	return s.db.WithContext(ctx).Model(&jobmodel.JobSnapshot{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"user_action": action,
			"actioned_at": now,
		}).Error
}

func (s *snapshotStore) UpdateSnapshotActionBatch(ctx context.Context, ids []string, action string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSnapshotActionBatch")
	defer tracing.End(span, &err)
	if len(ids) == 0 {
		return nil
	}
	now := time.Now()
	return s.db.WithContext(ctx).Model(&jobmodel.JobSnapshot{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"user_action": action,
			"actioned_at": now,
		}).Error
}
