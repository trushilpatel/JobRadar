// Package company wires the two decorator chains exposed by the company
// slice (public + admin) plus the standalone ScanSettings repository.
//
// Decorator chains (outer → inner):
//
//	public:
//	  RestRouter → RestService → ValidationService → CacheService
//	    → ImplService → RepositoryService
//
//	admin:
//	  AdminRestRouter → AdminRestService → AdminValidationService
//	    → AdminImplService(+cache invalidation) → AdminRepositoryService
//
// Both chains share the same *cache.Cache and the same TTL pulled from
// MultiUserConfig.CacheTTLCompanySeconds. The admin chain calls
// cache.Cache.Delete on company:public:{id} after every mutating method.
// (In production neither cache is actually wired — see company/CLAUDE.md.)
//
// NOTE: neither ProviderSet below is ever fed through Wire's codegen — Wire
// can't auto-resolve a linear chain of providers that all return the same
// CompanyService (or CompanyAdminService) interface. See apps/api/CLAUDE.md;
// the actual wiring is hand-maintained in apps/api/wire_gen.go.
package company

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/company/services"
)

// PublicProviderSet wires the public read-only chain.
var PublicProviderSet = wire.NewSet(
	services.NewRestRouter,
	services.NewRestService,
	services.NewValidationService,
	services.NewCacheService,
	services.NewImplService,
	services.NewRepositoryService,
)

// AdminProviderSet wires the admin chain plus the ScanSettings repository.
var AdminProviderSet = wire.NewSet(
	services.NewAdminRestRouter,
	services.NewAdminRestService,
	services.NewAdminValidationService,
	services.NewAdminImplService,
	services.NewAdminRepositoryService,
	services.NewScanSettingsRepositoryService,
)

// ProviderSet is the union of the two chains — the API binary depends on
// it for one-shot wiring.
var ProviderSet = wire.NewSet(
	PublicProviderSet,
	AdminProviderSet,
)
