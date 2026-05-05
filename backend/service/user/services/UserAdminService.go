package services

import (
	"context"

	usermodel "github.com/trushilpatel/jobradar/service/user/models"
)

// AdminService is the contract for admin-only user management operations.
// Every layer in the admin decorator chain honours this interface.
type AdminService interface {
	List(ctx context.Context, filter AdminUserFilter, page, limit int) (AdminListResult, error)
	GetWithRoles(ctx context.Context, id string) (*UserWithRoles, error)
	SetRoles(ctx context.Context, id string, roleIDs []string, grantedBy string) error
	SetStatus(ctx context.Context, id string, status string) error
	AdminDelete(ctx context.Context, id string) error
}

// AdminUserFilter holds optional predicates for user listing.
type AdminUserFilter struct {
	Status *string
	Email  string
}

// UserWithRoles combines a User row with the roles granted to that user.
type UserWithRoles struct {
	usermodel.User
	Roles []string `json:"roles"`
}

// AdminListResult is the paginated output of AdminService.List.
type AdminListResult struct {
	Items []UserWithRoles
	Total int64
}

// BaseAdminService auto-delegates every AdminService method to next. Admin
// decorator layers embed it and override only the methods they own.
type BaseAdminService struct {
	next AdminService
}

// NewBaseAdminService wraps next. Panics if next is nil — a misconfigured
// chain is a programmer error, never a runtime condition.
func NewBaseAdminService(next AdminService) BaseAdminService {
	if next == nil {
		panic("user.BaseAdminService: next must not be nil — check wire setup")
	}
	return BaseAdminService{next: next}
}

func (b *BaseAdminService) List(ctx context.Context, filter AdminUserFilter, page, limit int) (AdminListResult, error) {
	return b.next.List(ctx, filter, page, limit)
}

func (b *BaseAdminService) GetWithRoles(ctx context.Context, id string) (*UserWithRoles, error) {
	return b.next.GetWithRoles(ctx, id)
}

func (b *BaseAdminService) SetRoles(ctx context.Context, id string, roleIDs []string, grantedBy string) error {
	return b.next.SetRoles(ctx, id, roleIDs, grantedBy)
}

func (b *BaseAdminService) SetStatus(ctx context.Context, id string, status string) error {
	return b.next.SetStatus(ctx, id, status)
}

func (b *BaseAdminService) AdminDelete(ctx context.Context, id string) error {
	return b.next.AdminDelete(ctx, id)
}
