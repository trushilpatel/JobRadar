package services

import (
	"context"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	rolemodel "github.com/trushilpatel/jobradar/service/role/models"
)

// validationService is the input-validation layer of the role chain. It
// enforces the role-id allowlist (`user`, `admin`) so callers cannot grant
// arbitrary strings — a defence-in-depth check on top of the FK constraint
// that the migration adds at the DB level.
type validationService struct {
	BaseService
}

// NewValidationService constructs the validation layer.
func NewValidationService(next Service) Service {
	return &validationService{BaseService: NewBaseService(next)}
}

// validRoles is the set of role IDs the application is allowed to grant or
// revoke in Phase 1. Future phases may extend this — the comparison is a
// map lookup so adding entries is one line.
var validRoles = map[string]struct{}{
	rolemodel.RoleUser:  {},
	rolemodel.RoleAdmin: {},
}

func (v *validationService) Grant(ctx context.Context, userID, roleID string, grantedBy *string) error {
	if err := validateRoleID(roleID); err != nil {
		return err
	}
	return v.next.Grant(ctx, userID, roleID, grantedBy)
}

func (v *validationService) Revoke(ctx context.Context, userID, roleID string) error {
	if err := validateRoleID(roleID); err != nil {
		return err
	}
	return v.next.Revoke(ctx, userID, roleID)
}

func validateRoleID(roleID string) error {
	if _, ok := validRoles[roleID]; !ok {
		return domainerrs.ValidationErrors{{
			Field:   "role_id",
			Code:    "INVALID_VALUE",
			Message: "role_id must be one of: user, admin",
		}}
	}
	return nil
}

var _ Service = (*validationService)(nil)
