package services

import (
	"context"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
)

// adminValidationService guards the admin chain against bad inputs.
type adminValidationService struct {
	BaseAdminService
}

// NewAdminValidationService constructs the validation layer.
func NewAdminValidationService(next AdminService) AdminService {
	return &adminValidationService{BaseAdminService: NewBaseAdminService(next)}
}

func (v *adminValidationService) GetWithRoles(ctx context.Context, id string) (*UserWithRoles, error) {
	if id == "" {
		return nil, domainerrs.ErrBadRequest.WithMessage("user id required")
	}
	return v.next.GetWithRoles(ctx, id)
}

func (v *adminValidationService) SetRoles(ctx context.Context, id string, roleIDs []string, grantedBy string) error {
	if id == "" {
		return domainerrs.ErrBadRequest.WithMessage("user id required")
	}
	if grantedBy == "" {
		return domainerrs.ErrBadRequest.WithMessage("granted_by required")
	}
	return v.next.SetRoles(ctx, id, roleIDs, grantedBy)
}

func (v *adminValidationService) SetStatus(ctx context.Context, id string, status string) error {
	if id == "" {
		return domainerrs.ErrBadRequest.WithMessage("user id required")
	}
	switch status {
	case usermodel.StatusActive, usermodel.StatusSuspended:
	default:
		return domainerrs.ErrBadRequest.WithMessage("status must be active or suspended")
	}
	return v.next.SetStatus(ctx, id, status)
}

func (v *adminValidationService) AdminDelete(ctx context.Context, id string) error {
	if id == "" {
		return domainerrs.ErrBadRequest.WithMessage("user id required")
	}
	return v.next.AdminDelete(ctx, id)
}

var _ AdminService = (*adminValidationService)(nil)
