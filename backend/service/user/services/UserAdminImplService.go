package services

import (
	"context"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/platform/supabaseadmin"
)

// adminImplService is the business-logic layer of the admin user chain.
// It owns the Supabase → DB delete ordering for AdminDelete; everything
// else auto-delegates to the repository via BaseAdminService.
type adminImplService struct {
	BaseAdminService
	supabase supabaseadmin.Client
}

// NewAdminImplService constructs the admin orchestration layer. The
// supabase client may be nil in tests that do not exercise AdminDelete;
// production callers always pass a real client.
func NewAdminImplService(next AdminService, supabase supabaseadmin.Client) AdminService {
	return &adminImplService{
		BaseAdminService: NewBaseAdminService(next),
		supabase:         supabase,
	}
}

// AdminDelete removes the Supabase auth identity first, then the DB row.
// A Supabase failure aborts so we never orphan the application's users
// row. A nil supabase client skips the Supabase call (dev/test only).
func (s *adminImplService) AdminDelete(ctx context.Context, id string) error {
	if id == "" {
		return domainerrs.ErrBadRequest.WithMessage("user id required")
	}
	if s.supabase != nil {
		if err := s.supabase.DeleteUser(ctx, id); err != nil {
			return domainerrs.ErrInternal.WithMessage("user admin delete: supabase: " + err.Error())
		}
	}
	return s.next.AdminDelete(ctx, id)
}

var _ AdminService = (*adminImplService)(nil)
