//go:build wireinject

package main

import (
	"context"

	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/service/application"
	appsvc "github.com/trushilpatel/jobradar/service/application/services"
	"github.com/trushilpatel/jobradar/service/company"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	"github.com/trushilpatel/jobradar/service/dashboard"
	dashsvc "github.com/trushilpatel/jobradar/service/dashboard/services"
	insightssvc "github.com/trushilpatel/jobradar/service/insights/services"
	"github.com/trushilpatel/jobradar/service/job"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	"github.com/trushilpatel/jobradar/service/scan"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
	"gorm.io/gorm"
)

// noopLagReader returns 0 for all consumer lag queries. NATS JetStream handles
// retries via BackOff/MaxDeliver on durable consumers — there are no separate
// retry topics to monitor, so lag tracking via the old Kafka model is not applicable.
type noopLagReader struct{}

func (*noopLagReader) GetLag(_ context.Context, _, _ string) (int64, error) { return 0, nil }

// noopRetryDrainer is a no-op drain. NATS redelivers messages automatically via
// consumer BackOff — there is no separate retry topic to drain back to main.
type noopRetryDrainer struct{}

func (*noopRetryDrainer) Drain(_ context.Context, _, _, _ string) (int64, error) { return 0, nil }

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
func provideScanHistoryReader(s scansvc.ScanLogStore) companysvc.ScanHistoryReader {
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

func provideLagReader(_ *config.Config) (kafka.LagReader, error) {
	return &noopLagReader{}, nil
}

func provideRetryDrainer(_ *config.Config) (kafka.RetryDrainer, error) {
	return &noopRetryDrainer{}, nil
}

func InitializeApp(db *gorm.DB, cfg *config.Config, log logger.Logger) (*App, error) {
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
