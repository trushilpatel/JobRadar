package services

import (
	"context"

	"github.com/trushilpatel/jobradar/platform/cache"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

// adminImplService is the admin chain's business-logic decorator. Its
// single concern today is invalidating the public read cache on every
// successful mutation so public readers never see stale data.
type adminImplService struct {
	BaseAdminService
	cache cache.Cache
}

// NewAdminImplService wraps next and uses cache to invalidate
// company:public:{id} on mutations. A nil cache turns invalidation into a
// no-op — useful in tests that wire admin without a public cache.
func NewAdminImplService(next CompanyAdminService, c cache.Cache) CompanyAdminService {
	return &adminImplService{
		BaseAdminService: NewBaseAdminService(next),
		cache:            c,
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
	if view != nil && s.cache != nil {
		_ = s.cache.Delete(ctx, PublicCompanyKey(view.ID))
	}
	return view, nil
}

// UpdatePublic invalidates the per-id key both before and after the write.
// Pre-delete prevents a concurrent reader from racing in and re-populating
// the cache with the stale row.
func (s *adminImplService) UpdatePublic(ctx context.Context, id string, p companydto.CompanyPublicInput) error {
	if s.cache != nil {
		_ = s.cache.Delete(ctx, PublicCompanyKey(id))
	}
	if err := s.next.UpdatePublic(ctx, id, p); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx, PublicCompanyKey(id))
	}
	return nil
}

// UpdateScanSettings does NOT touch the public cache — scan_settings fields
// are not part of the public view. Method only delegates.

// Delete invalidates pre- and post-write so a concurrent reader cannot
// re-populate from the about-to-be-deleted row.
func (s *adminImplService) Delete(ctx context.Context, id string) error {
	if s.cache != nil {
		_ = s.cache.Delete(ctx, PublicCompanyKey(id))
	}
	if err := s.next.Delete(ctx, id); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx, PublicCompanyKey(id))
	}
	return nil
}

var _ CompanyAdminService = (*adminImplService)(nil)
