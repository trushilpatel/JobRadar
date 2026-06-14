package services

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"github.com/trushilpatel/jobradar/service/job/techtag"
	"gorm.io/datatypes"
)

// pipelineLedgerSink is the narrow subset of SeenLedgerStore the pipeline
// service needs — keeping it small lets the analyzer mock a 2-method
// double instead of the full ledger surface.
type pipelineLedgerSink interface {
	HasSeen(ctx context.Context, jobID string) (bool, error)
	UpdateSeenReason(ctx context.Context, jobID string, reason jobcommon.SeenReason) error
}

type pipelineService struct {
	jobs     JobWriter
	ledger   pipelineLedgerSink
	snapshot SnapshotWriter
	techTags TechTagWriter
	logger   logger.Logger
}

// NewPipelineService constructs a PipelineService backed by the provided
// stores. The ledger argument can be any type satisfying the narrow
// HasSeen + UpdateSeenReason pair (the full SeenLedgerStore qualifies).
func NewPipelineService(jobs JobWriter, ledger pipelineLedgerSink, snapshot SnapshotWriter, techTags TechTagWriter, log logger.Logger) PipelineService {
	return &pipelineService{jobs: jobs, ledger: ledger, snapshot: snapshot, techTags: techTags, logger: log}
}

// SaveEnrichedJob builds the job row, inserts it into `jobs`, writes the analytics
// snapshot (non-fatal on failure), and marks the pipeline ledger as seen.
func (p *pipelineService) SaveEnrichedJob(ctx context.Context, input dto.EnrichedJobInput) (err error) {
	ctx, span := tracing.Start(ctx, "job.pipeline.SaveEnrichedJob")
	defer tracing.End(span, &err)
	row, err := buildJobRow(input)
	if err != nil {
		return fmt.Errorf("build job row: %w", err)
	}

	if err := p.jobs.InsertJob(ctx, row); err != nil {
		// Duplicate job_post_id (a crawler→filter→enricher replay). The
		// authoritative row already exists, so skip the snapshot + tech-tag
		// writes (they key off row.ID, a UUID that was never persisted) and
		// still advance the ledger to 'saved' so it reflects terminal state.
		if stderrors.Is(err, domainerrs.ErrJobAlreadyExists) {
			metrics.EnricherJobsDuplicate.WithLabelValues(input.Summary.Adapter).Inc()
			p.logger.Info("enriched job already exists; skipping snapshot + tech-tags",
				logger.Str(logger.FieldJobID, input.Summary.ID),
				logger.Str(logger.FieldAdapter, input.Summary.Adapter))
			return p.markSavedTolerant(ctx, input.Summary.ID)
		}
		return fmt.Errorf("insert job: %w", err)
	}

	// Non-fatal: snapshot write failure is logged but does not block the saved job.
	if err := p.snapshot.InsertSnapshot(ctx, dto.SnapshotInput{
		ID:                row.ID,
		CompanyID:         input.Summary.CompanyID,
		Country:           input.Enriched.Country,
		State:             input.Enriched.State,
		Remote:            input.Enriched.Remote,
		Sponsorship:       input.Enriched.Sponsorship,
		RequiresCitizen:   input.Enriched.RequiresCitizen,
		SecurityClearance: input.Enriched.SecurityClearance,
		EmployType:        employType(input),
		ExperienceYears:   input.Enriched.ExperienceYears,
		TechStack:         []byte(row.TechStack), // already marshalled by buildJobRow
		RoleCategory:      input.Enriched.RoleCategory,
	}); err != nil {
		p.logger.Warn("snapshot insert failed", logger.Str(logger.FieldJobID, row.ID), logger.Err(err))
	}

	// Non-fatal: write validated+normalized tech tags for trend analysis.
	if cleaned := techtag.Clean(input.Enriched.TechStack); len(cleaned) > 0 {
		if err := p.techTags.InsertTechTags(ctx, row.ID, input.Summary.CompanyID, cleaned, time.Now()); err != nil {
			p.logger.Warn("tech tags insert failed", logger.Str(logger.FieldJobID, row.ID), logger.Err(err))
		}
	}

	return p.markSavedTolerant(ctx, input.Summary.ID)
}

// markSavedTolerant advances the ledger row for jobID to 'saved'. A missing
// ledger row (cleaned out by a maintenance migration, or never written because
// the row predates the ledger) is non-fatal at this terminal stage: the job is
// already persisted, so we log WARN, increment a metric, and return nil rather
// than crashing the enricher worker.
func (p *pipelineService) markSavedTolerant(ctx context.Context, jobID string) error {
	if err := p.ledger.UpdateSeenReason(ctx, jobID, jobcommon.SeenReasonSaved); err != nil {
		if stderrors.Is(err, domainerrs.ErrLedgerRowMissing) {
			metrics.EnricherLedgerMissing.WithLabelValues(string(jobcommon.SeenReasonSaved)).Inc()
			p.logger.Warn("ledger row missing on save; continuing",
				logger.Str(logger.FieldJobID, jobID),
				logger.Str(logger.FieldReason, string(jobcommon.SeenReasonSaved)))
			return nil
		}
		return fmt.Errorf("mark ledger seen: %w", err)
	}
	return nil
}

// MarkLedgerError marks the pipeline ledger entry for jobID with reason='error'.
func (p *pipelineService) MarkLedgerError(ctx context.Context, jobID string) (err error) {
	ctx, span := tracing.Start(ctx, "job.pipeline.MarkLedgerError")
	defer tracing.End(span, &err)
	return p.ledger.UpdateSeenReason(ctx, jobID, jobcommon.SeenReasonError)
}

// MarkLedgerIrrelevant marks the pipeline ledger entry for jobID with reason='llm_irrelevant'.
func (p *pipelineService) MarkLedgerIrrelevant(ctx context.Context, jobID string) (err error) {
	ctx, span := tracing.Start(ctx, "job.pipeline.MarkLedgerIrrelevant")
	defer tracing.End(span, &err)
	return p.ledger.UpdateSeenReason(ctx, jobID, jobcommon.SeenReasonLLMIrrelevant)
}

// MarkLedgerNonUSA marks the pipeline ledger entry for jobID with reason='non_usa_location'.
func (p *pipelineService) MarkLedgerNonUSA(ctx context.Context, jobID string) (err error) {
	ctx, span := tracing.Start(ctx, "job.pipeline.MarkLedgerNonUSA")
	defer tracing.End(span, &err)
	return p.ledger.UpdateSeenReason(ctx, jobID, jobcommon.SeenReasonNonUSA)
}

// HasSeenJob returns true if the job has already been processed.
func (p *pipelineService) HasSeenJob(ctx context.Context, jobID string) (_ bool, err error) {
	ctx, span := tracing.Start(ctx, "job.pipeline.HasSeenJob")
	defer tracing.End(span, &err)
	return p.ledger.HasSeen(ctx, jobID)
}

// buildJobRow maps EnrichedJobInput into a jobmodel.Job ready for DB insert.
//
// Authority precedence: the fetched JobPost is authoritative for the display
// fields it provides (Title, Location, EmploymentType) — falling back to the
// JobSummary only when the Post field is empty. This fixes empty-title rows for
// thin-listing adapters (Meta, Workday) whose summaries lack title/location at
// listing time. URL, JobPostID, and CompanyID always come from the Summary
// (stable identity from the crawler). Derived/structured fields (Country, State,
// tech stack, salary, …) come from the LLM enrich result.
func buildJobRow(input dto.EnrichedJobInput) (*jobmodel.Job, error) {
	enriched := input.Enriched

	techStack, err := json.Marshal(enriched.TechStack)
	if err != nil {
		return nil, fmt.Errorf("marshal tech_stack: %w", err)
	}

	responsibilities, err := json.Marshal(enriched.Responsibilities)
	if err != nil {
		return nil, fmt.Errorf("marshal responsibilities: %w", err)
	}

	salaryRange, err := json.Marshal(map[string]any{
		"min":      enriched.SalaryMin,
		"max":      enriched.SalaryMax,
		"currency": enriched.SalaryCurrency,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal salary_range: %w", err)
	}

	return &jobmodel.Job{
		ID:                uuid.New().String(),
		JobPostID:         input.Summary.ID,
		CompanyID:         input.Summary.CompanyID,
		Title:             title(input),
		Location:          nullableStr(jobLocation(input)),
		Remote:            enriched.Remote,
		Country:           nullableStr(enriched.Country),
		State:             nullableStr(enriched.State),
		URL:               input.Summary.URL,
		EmployType:        nullableStr(employType(input)),
		ExperienceYears:   enriched.ExperienceYears,
		TechStack:         datatypes.JSON(techStack),
		SalaryRange:       datatypes.JSON(salaryRange),
		LLMSummary:        nullableStr(enriched.Summary),
		Sponsorship:       enriched.Sponsorship,
		RequiresCitizen:   enriched.RequiresCitizen,
		SecurityClearance: enriched.SecurityClearance,
		RoleBrief:         nullableStr(enriched.RoleBrief),
		Responsibilities:  datatypes.JSON(responsibilities),
	}, nil
}

// title returns the authoritative job title: the fetched Post.Title when
// non-empty (after trimming), otherwise the Summary.Title. Thin-listing
// adapters publish empty summary titles, so the Post value wins.
func title(input dto.EnrichedJobInput) string {
	return preferPost(input.Post.Title, input.Summary.Title)
}

// jobLocation returns the authoritative location: Post.Location when non-empty,
// otherwise Summary.Location.
func jobLocation(input dto.EnrichedJobInput) string {
	return preferPost(input.Post.Location, input.Summary.Location)
}

// employType returns the authoritative employment type: Post.EmploymentType
// when non-empty, otherwise Summary.EmployType.
func employType(input dto.EnrichedJobInput) string {
	return preferPost(input.Post.EmploymentType, input.Summary.EmployType)
}

// preferPost returns the trimmed post value when it is non-empty, otherwise the
// summary fallback. Encodes the JobPost-over-JobSummary authority precedence.
func preferPost(post, summary string) string {
	if trimmed := strings.TrimSpace(post); trimmed != "" {
		return trimmed
	}
	return summary
}

// nullableStr returns nil if s is empty, otherwise returns a pointer to s.
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
