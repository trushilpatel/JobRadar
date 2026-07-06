// Package user is the feature-slice root for the user domain. The wire
// provider sets here document the intended composition, in canonical order:
//
//	self-service (/api/me):
//	  RestRouter → RestService → Validation → Cache → Impl → Repository
//
//	admin (/admin/users):
//	  AdminRestRouter → AdminRestService → AdminValidationService
//	    → AdminImplService(+cache invalidation) → AdminRepositoryService
//
// NOTE: neither ProviderSet is ever fed through Wire's codegen — Wire can't
// auto-resolve a linear chain of providers that all return the same Service
// (or AdminService) interface. See apps/api/CLAUDE.md; the actual wiring for
// both chains is hand-maintained in apps/api/wire_gen.go /
// apps/api/wire_multiuser.go. The /api/me middleware chain is applied at
// construction time of UserRestRouter via the variadic middleware argument.
package user

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/user/services"
)

// ProviderSet is the Wire provider set for the self-service (/api/me) chain.
var ProviderSet = wire.NewSet(
	services.NewRestRouter,
	services.NewRestService,
	services.NewValidationService,
	services.NewCacheService,
	services.NewImplService,
	services.NewRepositoryService,
)

// AdminProviderSet documents the admin (/admin/users) chain — hand-wired in
// apps/api/wire_multiuser.go, not executed via Wire (see package doc above).
var AdminProviderSet = wire.NewSet(
	services.NewAdminRestRouter,
	services.NewAdminRestService,
	services.NewAdminValidationService,
	services.NewAdminImplService,
	services.NewAdminRepositoryService,
)
