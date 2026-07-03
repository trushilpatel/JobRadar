//go:build wireinject

package main

import (
	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/service/application"
	"github.com/trushilpatel/jobradar/service/company"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	"github.com/trushilpatel/jobradar/service/job"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	"github.com/trushilpatel/jobradar/service/scan"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
	"gorm.io/gorm"
)

func provideScanConfig(cfg *config.Config) scansvc.Config {
	return scansvc.Config{ScanIntervalMinutes: cfg.Scheduler.ScanIntervalMinutes}
}

// provideScanLogReader binds ScanLogStore to the ScanLogReader interface
// needed by scan.Service.
func provideScanLogReader(s scansvc.ScanLogStore) scansvc.ScanLogReader {
	return s
}

// provideScanHistoryReader binds ScanLogStore to the ScanHistoryReader interface
// needed by company.Service.
func provideScanHistoryReader(s scansvc.ScanLogStore) companysvc.ScanHistoryReader {
	return s
}

// provideSeenLedgerWriter binds SeenLedgerStore to the SeenLedgerWriter interface
// needed by job.Service.
func provideSeenLedgerWriter(s jobsvc.SeenLedgerStore) jobsvc.SeenLedgerWriter {
	return s
}

// provideJobApplicationWriter constructs a dedicated ApplicationWriter for job.Service.
func provideJobApplicationWriter(db *gorm.DB) jobsvc.ApplicationWriter {
	return application.NewApplicationWriter(db)
}

func InitializeApp(db *gorm.DB, cfg *config.Config, log logger.Logger) (*App, error) {
	wire.Build(
		provideScanConfig,
		provideScanLogReader,
		provideScanHistoryReader,
		provideSeenLedgerWriter,
		provideJobApplicationWriter,
		company.ProviderSet,
		scan.ProviderSet,
		job.ProviderSet,
		application.ProviderSet,
		NewApp,
	)
	return nil, nil
}
