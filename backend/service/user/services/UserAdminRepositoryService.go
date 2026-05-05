package services

import (
	"context"
	"errors"
	"strings"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	rolemodel "github.com/trushilpatel/jobradar/service/role/models"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
		limit = 20
	}
	if limit > 100 {
		limit = 100
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

	var userRoles []rolemodel.UserRole
	if err := r.db.WithContext(ctx).Where("user_id IN ?", userIDs).Find(&userRoles).Error; err != nil {
		return AdminListResult{}, domainerrs.ErrInternal.WithMessage("user admin repo: list roles: " + err.Error())
	}

	rolesByUser := make(map[string][]string, len(rows))
	for _, ur := range userRoles {
		rolesByUser[ur.UserID] = append(rolesByUser[ur.UserID], ur.RoleID)
	}

	items := make([]UserWithRoles, len(rows))
	for i, u := range rows {
		roles := rolesByUser[u.ID]
		if roles == nil {
			roles = []string{}
		}
		items[i] = UserWithRoles{User: u, Roles: roles}
	}

	return AdminListResult{Items: items, Total: total}, nil
}

// GetWithRoles returns a single user with their role IDs.
func (r *adminRepositoryService) GetWithRoles(ctx context.Context, id string) (*UserWithRoles, error) {
	var u usermodel.User
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerrs.ErrNotFound.WithMessage("user not found")
		}
		return nil, domainerrs.ErrInternal.WithMessage("user admin repo: get: " + err.Error())
	}

	var userRoles []rolemodel.UserRole
	if err := r.db.WithContext(ctx).Where("user_id = ?", id).Find(&userRoles).Error; err != nil {
		return nil, domainerrs.ErrInternal.WithMessage("user admin repo: get roles: " + err.Error())
	}

	roles := make([]string, len(userRoles))
	for i, ur := range userRoles {
		roles[i] = ur.RoleID
	}

	return &UserWithRoles{User: u, Roles: roles}, nil
}

// SetRoles replaces the full set of roles for a user atomically: all
// existing rows are deleted then the new set is inserted. The grantedBy
// ID is stored on each new row for the audit trail.
func (r *adminRepositoryService) SetRoles(ctx context.Context, id string, roleIDs []string, grantedBy string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&rolemodel.UserRole{}).Error; err != nil {
			return domainerrs.ErrInternal.WithMessage("user admin repo: set roles delete: " + err.Error())
		}

		if len(roleIDs) == 0 {
			return nil
		}

		now := time.Now().UTC()
		rows := make([]rolemodel.UserRole, len(roleIDs))
		for i, rid := range roleIDs {
			rows[i] = rolemodel.UserRole{
				UserID:    id,
				RoleID:    rid,
				GrantedAt: now,
				GrantedBy: &grantedBy,
			}
		}

		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
			return domainerrs.ErrInternal.WithMessage("user admin repo: set roles insert: " + err.Error())
		}
		return nil
	})
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
