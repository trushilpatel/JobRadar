package services

import (
	"context"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/platform/cache"
	"github.com/trushilpatel/jobradar/platform/clerkadmin"
)

// adminImplService is the business-logic layer of the admin user chain.
// It owns the Clerk → DB delete ordering for AdminDelete and cache
// invalidation for SetRoles / SetStatus / AdminDelete so that revoked or
// suspended users lose access on their very next request.
type adminImplService struct {
	BaseAdminService
	clerk clerkadmin.Client
	cache cache.Cache
}

// NewAdminImplService constructs the admin orchestration layer. Both
// clerk and c may be nil (dev / test environments); production callers
// always pass real instances.
func NewAdminImplService(next AdminService, clerk clerkadmin.Client, c cache.Cache) AdminService {
	return &adminImplService{
		BaseAdminService: NewBaseAdminService(next),
		clerk:            clerk,
		cache:            c,
	}
}

// adminIdentityCacheKey returns the cache key used by RequireIdentity to
// store a user's identity bundle. Must match middleware.userCacheKey.
func adminIdentityCacheKey(id string) string { return "user:" + id }

// SetRoles delegates to the repository, then evicts the user's cached
// identity so the new role set takes effect on their next request.
func (s *adminImplService) SetRoles(ctx context.Context, id string, roleIDs []string, grantedBy string) error {
	if err := s.next.SetRoles(ctx, id, roleIDs, grantedBy); err != nil {
		return err
	}
	s.evictIdentity(ctx, id)
	return nil
}

// SetStatus delegates to the repository, then evicts the user's cached
// identity so a suspension or re-activation takes effect immediately.
func (s *adminImplService) SetStatus(ctx context.Context, id string, status string) error {
	if err := s.next.SetStatus(ctx, id, status); err != nil {
		return err
	}
	s.evictIdentity(ctx, id)
	return nil
}

// AdminDelete removes the Clerk auth user first, then the DB row, then evicts
// any cached identity bundle so in-flight sessions see the deletion on their
// next request.
func (s *adminImplService) AdminDelete(ctx context.Context, id string) error {
	if id == "" {
		return domainerrs.ErrBadRequest.WithMessage("user id required")
	}
	if s.clerk != nil {
		if err := s.clerk.DeleteUser(ctx, id); err != nil {
			return domainerrs.ErrInternal.WithMessage("user admin delete: clerk: " + err.Error())
		}
	}
	if err := s.next.AdminDelete(ctx, id); err != nil {
		return err
	}
	s.evictIdentity(ctx, id)
	return nil
}

// evictIdentity removes the user's identity bundle from the cache. A nil
// cache or a Delete error is silently ignored — the identity TTL is the
// fallback and the operation has already committed to the DB.
func (s *adminImplService) evictIdentity(ctx context.Context, id string) {
	if s.cache != nil {
		_ = s.cache.Delete(ctx, adminIdentityCacheKey(id))
	}
}

var _ AdminService = (*adminImplService)(nil)
