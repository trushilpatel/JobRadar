package main

import (
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	orchmiddleware "github.com/trushilpatel/jobradar/apps/orchestrator/middleware"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/cache"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
	crawlernodesvc "github.com/trushilpatel/jobradar/service/crawlernode/services"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
	"gorm.io/gorm"
)

// atsCacheTTL matches the TTL apps/api/wire_gen.go uses for the same
// AtsService read-through cache — inter_company_delay_s changes rarely
// enough that a shared 5-minute staleness window is acceptable here too.
const atsCacheTTL = 5 * time.Minute

// wiredApp bundles the route groups and background-ticker dependency main()
// needs after wiring. Pure composition — see wireApp.
type wiredApp struct {
	nodeRouter  handler.RouteGroup
	adminGate   handler.RouteGroup
	assignStore crawlernodesvc.AssignmentStore
}

// wireApp composes the crawlernode decorator chain, both REST routers, and
// the admin auth gate. Used by both main() (real NATS publisher, real DB)
// and genspec.go's buildSpecYAML (noop publisher, in-memory DB) so route
// registration — and therefore the generated OpenAPI spec — can never drift
// from what main() actually wires.
func wireApp(
	db *gorm.DB,
	cfg *config.Config,
	log logger.Logger,
	publisher crawlernodesvc.StreamPublisher,
	authErr error,
	nodeZoneCfg huma.Config,
	adminZoneCfg huma.Config,
) *wiredApp {
	nodeStore := crawlernodesvc.NewNodeRepositoryService(db)

	// AtsService chain (mirrors apps/api/wire_gen.go's repository->impl->cache
	// chain, minus the validation/REST layers this internal caller has no use
	// for) backs the AdapterDelayGate's inter_company_delay_s lookups.
	atsStore := atssvc.NewAtsStore(db)
	atsRepositoryService := atssvc.NewRepositoryService(atsStore)
	atsImplService := atssvc.NewImplService(atsRepositoryService)
	atsService := atssvc.NewCacheService(atsImplService, cache.NewMemoryCache(), atsCacheTTL)

	delayGate := crawlernodesvc.NewAdapterDelayGate(atsService)
	assignStore := crawlernodesvc.NewAssignmentRepositoryService(db, delayGate)
	seenLedger := jobsvc.NewSeenLedgerStore(db)
	ledgerSvc := jobsvc.NewLedgerService(seenLedger)
	ledgerStore := crawlernodesvc.NewLedgerRepositoryService(ledgerSvc)
	logStore := scansvc.NewLogStore(db)
	scanLogStore := crawlernodesvc.NewScanLogRepositoryService(logStore)
	scanSettings := crawlernodesvc.NewScanSettingsReader(db)

	implSvc := crawlernodesvc.NewImplService(
		nodeStore, assignStore, ledgerStore, scanLogStore,
		publisher, scanSettings, cfg.Crawler.CrawlerTokenSecret,
	)
	nodeSvc := crawlernodesvc.NewValidationService(implSvc)

	nodeRestSvc := crawlernodesvc.NewNodeRestService(nodeSvc)
	adminRestSvc := crawlernodesvc.NewAdminRestService(nodeSvc)
	authMw := orchmiddleware.RequireNodeToken(nodeStore, cfg.Crawler.CrawlerTokenSecret)
	nodeRouter := crawlernodesvc.NewNodeRestRouter(nodeRestSvc, authMw, nodeZoneCfg)
	adminRouter := crawlernodesvc.NewAdminRestRouter(adminRestSvc, adminZoneCfg)

	adminGate := wireAdminGate(db, cfg, log, authErr, adminRouter)

	return &wiredApp{nodeRouter: nodeRouter, adminGate: adminGate, assignStore: assignStore}
}
