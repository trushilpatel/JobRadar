// Package audit is the feature-slice root for the audit domain. Phase 1
// wires Impl → Repository — no validation or REST decorator ships yet
// because the audit log is consumed by middleware and (later) an admin UI,
// neither of which need the validation layer.
package audit

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/audit/services"
)

// ProviderSet is the Wire provider set for the audit domain.
var ProviderSet = wire.NewSet(
	services.NewRepositoryService,
	services.NewImplService,
)
