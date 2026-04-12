package job_listing_crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/trushilpatel/jobradar/internal/ats_fetcher"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
)

// Engine crawls company careers pages, deduplicates jobs, and publishes new ones to Kafka.
type Engine struct {
	companies    store.CompanyReader
	ledger       store.SeenLedgerStore
	scanLog      store.CompanyScanLogWriter
	publisher    kafka.Publisher
	companyDelay time.Duration
	log          logger.Logger
}

func NewEngine(
	companies store.CompanyReader,
	ledger store.SeenLedgerStore,
	scanLog store.CompanyScanLogWriter,
	publisher kafka.Publisher,
	companyDelay time.Duration,
	log logger.Logger,
) *Engine {
	return &Engine{
		companies:    companies,
		ledger:       ledger,
		scanLog:      scanLog,
		publisher:    publisher,
		companyDelay: companyDelay,
		log:          log,
	}
}

// RunOnce runs one full scan cycle across all active companies.
func (e *Engine) RunOnce(ctx context.Context) {
	companies, err := e.companies.GetActiveCompanies()
	if err != nil {
		e.log.Error("failed to load companies", err)
		return
	}
	e.log.Info("starting scan cycle", logger.Int("active_companies", len(companies)))
	for _, company := range companies {
		e.scanCompany(ctx, company)
	}
	e.log.Info("scan cycle complete")
}

// RunForCompany runs a scan for a single company by ID.
// Used when triggered via the company.scan_request Kafka topic.
func (e *Engine) RunForCompany(ctx context.Context, companyID string) {
	company, err := e.companies.GetCompanyByID(companyID)
	if err != nil {
		e.log.Error("company not found", err, logger.Str(logger.FieldCompany, companyID))
		return
	}
	if !company.Active {
		e.log.Warn("skipping inactive company",
			logger.Str(logger.FieldCompany, company.ID),
			logger.Str("name", company.Name),
		)
		return
	}
	e.log.Debug("triggered scan",
		logger.Str(logger.FieldCompany, company.ID),
		logger.Str("name", company.Name),
	)
	e.scanCompany(ctx, company)
}

func (e *Engine) scanCompany(ctx context.Context, company model.Company) {
	start := time.Now()
	companyLog := e.log.With(
		logger.Str(logger.FieldCompany, company.ID),
		logger.Str("name", company.Name),
	)

	adp, err := ats_fetcher.Registry(company.AdapterType)
	if err != nil {
		errMsg := fmt.Sprintf("no adapter for type %q: %v", company.AdapterType, err)
		companyLog.Error("no adapter found", err)
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)
		return
	}

	result, err := adp.FetchJobs(ctx, company.ID)
	if err != nil {
		errMsg := fmt.Sprintf("fetch failed: %v", err)
		companyLog.Error("fetch jobs failed", err)
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)
		return
	}

	fetched := len(result.Jobs)
	newCount := 0

	for _, item := range result.Jobs {
		seen, err := e.ledger.HasSeen(item.ID)
		if err != nil {
			companyLog.Error("seen_ledger check failed", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}
		if seen {
			continue
		}

		if err := e.ledger.MarkSeen(item.ID, company.ID, model.SeenReasonFetch); err != nil {
			companyLog.Error("failed to mark seen", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}

		rawJob := toRawJob(item, company.ID, company.AdapterType)
		if err := e.publish(ctx, rawJob); err != nil {
			companyLog.Error("publish failed", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}

		newCount++
	}

	companyLog.Info("scan complete",
		logger.Int(logger.FieldFetched, fetched),
		logger.Int(logger.FieldNew, newCount),
		logger.Dur(logger.FieldDuration, time.Since(start).Round(time.Millisecond)),
	)
	e.recordResult(ctx, company.ID, fetched, newCount, time.Since(start), nil)

	if e.companyDelay > 0 {
		companyLog.Debug("sleeping before next company", logger.Dur("delay", e.companyDelay))
		select {
		case <-time.After(e.companyDelay):
		case <-ctx.Done():
		}
	}
}

// toRawJob converts a FetchJobsItem into the RawJob Kafka message.
// CompanyID and AdapterType come from the company record, not the listing.
func toRawJob(item ats_fetcher.FetchJobsItem, companyID, adapterType string) ats_fetcher.RawJob {
	return ats_fetcher.RawJob{
		ID:          item.ID,
		CompanyID:   companyID,
		Adapter:     adapterType,
		Title:       item.Title,
		Location:    item.Location,
		URL:         item.URL,
		EmployType:  item.EmploymentType,
		FullContent: item.FullContent,
	}
}

func (e *Engine) publish(ctx context.Context, job ats_fetcher.RawJob) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return e.publisher.Publish(ctx, kafka.TopicJobListingsRaw, payload)
}

func (e *Engine) recordResult(ctx context.Context, companyID string, fetched, newCount int, duration time.Duration, errMsg *string) {
	status := model.ScanStatusOK
	if errMsg != nil {
		status = model.ScanStatusError
	}
	durationMs := int(duration.Milliseconds())
	if err := e.scanLog.InsertCompanyScanLog(companyID, fetched, newCount, durationMs, errMsg); err != nil {
		e.log.Error("failed to write scan log", err, logger.Str(logger.FieldCompany, companyID))
	}
	if err := e.scanLog.UpdateCompanyScanStatus(companyID, status); err != nil {
		e.log.Error("failed to update company status", err, logger.Str(logger.FieldCompany, companyID))
	}
}
