// Package role is the feature-slice root for the role domain. Phase 1 wires
// a shallow chain (Validation → Impl → Repository) — no admin UI ships in
// this phase, so neither cache nor REST decorator is needed.
package role

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/role/services"
)

// ProviderSet is the Wire provider set for the role domain.
var ProviderSet = wire.NewSet(
	services.NewRepositoryService,
	services.NewImplService,
	services.NewValidationService,
)
