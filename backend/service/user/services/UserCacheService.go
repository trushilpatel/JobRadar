package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/platform/cache"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
)

// cacheService decorates the user chain with a read-through cache. Two
// separate keys are managed:
//
//   - user:row:{id}      — the *usermodel.User row served by Get.
//   - user:{id}          — the Identity bundle written by the auth middleware.
//     We don't read it here, but every write must
//     invalidate it so suspended/renamed users do not
//     ride a stale bundle for a full TTL.
//
// All other methods auto-delegate via BaseService.
type cacheService struct {
	BaseService
	cache cache.Cache
	ttl   time.Duration
}

// NewCacheService wraps next with a read-through cache. A nil cache or
// ttl<=0 turns the decorator into a pass-through — useful in tests that
// don't care about the cache layer.
func NewCacheService(next Service, c cache.Cache, ttl time.Duration) Service {
	return &cacheService{
		BaseService: NewBaseService(next),
		cache:       c,
		ttl:         ttl,
	}
}

// rowCacheKey is the per-row cache key for the User struct. Kept distinct
// from the identity-bundle key so the two never collide on JSON shape.
func rowCacheKey(id string) string { return "user:row:" + id }

// identityCacheKey is the cache key for the Identity bundle written by the
// auth middleware. Defined here (not in the middleware package, to avoid a
// hard dependency) and shared by both chains that need to invalidate it on
// writes: this file's self-service cacheService and the admin chain's
// adminImplService (UserAdminImplService.go) — one definition, not two.
func identityCacheKey(id string) string { return "user:" + id }

// Get tries the cache first, falls back to the next layer on miss, and
// writes the loaded value back. A cache transport failure surfaces as a
// miss so the source of truth is still consulted.
func (s *cacheService) Get(ctx context.Context, id string) (*usermodel.User, error) {
	if s.cache == nil || s.ttl <= 0 {
		return s.next.Get(ctx, id)
	}
	var cached usermodel.User
	hit, err := s.cache.Get(ctx, rowCacheKey(id), &cached)
	if err == nil && hit {
		return &cached, nil
	}
	user, err := s.next.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	// Best-effort write — never block the caller on a cache failure.
	_ = s.cache.Set(ctx, rowCacheKey(id), user, s.ttl)
	return user, nil
}

// UpdateProfile invalidates both cache keys before delegating. We do this
// before the write so a concurrent Get cannot race ahead and re-populate
// the cache with the stale row.
func (s *cacheService) UpdateProfile(ctx context.Context, id string, displayName string) error {
	if s.cache != nil {
		_ = s.cache.Delete(ctx, rowCacheKey(id), identityCacheKey(id))
	}
	if err := s.next.UpdateProfile(ctx, id, displayName); err != nil {
		return err
	}
	// Invalidate again after the write — covers the case where a concurrent
	// reader populated the cache between our pre-delete and the DB write.
	if s.cache != nil {
		_ = s.cache.Delete(ctx, rowCacheKey(id), identityCacheKey(id))
	}
	return nil
}

// Delete invalidates the cache before delegating. Same race protection as
// UpdateProfile — and the post-delete cleanup ensures a stale read between
// the two does not survive.
func (s *cacheService) Delete(ctx context.Context, id string) error {
	if s.cache != nil {
		_ = s.cache.Delete(ctx, rowCacheKey(id), identityCacheKey(id))
	}
	if err := s.next.Delete(ctx, id); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx, rowCacheKey(id), identityCacheKey(id))
	}
	return nil
}

var _ Service = (*cacheService)(nil)
