// Package audit is the feature-slice root for the audit domain. Phase 1
// wires Impl → Repository — no validation or REST decorator ships yet
// because the audit log is consumed by middleware and (later) an admin UI,
// neither of which need the validation layer.
//
// NOTE: this ProviderSet documents the intended composition only — it is
// never fed through Wire's codegen; see apps/api/CLAUDE.md.
package audit

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/audit/services"
)

// ProviderSet is the Wire provider set for the audit domain.
var ProviderSet = wire.NewSet(
	services.NewImplService,
	services.NewRepositoryService,
)
