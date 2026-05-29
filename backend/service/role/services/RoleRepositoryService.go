package services

import (
	"context"
	"errors"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	rolemodel "github.com/trushilpatel/jobradar/service/role/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// repositoryService is the terminal role-domain layer.
type repositoryService struct {
	db *gorm.DB
}

// NewRepositoryService constructs the terminal Service implementation.
func NewRepositoryService(db *gorm.DB) Service {
	return &repositoryService{db: db}
}

// ListForUser returns the role IDs assigned to userID, or an empty slice if
// none. A missing user is not surfaced as NotFound here — the caller's
// concept of "user does not exist" is enforced upstream in the auth flow,
// which provisions the row before reaching the role layer.
func (r *repositoryService) ListForUser(ctx context.Context, userID string) (_ []string, err error) {
	ctx, span := tracing.Start(ctx, "role.repo.ListForUser")
	defer tracing.End(span, &err)

	if userID == "" {
		return nil, domainerrs.ErrBadRequest.WithMessage("user id required")
	}
	var ids []string
	if err := r.db.WithContext(ctx).
		Model(&rolemodel.UserRole{}).
		Where("user_id = ?", userID).
		Pluck("role_id", &ids).Error; err != nil {
		return nil, domainerrs.ErrInternal.WithMessage("role repo: list: " + err.Error())
	}
	return ids, nil
}

// Grant inserts a (user_id, role_id) row idempotently. Conflicts on the
// composite primary key are silently swallowed so callers can call Grant
// repeatedly without explicit existence checks.
func (r *repositoryService) Grant(ctx context.Context, userID, roleID string, grantedBy *string) (err error) {
	ctx, span := tracing.Start(ctx, "role.repo.Grant")
	defer tracing.End(span, &err)

	if userID == "" || roleID == "" {
		return domainerrs.ErrBadRequest.WithMessage("user id and role id required")
	}
	row := rolemodel.UserRole{
		UserID:    userID,
		RoleID:    roleID,
		GrantedAt: time.Now().UTC(),
		GrantedBy: grantedBy,
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return domainerrs.ErrInternal.WithMessage("role repo: grant: " + err.Error())
	}
	return nil
}

// Revoke removes a single (user_id, role_id) row. A missing row surfaces as
// NotFound so callers can distinguish "you tried to revoke something that
// wasn't granted" from "the DB write failed".
func (r *repositoryService) Revoke(ctx context.Context, userID, roleID string) (err error) {
	ctx, span := tracing.Start(ctx, "role.repo.Revoke")
	defer tracing.End(span, &err)

	if userID == "" || roleID == "" {
		return domainerrs.ErrBadRequest.WithMessage("user id and role id required")
	}
	res := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		Delete(&rolemodel.UserRole{})
	if res.Error != nil {
		return domainerrs.ErrInternal.WithMessage("role repo: revoke: " + res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return domainerrs.ErrNotFound.WithMessage("user role not found")
	}
	return nil
}

// ListAll returns every role row in the DB. Used by the future admin UI to
// populate the role-select dropdown. Phase 1 only seeds two rows so this is
// inexpensive.
func (r *repositoryService) ListAll(ctx context.Context) (_ []rolemodel.Role, err error) {
	ctx, span := tracing.Start(ctx, "role.repo.ListAll")
	defer tracing.End(span, &err)

	var rows []rolemodel.Role
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, domainerrs.ErrInternal.WithMessage("role repo: list all: " + err.Error())
	}
	return rows, nil
}
