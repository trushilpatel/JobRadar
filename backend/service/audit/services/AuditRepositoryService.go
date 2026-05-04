package services

import (
	"context"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	auditmodel "github.com/trushilpatel/jobradar/service/audit/models"
	"gorm.io/gorm"
)

// repositoryService is the terminal audit-domain layer. It owns the only
// SQL the audit chain is allowed to issue.
type repositoryService struct {
	db *gorm.DB
}

// NewRepositoryService constructs the terminal Service implementation.
func NewRepositoryService(db *gorm.DB) Service {
	return &repositoryService{db: db}
}

// Append inserts a single audit row. The row's ID is allowed to be empty —
// the DB default (`gen_random_uuid()`) populates it. Created_at is treated
// the same way: zero ⇒ `now()` from the column default.
func (r *repositoryService) Append(ctx context.Context, entry auditmodel.Entry) error {
	if entry.Action == "" {
		return domainerrs.ErrBadRequest.WithMessage("action required")
	}
	if err := r.db.WithContext(ctx).Create(&entry).Error; err != nil {
		return domainerrs.ErrInternal.WithMessage("audit repo: append: " + err.Error())
	}
	return nil
}

// List returns audit rows ordered by created_at desc with the given filters.
// Total is the unfiltered-by-pagination count so the caller can render a
// page indicator.
func (r *repositoryService) List(ctx context.Context, filter ListFilter) ([]auditmodel.Entry, int64, error) {
	q := r.db.WithContext(ctx).Model(&auditmodel.Entry{})
	if filter.ActorUserID != nil {
		q = q.Where("actor_user_id = ?", *filter.ActorUserID)
	}
	if filter.Action != nil {
		q = q.Where("action = ?", *filter.Action)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, domainerrs.ErrInternal.WithMessage("audit repo: count: " + err.Error())
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var rows []auditmodel.Entry
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, domainerrs.ErrInternal.WithMessage("audit repo: list: " + err.Error())
	}
	return rows, total, nil
}
