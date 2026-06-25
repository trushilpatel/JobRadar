package services

import (
	"context"
	"errors"
	"strings"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
	"gorm.io/gorm"
)

const (
	defaultUserAdminPageLimit = 20
	maxUserAdminPageLimit     = 100
)

// adminRepositoryService is the terminal layer for the admin user chain.
// It owns all SQL that the admin decorator chain is allowed to issue against
// the users and user_roles tables.
type adminRepositoryService struct {
	db *gorm.DB
}

// NewAdminRepositoryService constructs the terminal AdminService implementation.
func NewAdminRepositoryService(db *gorm.DB) AdminService {
	return &adminRepositoryService{db: db}
}

// List returns a page of users with their role IDs joined, ordered by
// created_at DESC. Total is the unfiltered count so the caller can render
// a page indicator.
func (r *adminRepositoryService) List(ctx context.Context, filter AdminUserFilter, page, limit int) (AdminListResult, error) {
	q := r.db.WithContext(ctx).Model(&usermodel.User{})

	if filter.Status != nil {
		q = q.Where("status = ?", *filter.Status)
	}
	if filter.Email != "" {
		q = q.Where("email ILIKE ?", "%"+strings.TrimSpace(filter.Email)+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return AdminListResult{}, domainerrs.ErrInternal.WithMessage("user admin repo: list count: " + err.Error())
	}

	if limit <= 0 {
		limit = defaultUserAdminPageLimit
	}
	if limit > maxUserAdminPageLimit {
		limit = maxUserAdminPageLimit
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	var rows []usermodel.User
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return AdminListResult{}, domainerrs.ErrInternal.WithMessage("user admin repo: list: " + err.Error())
	}

	if len(rows) == 0 {
		return AdminListResult{Items: []UserWithRoles{}, Total: total}, nil
	}

	userIDs := make([]string, len(rows))
	for i, u := range rows {
		userIDs[i] = u.ID
	}

	// Roles are now carried in Clerk JWT claims — no user_roles DB table.
	items := make([]UserWithRoles, len(rows))
	for i, u := range rows {
		items[i] = UserWithRoles{User: u, Roles: []string{}}
	}

	return AdminListResult{Items: items, Total: total}, nil
}

// GetWithRoles returns a single user. Roles are now carried in Clerk JWT claims,
// so the Roles field of the returned UserWithRoles is always an empty slice.
func (r *adminRepositoryService) GetWithRoles(ctx context.Context, id string) (*UserWithRoles, error) {
	var u usermodel.User
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerrs.ErrNotFound.WithMessage("user not found")
		}
		return nil, domainerrs.ErrInternal.WithMessage("user admin repo: get: " + err.Error())
	}
	return &UserWithRoles{User: u, Roles: []string{}}, nil
}

// SetRoles is a no-op in the Clerk auth model — roles are managed via
// Clerk publicMetadata, not via the DB user_roles table (dropped in migration 081).
func (r *adminRepositoryService) SetRoles(_ context.Context, _ string, _ []string, _ string) error {
	return nil
}

// SetStatus updates the user's status column.
func (r *adminRepositoryService) SetStatus(ctx context.Context, id string, status string) error {
	res := r.db.WithContext(ctx).Model(&usermodel.User{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": status, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return domainerrs.ErrInternal.WithMessage("user admin repo: set status: " + res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return domainerrs.ErrNotFound.WithMessage("user not found")
	}
	return nil
}

// AdminDelete removes the user row. The impl layer is responsible for
// calling the Supabase admin API before this method — the repository only
// owns the DB delete.
func (r *adminRepositoryService) AdminDelete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&usermodel.User{})
	if res.Error != nil {
		return domainerrs.ErrInternal.WithMessage("user admin repo: delete: " + res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return domainerrs.ErrNotFound.WithMessage("user not found")
	}
	return nil
}

var _ AdminService = (*adminRepositoryService)(nil)
