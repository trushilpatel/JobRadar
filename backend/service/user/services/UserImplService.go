package services

import (
	"context"
	"errors"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/platform/clerkadmin"
)

// implService is the business-logic layer of the user chain. It owns:
//
//   - Delete orchestration: Clerk auth user first, then the DB row. The
//     order matters — if the Clerk call fails we still have a useful
//     application row; if the DB call fails after Clerk succeeded the
//     caller can retry and the Clerk delete is idempotent (404 → success).
//
// Every other Service method auto-delegates to the repository via BaseService.
type implService struct {
	BaseService
	clerk clerkadmin.Client
}

// NewImplService constructs the orchestration layer. The clerk client may
// be nil in tests that do not exercise Delete; production callers always pass
// a real client because /api/me DELETE depends on it.
func NewImplService(next Service, clerk clerkadmin.Client) Service {
	return &implService{
		BaseService: NewBaseService(next),
		clerk:       clerk,
	}
}

// Delete removes the Clerk auth user first, then the DB row. A Clerk failure
// aborts the chain so we never leave the application users row referencing a
// now-orphaned auth identity. A nil clerk client is tolerated so unit tests
// can exercise the DB-only path.
//
// clerkadmin.ErrUserNotFound (Clerk 404) is tolerated, not fatal — it means
// the Clerk account is already gone (e.g. a prior delete attempt's Clerk
// call succeeded but the DB delete step afterward failed). Treating it as
// fatal would permanently block that account from ever being deleted.
func (s *implService) Delete(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "user.impl.Delete")
	defer tracing.End(span, &err)

	if id == "" {
		return domainerrs.ErrBadRequest.WithMessage("user id required")
	}
	if s.clerk != nil {
		if err := s.clerk.DeleteUser(ctx, id); err != nil && !errors.Is(err, clerkadmin.ErrUserNotFound) {
			return domainerrs.ErrInternal.WithMessage("user delete: clerk: " + err.Error())
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
