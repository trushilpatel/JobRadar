package services

import (
	"context"
	"fmt"
	"time"

	"github.com/trushilpatel/jobradar/platform/cache"
	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
)

// atsCacheService decorates the ats chain with a read-through cache.
//
// Two key families are managed:
//
//   - "ats:list"  — the full ATS list served by ListATS.
//   - "ats:"+name — a single row served by GetATS.
//
// UpdateATS invalidates both the list key and the per-name key before the
// write and again after, so stale reads between the two evictions cannot
// survive for more than the pre-write window.
//
// A nil cache or ttl≤0 turns the decorator into a pass-through — useful in
// tests that don't care about the cache layer.
type atsCacheService struct {
	BaseAtsService
	cache cache.Cache
	ttl   time.Duration
}

// NewCacheService wraps next with a read-through cache.
func NewCacheService(next AtsService, c cache.Cache, ttl time.Duration) AtsService {
	return &atsCacheService{
		BaseAtsService: NewBaseAtsService(next),
		cache:          c,
		ttl:            ttl,
	}
}

const atsListKey = "ats:list"

func atsNameKey(name string) string { return "ats:" + name }

// ListATS tries the cache first; on a miss calls next and populates the cache.
func (s *atsCacheService) ListATS(ctx context.Context) ([]atsdto.ATSDTO, error) {
	if s.cache == nil || s.ttl <= 0 {
		return s.next.ListATS(ctx)
	}
	var cached []atsdto.ATSDTO
	hit, err := s.cache.Get(ctx, atsListKey, &cached)
	if err == nil && hit {
		return cached, nil
	}
	items, err := s.next.ListATS(ctx)
	if err != nil {
		return nil, fmt.Errorf("ats.cache.ListATS: %w", err)
	}
	// Best-effort write — never block the caller on a cache failure.
	_ = s.cache.Set(ctx, atsListKey, items, s.ttl)
	return items, nil
}

// GetATS tries the cache first; on a miss calls next and populates the cache.
func (s *atsCacheService) GetATS(ctx context.Context, name string) (atsdto.ATSDTO, error) {
	if s.cache == nil || s.ttl <= 0 {
		return s.next.GetATS(ctx, name)
	}
	key := atsNameKey(name)
	var cached atsdto.ATSDTO
	hit, err := s.cache.Get(ctx, key, &cached)
	if err == nil && hit {
		return cached, nil
	}
	item, err := s.next.GetATS(ctx, name)
	if err != nil {
		return atsdto.ATSDTO{}, fmt.Errorf("ats.cache.GetATS: %w", err)
	}
	// Best-effort write.
	_ = s.cache.Set(ctx, key, item, s.ttl)
	return item, nil
}

// UpdateATS invalidates both cache keys before the write and again after so
// concurrent readers cannot re-populate the cache with stale data between
// the two evictions.
func (s *atsCacheService) UpdateATS(ctx context.Context, name string, input atsdto.UpdateATSInput) (atsdto.ATSDTO, error) {
	if s.cache != nil {
		_ = s.cache.Delete(ctx, atsListKey, atsNameKey(name))
	}
	result, err := s.next.UpdateATS(ctx, name, input)
	if s.cache != nil {
		_ = s.cache.Delete(ctx, atsListKey, atsNameKey(name))
	}
	if err != nil {
		return atsdto.ATSDTO{}, fmt.Errorf("ats.cache.UpdateATS: %w", err)
	}
	return result, nil
}

// Compile-time assertion: atsCacheService implements AtsService.
var _ AtsService = (*atsCacheService)(nil)
