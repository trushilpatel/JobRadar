package services

import (
	"context"
	"errors"
	"strings"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	rolemodel "github.com/trushilpatel/jobradar/service/role/models"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// repositoryService is the terminal user-domain layer — every Service method
// either reads or writes the `users` table directly. It holds no logic
// beyond what is needed to translate domain inputs into GORM calls and back
// into typed errors the rest of the chain can introspect.
type repositoryService struct {
	db *gorm.DB
}

// NewRepositoryService constructs the terminal Service implementation.
// Returning the interface (not the concrete type) keeps higher layers
// dependency-inverted at compile time.
func NewRepositoryService(db *gorm.DB) Service {
	return &repositoryService{db: db}
}

func (r *repositoryService) Get(ctx context.Context, id string) (_ *usermodel.User, err error) {
	ctx, span := tracing.Start(ctx, "user.repo.Get")
	defer tracing.End(span, &err)

	var u usermodel.User
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerrs.ErrNotFound.WithMessage("user not found")
		}
		return nil, domainerrs.ErrInternal.WithMessage("user repo: get: " + err.Error())
	}
	return &u, nil
}

func (r *repositoryService) GetByEmail(ctx context.Context, email string) (_ *usermodel.User, err error) {
	ctx, span := tracing.Start(ctx, "user.repo.GetByEmail")
	defer tracing.End(span, &err)

	var u usermodel.User
	if err := r.db.WithContext(ctx).Where("email = ?", strings.ToLower(strings.TrimSpace(email))).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerrs.ErrNotFound.WithMessage("user not found")
		}
		return nil, domainerrs.ErrInternal.WithMessage("user repo: get by email: " + err.Error())
	}
	return &u, nil
}

// Provision performs an idempotent UPSERT on the user row keyed by id, then
// best-effort grants the baseline `user` role. Concurrent first-login
// requests for the same JWT subject converge on a single row because the
// id column is the conflict target.
func (r *repositoryService) Provision(ctx context.Context, id, email string) (_ *usermodel.User, err error) {
	ctx, span := tracing.Start(ctx, "user.repo.Provision")
	defer tracing.End(span, &err)

	now := time.Now().UTC()
	row := usermodel.User{
		ID:        id,
		Email:     strings.ToLower(strings.TrimSpace(email)),
		Status:    usermodel.StatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	// On conflict on id, do nothing — keep the existing row. The follow-up
	// First() reads whichever row is now in the DB.
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(&row).Error; err != nil {
		return nil, domainerrs.ErrInternal.WithMessage("user repo: provision: " + err.Error())
	}

	// Grant the baseline `user` role idempotently — first-login users always
	// carry the `user` role even if no admin promotion fires.
	roleRow := rolemodel.UserRole{UserID: id, RoleID: rolemodel.RoleUser, GrantedAt: now}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&roleRow).Error; err != nil {
		return nil, domainerrs.ErrInternal.WithMessage("user repo: provision role: " + err.Error())
	}

	var fresh usermodel.User
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&fresh).Error; err != nil {
		return nil, domainerrs.ErrInternal.WithMessage("user repo: provision read-back: " + err.Error())
	}
	return &fresh, nil
}

// UpdateProfile sets display_name + updated_at. Empty-string display name is
// stored as NULL so callers can clear the value.
func (r *repositoryService) UpdateProfile(ctx context.Context, id string, displayName string) (err error) {
	ctx, span := tracing.Start(ctx, "user.repo.UpdateProfile")
	defer tracing.End(span, &err)

	now := time.Now().UTC()
	updates := map[string]any{
		"updated_at": now,
	}
	trimmed := strings.TrimSpace(displayName)
	if trimmed == "" {
		updates["display_name"] = nil
	} else {
		updates["display_name"] = trimmed
	}
	res := r.db.WithContext(ctx).Model(&usermodel.User{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return domainerrs.ErrInternal.WithMessage("user repo: update profile: " + res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return domainerrs.ErrNotFound.WithMessage("user not found")
	}
	return nil
}

// Delete removes the user row. CASCADE on user_roles, user_job_state, and the
// future per-user tables takes care of dependent rows; the repo does not
// orchestrate that itself.
func (r *repositoryService) Delete(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "user.repo.Delete")
	defer tracing.End(span, &err)

	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&usermodel.User{})
	if res.Error != nil {
		return domainerrs.ErrInternal.WithMessage("user repo: delete: " + res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return domainerrs.ErrNotFound.WithMessage("user not found")
	}
	return nil
}
