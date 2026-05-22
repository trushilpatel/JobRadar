// Package services holds the company domain decorator chains.
//
// Phase 2 split: the company slice exposes TWO independent decorator chains
// against TWO interfaces.
//
//	CompanyService       — public, read-only. Mounted under /api/companies.
//	                       Returns CompanyPublicView (lean public fields only).
//	CompanyAdminService  — admin, full read/write. Mounted under /admin/companies.
//	                       Returns CompanyAdminView (public + scan_settings).
//
// Each interface gets its own BaseService auto-delegator and its own
// implementation chain (Repo → Impl → Validation → Rest → Router for admin,
// Repo → Impl → Cache → Validation → Rest → Router for public).
//
// A separate ScanSettingsRepository (in CompanyScanSettingsRepositoryService.go)
// is the single point of access for `company_scan_settings` rows — used by
// the admin chain and by background workers.
package services

import (
	"context"

	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
)

// ScanHistoryReader is the narrow interface the company admin repo needs to
// populate per-company scan history in list responses. Implemented by
// scan.ScanLogStore. Declared here (and not in dto/) to satisfy the rule that
// only {Domain}Service.go may declare interfaces in a domain.
type ScanHistoryReader interface {
	GetScanHistories(ctx context.Context, companyIDs []string) (map[string][]bool, error)
}

// ScanSettingsRepository is the single point of access for the
// `company_scan_settings` table. It is consumed by:
//
//   - The admin chain (CompanyAdminImplService) — when admins create or
//     update settings via /admin/companies/{id}/scan-settings.
//   - Background workers (scheduler, crawler) — to read adapter_type /
//     adapter_config / scan_interval_override / active, and to write
//     scan-state outcomes (UpdateScanState).
//
// Every method rejects an empty companyID with ErrInternal.WithMessage(
// "company id required") before touching the DB so a forgotten ctx scoping
// can never silently broaden a query to the entire table.
type ScanSettingsRepository interface {
	Get(ctx context.Context, companyID string) (*companymodel.CompanyScanSettings, error)
	GetMissing(ctx context.Context) ([]string, error)
	Create(ctx context.Context, s companymodel.CompanyScanSettings) error
	Update(ctx context.Context, companyID string, p companydto.CompanyScanSettingsInput) error
	UpdateScanState(ctx context.Context, companyID string, st companydto.CompanyScanStateUpdate) error
	Delete(ctx context.Context, companyID string) error
}

// CompanyService is the public read-only contract for the company domain.
// Returned views never carry operational state (adapter type, scan
// timestamps, error counters) — those live on CompanyAdminView and are
// reachable only through CompanyAdminService.
type CompanyService interface {
	ListPublic(ctx context.Context, filter companydto.CompanyFilter, limit int) (companydto.CompanyPublicListResult, error)
	GetPublic(ctx context.Context, id string) (*companydto.CompanyPublicView, error)
}

// BasePublicService auto-delegates every CompanyService method to next.
// Embed it in decorators that override only a subset of methods.
type BasePublicService struct {
	next CompanyService
}

// NewBasePublicService wraps next. Panics on nil — a nil next is always a
// wiring mistake, never a runtime condition.
func NewBasePublicService(next CompanyService) BasePublicService {
	if next == nil {
		panic("BasePublicService: next must not be nil — check wire setup")
	}
	return BasePublicService{next: next}
}

func (b *BasePublicService) ListPublic(ctx context.Context, filter companydto.CompanyFilter, limit int) (companydto.CompanyPublicListResult, error) {
	return b.next.ListPublic(ctx, filter, limit)
}

func (b *BasePublicService) GetPublic(ctx context.Context, id string) (*companydto.CompanyPublicView, error) {
	return b.next.GetPublic(ctx, id)
}

// CompanyAdminService is the admin contract for the company domain. Carries
// the full mutation surface plus the operational reads workers depend on
// (GetByIDs / GetAll). All write methods invalidate the public cache via the
// admin Impl layer — public readers never see stale data.
type CompanyAdminService interface {
	// List returns admin views with scan_settings LEFT-JOINed in a single
	// query. Honours the full CompanyFilter including operational fields
	// (Active, AdapterTypes, HasSettings).
	List(ctx context.Context, filter companydto.CompanyFilter, limit int) (companydto.CompanyAdminListResult, error)
	Get(ctx context.Context, id string) (*companydto.CompanyAdminView, error)

	// Create inserts a `companies` row plus an optional
	// `company_scan_settings` row in a single transaction. When settings is
	// empty (CompanyScanSettingsInput.IsEmpty), no scan_settings row is
	// written and the company starts in the un-onboarded-for-scan state.
	Create(ctx context.Context, public companydto.CompanyPublicInput, settings companydto.CompanyScanSettingsInput) (*companydto.CompanyAdminView, error)

	// UpdatePublic patches the `companies` row only — partial-patch on the
	// public fields, never touches scan_settings.
	UpdatePublic(ctx context.Context, id string, p companydto.CompanyPublicInput) error

	// UpdateScanSettings patches the `company_scan_settings` row. Returns
	// NOT_FOUND when no settings row exists yet for the company — admins
	// must POST one first via Create's settings argument or a future
	// dedicated endpoint.
	UpdateScanSettings(ctx context.Context, id string, p companydto.CompanyScanSettingsInput) error

	// Delete removes the `companies` row. The matching scan_settings row is
	// removed by ON DELETE CASCADE.
	Delete(ctx context.Context, id string) error

	// SetAllActive toggles `company_scan_settings.active` across every row
	// in the table. Companies without a scan_settings row are unaffected.
	SetAllActive(ctx context.Context, active bool) error

	// GetByIDs returns admin views for the given IDs (scan_settings
	// LEFT-JOINed). Returns an empty slice when ids is empty.
	GetByIDs(ctx context.Context, ids []string) ([]companydto.CompanyAdminView, error)

	// GetAll returns every company as an admin view (workers consume this
	// through narrower wrappers — see ScanSettingsRepository for the
	// per-company hot path).
	GetAll(ctx context.Context) ([]companydto.CompanyAdminView, error)
}

// BaseAdminService auto-delegates every CompanyAdminService method to next.
type BaseAdminService struct {
	next CompanyAdminService
}

// NewBaseAdminService wraps next. Panics on nil for the same reason as
// NewBasePublicService.
func NewBaseAdminService(next CompanyAdminService) BaseAdminService {
	if next == nil {
		panic("BaseAdminService: next must not be nil — check wire setup")
	}
	return BaseAdminService{next: next}
}

func (b *BaseAdminService) List(ctx context.Context, filter companydto.CompanyFilter, limit int) (companydto.CompanyAdminListResult, error) {
	return b.next.List(ctx, filter, limit)
}

func (b *BaseAdminService) Get(ctx context.Context, id string) (*companydto.CompanyAdminView, error) {
	return b.next.Get(ctx, id)
}

func (b *BaseAdminService) Create(ctx context.Context, public companydto.CompanyPublicInput, settings companydto.CompanyScanSettingsInput) (*companydto.CompanyAdminView, error) {
	return b.next.Create(ctx, public, settings)
}

func (b *BaseAdminService) UpdatePublic(ctx context.Context, id string, p companydto.CompanyPublicInput) error {
	return b.next.UpdatePublic(ctx, id, p)
}

func (b *BaseAdminService) UpdateScanSettings(ctx context.Context, id string, p companydto.CompanyScanSettingsInput) error {
	return b.next.UpdateScanSettings(ctx, id, p)
}

func (b *BaseAdminService) Delete(ctx context.Context, id string) error {
	return b.next.Delete(ctx, id)
}

func (b *BaseAdminService) SetAllActive(ctx context.Context, active bool) error {
	return b.next.SetAllActive(ctx, active)
}

func (b *BaseAdminService) GetByIDs(ctx context.Context, ids []string) ([]companydto.CompanyAdminView, error) {
	return b.next.GetByIDs(ctx, ids)
}

func (b *BaseAdminService) GetAll(ctx context.Context) ([]companydto.CompanyAdminView, error) {
	return b.next.GetAll(ctx)
}
