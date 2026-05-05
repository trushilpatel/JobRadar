package services

import (
	"context"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/platform/supabaseadmin"
)

// implService is the business-logic layer of the user chain. It owns:
//
//   - Delete orchestration: Supabase auth row first, then the DB row. The
//     order matters — if the Supabase call fails we still have a useful
//     application row; if the DB call fails after Supabase succeeded the
//     caller can retry and the Supabase delete is idempotent (404 → success).
//
// Every other Service method auto-delegates to the repository via BaseService.
type implService struct {
	BaseService
	supabase supabaseadmin.Client
}

// NewImplService constructs the orchestration layer. The supabase client may
// be nil in tests that do not exercise Delete; production callers always pass
// a real client because /api/me DELETE depends on it.
func NewImplService(next Service, supabase supabaseadmin.Client) Service {
	return &implService{
		BaseService: NewBaseService(next),
		supabase:    supabase,
	}
}

// Delete removes the Supabase auth identity first, then the DB row. A
// Supabase failure aborts the chain so we never leave the application's
// users row referencing a now-orphaned auth identity. A nil supabase client
// is a programmer error — we panic at construction in production wiring,
// but tolerate it here so unit tests can exercise the DB-only path.
func (s *implService) Delete(ctx context.Context, id string) error {
	if id == "" {
		return domainerrs.ErrBadRequest.WithMessage("user id required")
	}
	if s.supabase != nil {
		if err := s.supabase.DeleteUser(ctx, id); err != nil {
			return domainerrs.ErrInternal.WithMessage("user delete: supabase: " + err.Error())
		}
	}
	return s.next.Delete(ctx, id)
}

// Get / GetByEmail / Provision / UpdateProfile auto-delegate to next via
// BaseService — the impl layer holds no orchestration for those today. Adding
// future business rules (e.g. email change verification) means overriding
// here without touching the repository.

// Compile-time assertion: implService implements Service. Cheap insurance
// against the embed-pattern silently dropping a method override.
var _ Service = (*implService)(nil)
