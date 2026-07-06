package crawlernode

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
)

// ProviderSet is the Wire provider set for the crawlernode domain.
//
// Chain:
//
//	NodeRepositoryService      (NodeStore — terminal)
//	AdapterDelayGate           (per-adapter inter-company delay cooldown; needs an ats.AtsService)
//	AssignmentRepositoryService (AssignmentStore — terminal, needs an *AdapterDelayGate)
//	LedgerRepositoryService    (LedgerStore — terminal, wraps job.LedgerService)
//	ScanLogRepositoryService   (ScanLogStore — terminal, wraps scan.ScanLogWriter)
//	ScanSettingsReader         (ScanSettingsReader — terminal)
//	  → ImplService             (NodeService — business logic)
//	    → ValidationService     (NodeService — input validation)
//	      → NodeRestService     (HTTP handler — node-facing endpoints)
//	      → AdminRestService    (HTTP handler — admin endpoints)
//	        → NodeRestRouter    (route registration)
//	        → AdminRestRouter   (route registration)
//
// Note: NewImplService takes a tokenSecret string parameter. Wire callers
// must bind this via a named type provider or wire.Value at the app layer.
// NewAdapterDelayGate needs an ats/services.AtsService — wire callers must
// provide one (see ats.ProviderSet or the manual chain in
// apps/orchestrator/main.go).
//
// NOTE: this ProviderSet is never fed through Wire's codegen and isn't
// consumed by apps/orchestrator/ at all — see crawlernode/CLAUDE.md and
// apps/api/CLAUDE.md.
var ProviderSet = wire.NewSet(
	services.NewNodeRestRouter,
	services.NewAdminRestRouter,
	services.NewNodeRestService,
	services.NewAdminRestService,
	services.NewValidationService,
	services.NewImplService,
	services.NewNodeRepositoryService,
	services.NewAdapterDelayGate,
	services.NewAssignmentRepositoryService,
	services.NewLedgerRepositoryService,
	services.NewScanLogRepositoryService,
	services.NewScanSettingsReader,
)
