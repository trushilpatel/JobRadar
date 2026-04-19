//go:build wireinject

package main

import (
	"context"

	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/service/application"
	appsvc "github.com/trushilpatel/jobradar/service/application/services"
	"github.com/trushilpatel/jobradar/service/company"
	"github.com/trushilpatel/jobradar/service/dashboard"
	dashsvc "github.com/trushilpatel/jobradar/service/dashboard/services"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
	"github.com/trushilpatel/jobradar/service/job"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	"github.com/trushilpatel/jobradar/service/scan"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
	"gorm.io/gorm"
)

func provideKafkaConfig(cfg *config.Config) config.KafkaConfig {
	return cfg.Kafka
}

func provideCompanyConfig(cfg *config.Config) company.Config {
	return company.Config{ScanIntervalMinutes: cfg.Scheduler.ScanIntervalMinutes}
}

func provideScanConfig(cfg *config.Config) scansvc.Config {
	return scansvc.Config{ScanIntervalMinutes: cfg.Scheduler.ScanIntervalMinutes}
}

// provideScanLogReader binds ScanLogStore (returned by NewLogStore) to the ScanLogReader interface
// needed by dashboard.Service.
func provideScanLogReader(s scansvc.ScanLogStore) scansvc.ScanLogReader {
	return s
}

// provideScanHistoryReader binds ScanLogStore to the ScanHistoryReader interface
// needed by company.Service.
func provideScanHistoryReader(s scansvc.ScanLogStore) company.ScanHistoryReader {
	return s
}

// provideSeenLedgerWriter binds SeenLedgerStore (returned by NewSeenLedgerStore) to the
// SeenLedgerWriter interface needed by job.Service.
func provideSeenLedgerWriter(s jobsvc.SeenLedgerStore) jobsvc.SeenLedgerWriter {
	return s
}

// jobCountReaderAdapter wraps jobsvc.JobStore to satisfy the dashboard.JobCountReader interface.
type jobCountReaderAdapter struct{ store jobsvc.JobStore }

func (a *jobCountReaderAdapter) GetJobCounts() (int64, int64, int64, error) {
	counts, err := a.store.GetJobCounts(context.Background())
	if err != nil {
		return 0, 0, 0, err
	}
	return counts.New, counts.Saved, counts.Applied, nil
}

// provideJobCountReader binds JobStore (returned by NewJobStore) to the JobCountReader
// interface needed by dashboard.Service.
func provideJobCountReader(s jobsvc.JobStore) dashboard.JobCountReader {
	return &jobCountReaderAdapter{store: s}
}

// provideJobApplicationWriter constructs a dedicated ApplicationWriter for job.Service.
func provideJobApplicationWriter(db *gorm.DB) jobsvc.ApplicationWriter {
	return application.NewApplicationWriter(db)
}

// provideApplicationListReader binds ApplicationService to the ApplicationListReader interface
// needed by dashboard.UserDashboardService.
func provideApplicationListReader(s appsvc.ApplicationService) dashsvc.ApplicationListReader {
	return s
}

// provideLagReader creates a Kafka LagReader from the config.
func provideLagReader(cfg *config.Config) (kafka.LagReader, error) {
	return kafka.NewLagReader(cfg.Kafka)
}

func InitializeApp(db *gorm.DB, cfg *config.Config) (*App, error) {
	wire.Build(
		provideCompanyConfig,
		provideScanConfig,
		provideScanLogReader,
		provideScanHistoryReader,
		provideSeenLedgerWriter,
		provideJobCountReader,
		provideJobApplicationWriter,
		provideApplicationListReader,
		provideLagReader,
		company.ProviderSet,
		scan.ProviderSet,
		job.ProviderSet,
		application.ProviderSet,
		dashboard.ProviderSet,
		insightssvc.ProviderSet,
		NewApp,
	)
	return nil, nil
}
