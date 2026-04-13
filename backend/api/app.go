package api

import (
	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/api/handler"
	"github.com/trushilpatel/jobradar/store"
	"gorm.io/gorm"
)

// App holds all stores constructed from the database.
// It is the single place where store wiring lives — main.go never touches stores.
type App struct {
	Jobs      store.JobStore
	Ledger    store.SeenLedgerStore
	Discard   store.DiscardStore
	Snapshot  store.SnapshotWriter
	Apps      store.ApplicationStore
	Companies store.CompanyStore
	ScanLog   store.CompanyScanLogStore
	Insights  store.InsightsReader
	ScanQueue store.ScanQueueStore

	scanIntervalMinutes int
}

// NewApp constructs all stores from db and cfg.
func NewApp(db *gorm.DB, cfg *config.Config) *App {
	return &App{
		Jobs:                store.NewJobStore(db),
		Ledger:              store.NewSeenLedgerStore(db),
		Discard:             store.NewDiscardStore(db),
		Snapshot:            store.NewSnapshotStore(db),
		Apps:                store.NewApplicationStore(db),
		Companies:           store.NewCompanyStore(db),
		ScanLog:             store.NewCompanyScanLogStore(db),
		Insights:            store.NewInsightsStore(db),
		ScanQueue:           store.NewScanQueueStore(db),
		scanIntervalMinutes: cfg.ScanIntervalMinutes,
	}
}

// Handlers returns all route groups.
// To add a new resource: add its store to App, construct it here, done.
// router.go and main.go never change.
func (a *App) Handlers() []handler.RouteGroup {
	return []handler.RouteGroup{
		handler.NewJobHandler(a.Jobs, a.Ledger, a.Discard, a.Snapshot),
		handler.NewApplicationHandler(a.Apps, a.Jobs),
		handler.NewCompanyHandler(a.Companies, a.ScanLog, a.ScanQueue, a.scanIntervalMinutes),
		handler.NewDashboardHandler(a.Jobs, a.ScanLog, a.Insights),
		handler.NewInsightsHandler(a.Insights),
	}
}
