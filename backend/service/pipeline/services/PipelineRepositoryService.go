package services

import (
	"context"
	"database/sql"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	pipelinedto "github.com/trushilpatel/jobradar/service/pipeline/dto"
	"gorm.io/gorm"
)

// PipelineStatsWindow bounds how far back the "recent" throughput/error
// counts look. Passed in as a bound parameter (rather than a DB-side NOW())
// so the same query is portable across the Postgres (prod) and SQLite (unit
// test) dialects and so tests can seed deterministic fixtures.
const PipelineStatsWindow = 24 * time.Hour

// pipelineDBStatsRow is an internal scan struct that carries GORM column
// tags. PipelineStats (the DTO) is kept ORM-agnostic; the mapping happens
// here at the repo boundary.
type pipelineDBStatsRow struct {
	TotalCompanies     int64 `gorm:"column:total_companies"`
	ActiveCompanies    int64 `gorm:"column:active_companies"`
	Queued             int64 `gorm:"column:queued"`
	Crawling           int64 `gorm:"column:crawling"`
	NewJobs24h         int64 `gorm:"column:new_jobs_24h"`
	Errors24h          int64 `gorm:"column:errors_24h"`
	AwaitingFilter     int64 `gorm:"column:awaiting_filter"`
	AwaitingEnrichment int64 `gorm:"column:awaiting_enrichment"`
}

// pipelineStatsQuery computes a live snapshot in a single round trip. Every
// count is a scalar subquery (no shared FROM clause) so the statement stays
// valid on both Postgres and SQLite — no dialect-specific FILTER clause or
// NOW() call. `awaiting_filter`/`awaiting_enrichment` mirror the retired
// mv_pipeline_db_stats view's logic: a ledger row for a stage with no
// matching `jobs` row yet means the job is still stuck at that stage.
const pipelineStatsQuery = `
SELECT
	(SELECT COUNT(*) FROM companies) AS total_companies,
	(SELECT COUNT(*)
	   FROM companies c
	   LEFT JOIN company_scan_settings css ON css.company_id = c.id
	  WHERE css.active) AS active_companies,
	(SELECT COUNT(*) FROM company_scan_queue WHERE status = 'queued') AS queued,
	(SELECT COUNT(*) FROM company_scan_queue WHERE status = 'running') AS crawling,
	(SELECT COUNT(*) FROM jobs WHERE added_at >= @since) AS new_jobs_24h,
	(SELECT COUNT(*) FROM job_pipeline_ledger
	  WHERE reason = 'error' AND first_seen_at >= @since) AS errors_24h,
	(SELECT COUNT(*)
	   FROM job_pipeline_ledger lp
	   LEFT JOIN jobs jt ON jt.job_post_id = lp.job_id
	  WHERE lp.reason = 'fetch' AND jt.job_post_id IS NULL) AS awaiting_filter,
	(SELECT COUNT(*)
	   FROM job_pipeline_ledger lf
	   LEFT JOIN jobs jt2 ON jt2.job_post_id = lf.job_id
	  WHERE lf.reason = 'filtered' AND jt2.job_post_id IS NULL) AS awaiting_enrichment
`

// PipelineStore is the data-access contract for pipeline health stats.
// Defined here so the repository implementation stays testable via a fake
// store without a real DB connection.
type PipelineStore interface {
	GetPipelineDBStats(ctx context.Context, since time.Time) (pipelineDBStatsRow, error)
}

// pipelineStore is the GORM implementation of PipelineStore.
type pipelineStore struct {
	db *gorm.DB
}

// NewPipelineStore constructs the GORM store. Exported so test packages can
// instantiate it with an in-memory SQLite DB.
func NewPipelineStore(db *gorm.DB) PipelineStore {
	return &pipelineStore{db: db}
}

func (s *pipelineStore) GetPipelineDBStats(ctx context.Context, since time.Time) (pipelineDBStatsRow, error) {
	var row pipelineDBStatsRow
	err := s.db.WithContext(ctx).Raw(pipelineStatsQuery, sql.Named("since", since)).Scan(&row).Error
	return row, err
}

// pipelineRepositoryService is the terminal pipeline-domain layer.
type pipelineRepositoryService struct {
	store PipelineStore
}

// NewPipelineRepositoryService constructs the terminal PipelineService
// implementation. Returning the interface keeps higher layers
// dependency-inverted at compile time.
func NewPipelineRepositoryService(store PipelineStore) PipelineService {
	return &pipelineRepositoryService{store: store}
}

func (r *pipelineRepositoryService) GetStats(ctx context.Context) (_ pipelinedto.PipelineStats, err error) {
	ctx, span := tracing.Start(ctx, "pipeline.store.GetPipelineDBStats")
	defer tracing.End(span, &err)

	since := time.Now().UTC().Add(-PipelineStatsWindow)
	row, err := r.store.GetPipelineDBStats(ctx, since)
	if err != nil {
		return pipelinedto.PipelineStats{}, domainerrs.ErrInternal.WithMessage(
			"pipeline.repository.GetStats: " + err.Error(),
		)
	}
	return pipelinedto.PipelineStats{
		TotalCompanies:     row.TotalCompanies,
		ActiveCompanies:    row.ActiveCompanies,
		Queued:             row.Queued,
		Crawling:           row.Crawling,
		AwaitingFilter:     row.AwaitingFilter,
		AwaitingEnrichment: row.AwaitingEnrichment,
		NewJobs24h:         row.NewJobs24h,
		Errors24h:          row.Errors24h,
	}, nil
}

// TriggerRetry is a validated no-op: NATS JetStream's BackOff/MaxDeliver
// redelivers automatically, so there is nothing to drain. The (worker, tier)
// pair is already validated by PipelineValidationService by the time a call
// reaches here.
func (r *pipelineRepositoryService) TriggerRetry(
	_ context.Context,
	_ pipelinedto.TriggerRetryInput,
) (pipelinedto.TriggerRetryResult, error) {
	return pipelinedto.TriggerRetryResult{Drained: 0}, nil
}

// Compile-time assertion: pipelineRepositoryService implements PipelineService.
var _ PipelineService = (*pipelineRepositoryService)(nil)
