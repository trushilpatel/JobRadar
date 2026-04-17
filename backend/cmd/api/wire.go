//go:build wireinject

package main

import (
	"context"

	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/api"
	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/service/application"
	"github.com/trushilpatel/jobradar/service/company"
	"github.com/trushilpatel/jobradar/service/dashboard"
	"github.com/trushilpatel/jobradar/service/insights"
	"github.com/trushilpatel/jobradar/service/job"
	"github.com/trushilpatel/jobradar/service/scan"
	"gorm.io/gorm"
)

func provideCompanyConfig(cfg *config.Config) company.Config {
	return company.Config{ScanIntervalMinutes: cfg.ScanIntervalMinutes}
}

func provideScanConfig(cfg *config.Config) scan.Config {
	return scan.Config{ScanIntervalMinutes: cfg.ScanIntervalMinutes}
}

// provideScanLogReader binds ScanLogStore (returned by NewLogStore) to the ScanLogReader interface
// needed by dashboard.Service.
func provideScanLogReader(s scan.ScanLogStore) scan.ScanLogReader {
	return s
}

// provideSeenLedgerWriter binds SeenLedgerStore (returned by NewSeenLedgerStore) to the
// SeenLedgerWriter interface needed by job.Service.
func provideSeenLedgerWriter(s job.SeenLedgerStore) job.SeenLedgerWriter {
	return s
}

// jobCountReaderAdapter wraps job.JobStore to satisfy the dashboard.JobCountReader interface.
type jobCountReaderAdapter struct{ store job.JobStore }

func (a *jobCountReaderAdapter) GetJobCounts() (int64, int64, int64, error) {
	counts, err := a.store.GetJobCounts(context.Background())
	if err != nil {
		return 0, 0, 0, err
	}
	return counts.New, counts.Saved, counts.Applied, nil
}

// provideJobCountReader binds JobStore (returned by NewJobStore) to the JobCountReader
// interface needed by dashboard.Service.
func provideJobCountReader(s job.JobStore) dashboard.JobCountReader {
	return &jobCountReaderAdapter{store: s}
}

func InitializeApp(db *gorm.DB, cfg *config.Config) (*api.App, error) {
	wire.Build(
		provideCompanyConfig,
		provideScanConfig,
		provideScanLogReader,
		provideSeenLedgerWriter,
		provideJobCountReader,
		company.ProviderSet,
		scan.ProviderSet,
		job.ProviderSet,
		application.ProviderSet,
		dashboard.ProviderSet,
		insights.ProviderSet,
		api.NewApp,
	)
	return nil, nil
}
