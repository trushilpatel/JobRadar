package services

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/cache"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

// FieldCacheKey is the structured log field for cache key values.
const fieldCacheKey = "cache_key"

// PublicListGenKey is the cache key holding the monotonically-increasing
// generation counter for the public company list. Its value is baked into
// every list-page key (see publicListKey) so a single Set/bump on the admin
// write path invalidates EVERY cached list page at once — without enumerating
// or deleting individual page keys (which the cache.Cache interface cannot do).
// Exported so the admin chain can bump it.
const PublicListGenKey = "companies:public:list:gen"

// publicCacheService decorates the public chain with a read-through cache.
// Three key spaces are managed:
//
//   - company:public:{id}                          — single-row GetPublic results
//   - companies:public:list:gen                    — list generation counter
//   - companies:public:list:{gen}:{filterHash}     — cursor-paged ListPublic results
//
// The admin chain invalidates the per-row key directly and the entire list
// key space by bumping the generation counter — see CompanyAdminImplService.
type publicCacheService struct {
	BasePublicService
	cache cache.Cache
	ttl   time.Duration
	log   logger.Logger
}

// NewCacheService wraps next with a read-through cache. A nil cache or
// ttl<=0 turns the decorator into a pass-through — useful in tests that
// don't care about the cache layer. A nil log silences structured logging.
func NewCacheService(next CompanyService, c cache.Cache, ttl time.Duration, log logger.Logger) CompanyService {
	if log == nil {
		log = logger.NewNoop()
	}
	return &publicCacheService{
		BasePublicService: NewBasePublicService(next),
		cache:             c,
		ttl:               ttl,
		log:               log,
	}
}

// PublicCompanyKey returns the per-row cache key for the public view.
// Exported so the admin chain (and tests) can invalidate it precisely.
func PublicCompanyKey(id string) string { return "company:public:" + id }

// publicCompanyKey is the package-internal alias used by the cache decorator.
func publicCompanyKey(id string) string { return PublicCompanyKey(id) }

// publicListKey is the cache key for one cursor-page of ListPublic results.
// The hash captures every input that affects the result so distinct queries
// never collide. The current generation is folded into the key, so bumping
// the generation counter on an admin write makes every previously-cached page
// unreachable (a guaranteed miss) — the practical equivalent of an
// "invalidate all list pages" operation against a Get/Set/Delete-only cache.
func publicListKey(gen int64, filter companydto.CompanyFilter, limit int) string {
	raw := fmt.Sprintf("s=%s|sb=%s|sd=%s|c=%s|l=%d",
		filter.Search, filter.SortBy, filter.SortDir, filter.Cursor, limit)
	sum := sha1.Sum([]byte(raw))
	return fmt.Sprintf("companies:public:list:%d:%s", gen, hex.EncodeToString(sum[:]))
}

// readListGen returns the current public-list generation, defaulting to 0 on
// a miss or transport error so a cold cache still produces a stable key.
func (s *publicCacheService) readListGen(ctx context.Context) int64 {
	var gen int64
	hit, err := s.cache.Get(ctx, PublicListGenKey, &gen)
	if err != nil {
		s.log.Warn("cache.Get transport error on list generation", logger.Str(fieldCacheKey, PublicListGenKey), logger.Err(err))
		return 0
	}
	if !hit {
		return 0
	}
	return gen
}

// GetPublic tries the cache first, falls back to the next layer on miss,
// and writes the loaded value back. A cache transport failure surfaces as
// a miss so the source of truth is still consulted.
func (s *publicCacheService) GetPublic(ctx context.Context, id string) (*companydto.CompanyPublicView, error) {
	if s.cache == nil || s.ttl <= 0 {
		return s.next.GetPublic(ctx, id)
	}
	key := publicCompanyKey(id)
	var cached companydto.CompanyPublicView
	hit, cacheErr := s.cache.Get(ctx, key, &cached)
	if cacheErr != nil {
		s.log.Warn("cache.Get transport error on GetPublic", logger.Str(fieldCacheKey, key), logger.Err(cacheErr))
	} else if hit {
		s.log.Debug("cache hit on GetPublic", logger.Str(fieldCacheKey, key))
		return &cached, nil
	} else {
		s.log.Debug("cache miss on GetPublic", logger.Str(fieldCacheKey, key))
	}
	view, err := s.next.GetPublic(ctx, id)
	if err != nil {
		return nil, err
	}
	if view != nil {
		if setErr := s.cache.Set(ctx, key, view, s.ttl); setErr != nil {
			s.log.Warn("cache.Set failed on GetPublic", logger.Str(fieldCacheKey, key), logger.Err(setErr))
		}
	}
	return view, nil
}

// ListPublic caches each (generation, filter, cursor, limit) combination
// separately. Admin writes invalidate all list pages at once by bumping the
// generation counter (see CompanyAdminImplService.bumpListGen) — the next read
// computes a new key, misses, and reloads from the DB.
func (s *publicCacheService) ListPublic(ctx context.Context, filter companydto.CompanyFilter, limit int) (companydto.CompanyPublicListResult, error) {
	if s.cache == nil || s.ttl <= 0 {
		return s.next.ListPublic(ctx, filter, limit)
	}
	gen := s.readListGen(ctx)
	key := publicListKey(gen, filter, limit)
	var cached companydto.CompanyPublicListResult
	hit, cacheErr := s.cache.Get(ctx, key, &cached)
	if cacheErr != nil {
		s.log.Warn("cache.Get transport error on ListPublic", logger.Str(fieldCacheKey, key), logger.Err(cacheErr))
	} else if hit {
		s.log.Debug("cache hit on ListPublic", logger.Str(fieldCacheKey, key))
		return cached, nil
	} else {
		s.log.Debug("cache miss on ListPublic", logger.Str(fieldCacheKey, key))
	}
	result, err := s.next.ListPublic(ctx, filter, limit)
	if err != nil {
		return companydto.CompanyPublicListResult{}, err
	}
	if setErr := s.cache.Set(ctx, key, result, s.ttl); setErr != nil {
		s.log.Warn("cache.Set failed on ListPublic", logger.Str(fieldCacheKey, key), logger.Err(setErr))
	}
	return result, nil
}

var _ CompanyService = (*publicCacheService)(nil)
