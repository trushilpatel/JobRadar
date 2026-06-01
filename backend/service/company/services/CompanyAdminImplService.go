package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/cache"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

// adminImplService is the admin chain's business-logic decorator. Its single
// concern is invalidating the public read cache on every successful mutation
// so public readers never see stale data. Two invalidation surfaces:
//
//   - the per-row key company:public:{id} is deleted directly;
//   - the entire public list key space is invalidated by bumping the
//     generation counter (PublicListGenKey), which changes the key every
//     subsequent list read computes — see CompanyCacheService.publicListKey.
type adminImplService struct {
	BaseAdminService
	cache cache.Cache
	log   logger.Logger
}

// bumpListGen increments the public-list generation counter so every cached
// list page becomes unreachable. Best-effort: a cache transport failure is
// logged, never surfaced — the DB write already succeeded and a stale list
// page expires under its own TTL as a backstop. Read-modify-write is used
// because the cache.Cache contract has no atomic Incr; a lost race only means
// an extra (harmless) generation gap, never a stale read, because any change
// to the value still changes the key.
func (s *adminImplService) bumpListGen(ctx context.Context) {
	if s.cache == nil {
		return
	}
	var gen int64
	if _, err := s.cache.Get(ctx, PublicListGenKey, &gen); err != nil {
		s.log.Warn("cache.Get failed on list-gen bump", logger.Err(err))
		// Fall through with gen=0 → Set to 1; still invalidates current pages.
	}
	if err := s.cache.Set(ctx, PublicListGenKey, gen+1, 0); err != nil {
		s.log.Warn("cache.Set failed on list-gen bump", logger.Err(err))
	}
}

// NewAdminImplService wraps next and uses cache to invalidate
// company:public:{id} on mutations. A nil cache turns invalidation into a
// no-op — useful in tests that wire admin without a public cache.
// A nil log silences all structured logging.
func NewAdminImplService(next CompanyAdminService, c cache.Cache, log logger.Logger) CompanyAdminService {
	if log == nil {
		log = logger.NewNoop()
	}
	return &adminImplService{
		BaseAdminService: NewBaseAdminService(next),
		cache:            c,
		log:              log,
	}
}

// Create invalidates the (likely-absent) public cache key for the new id
// pre-emptively, then writes. This keeps invalidation symmetric across all
// mutating methods.
func (s *adminImplService) Create(ctx context.Context, public companydto.CompanyPublicInput, settings companydto.CompanyScanSettingsInput) (*companydto.CompanyAdminView, error) {
	view, err := s.next.Create(ctx, public, settings)
	if err != nil {
		return nil, err
	}
	if view != nil {
		adapterType := ""
		if settings.AdapterType != nil {
			adapterType = *settings.AdapterType
		}
		s.log.Info("company created",
			logger.Str(logger.FieldCompany, view.ID),
			logger.Bool(logger.FieldHasSettings, !settings.IsEmpty()),
			logger.Str(logger.FieldAdapterType, adapterType))
		if s.cache != nil {
			if delErr := s.cache.Delete(ctx, PublicCompanyKey(view.ID)); delErr != nil {
				s.log.Warn("cache invalidation failed on Create", logger.Str(logger.FieldCompany, view.ID), logger.Err(delErr))
			}
		}
		// A new company changes the public list → invalidate all list pages.
		s.bumpListGen(ctx)
	}
	return view, nil
}

// UpdatePublic invalidates the per-id key both before and after the write.
// Pre-delete prevents a concurrent reader from racing in and re-populating
// the cache with the stale row.
func (s *adminImplService) UpdatePublic(ctx context.Context, id string, p companydto.CompanyPublicInput) error {
	if s.cache != nil {
		if delErr := s.cache.Delete(ctx, PublicCompanyKey(id)); delErr != nil {
			s.log.Warn("cache pre-invalidation failed on UpdatePublic", logger.Str(logger.FieldCompany, id), logger.Err(delErr))
		}
	}
	if err := s.next.UpdatePublic(ctx, id, p); err != nil {
		return err
	}
	s.log.Info("company public fields updated", logger.Str(logger.FieldCompany, id))
	if s.cache != nil {
		if delErr := s.cache.Delete(ctx, PublicCompanyKey(id)); delErr != nil {
			s.log.Warn("cache post-invalidation failed on UpdatePublic", logger.Str(logger.FieldCompany, id), logger.Err(delErr))
		}
	}
	// Public fields appear in list rows → invalidate all list pages.
	s.bumpListGen(ctx)
	return nil
}

// UpdateScanSettings does NOT touch the public cache — scan_settings fields
// are not part of the public view. Method only delegates.

// Delete invalidates pre- and post-write so a concurrent reader cannot
// re-populate from the about-to-be-deleted row.
func (s *adminImplService) Delete(ctx context.Context, id string) error {
	if s.cache != nil {
		if delErr := s.cache.Delete(ctx, PublicCompanyKey(id)); delErr != nil {
			s.log.Warn("cache pre-invalidation failed on Delete", logger.Str(logger.FieldCompany, id), logger.Err(delErr))
		}
	}
	if err := s.next.Delete(ctx, id); err != nil {
		return err
	}
	s.log.Info("company deleted", logger.Str(logger.FieldCompany, id))
	if s.cache != nil {
		if delErr := s.cache.Delete(ctx, PublicCompanyKey(id)); delErr != nil {
			s.log.Warn("cache post-invalidation failed on Delete", logger.Str(logger.FieldCompany, id), logger.Err(delErr))
		}
	}
	// A removed company changes the public list → invalidate all list pages.
	s.bumpListGen(ctx)
	return nil
}

var _ CompanyAdminService = (*adminImplService)(nil)
