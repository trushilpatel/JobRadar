package services

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/trushilpatel/jobradar/platform/cache"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

// publicCacheService decorates the public chain with a read-through cache.
// Two key spaces are managed:
//
//   - company:public:{id}                     — single-row GetPublic results
//   - companies:public:list:{filterHash}      — paged ListPublic results
//
// The admin chain invalidates these keys via the same cache.Cache interface
// on every write — see CompanyAdminImplService.
type publicCacheService struct {
	BasePublicService
	cache cache.Cache
	ttl   time.Duration
}

// NewCacheService wraps next with a read-through cache. A nil cache or
// ttl<=0 turns the decorator into a pass-through — useful in tests that
// don't care about the cache layer.
func NewCacheService(next CompanyService, c cache.Cache, ttl time.Duration) CompanyService {
	return &publicCacheService{
		BasePublicService: NewBasePublicService(next),
		cache:             c,
		ttl:               ttl,
	}
}

// PublicCompanyKey returns the per-row cache key for the public view.
// Exported so the admin chain (and tests) can invalidate it precisely.
func PublicCompanyKey(id string) string { return "company:public:" + id }

// publicCompanyKey is the package-internal alias used by the cache decorator.
func publicCompanyKey(id string) string { return PublicCompanyKey(id) }

// publicListKey is the cache key for one paged ListPublic response. The
// hash captures every input that affects the result so distinct queries
// never collide.
func publicListKey(filter companydto.CompanyFilter, page, limit int) string {
	raw := fmt.Sprintf("s=%s|sb=%s|sd=%s|p=%d|l=%d",
		filter.Search, filter.SortBy, filter.SortDir, page, limit)
	sum := sha1.Sum([]byte(raw))
	return "companies:public:list:" + hex.EncodeToString(sum[:])
}

// GetPublic tries the cache first, falls back to the next layer on miss,
// and writes the loaded value back. A cache transport failure surfaces as
// a miss so the source of truth is still consulted.
func (s *publicCacheService) GetPublic(ctx context.Context, id string) (*companydto.CompanyPublicView, error) {
	if s.cache == nil || s.ttl <= 0 {
		return s.next.GetPublic(ctx, id)
	}
	var cached companydto.CompanyPublicView
	hit, err := s.cache.Get(ctx, publicCompanyKey(id), &cached)
	if err == nil && hit {
		return &cached, nil
	}
	view, err := s.next.GetPublic(ctx, id)
	if err != nil {
		return nil, err
	}
	if view != nil {
		_ = s.cache.Set(ctx, publicCompanyKey(id), view, s.ttl)
	}
	return view, nil
}

// ListPublic caches each (filter, page, limit) combination separately.
// Admin writes invalidate all list keys via InvalidateAll on the package's
// shared cache helper (see CompanyAdminImplService).
func (s *publicCacheService) ListPublic(ctx context.Context, filter companydto.CompanyFilter, page, limit int) (companydto.CompanyPublicListResult, error) {
	if s.cache == nil || s.ttl <= 0 {
		return s.next.ListPublic(ctx, filter, page, limit)
	}
	key := publicListKey(filter, page, limit)
	var cached companydto.CompanyPublicListResult
	hit, err := s.cache.Get(ctx, key, &cached)
	if err == nil && hit {
		return cached, nil
	}
	result, err := s.next.ListPublic(ctx, filter, page, limit)
	if err != nil {
		return companydto.CompanyPublicListResult{}, err
	}
	_ = s.cache.Set(ctx, key, result, s.ttl)
	return result, nil
}

var _ CompanyService = (*publicCacheService)(nil)
