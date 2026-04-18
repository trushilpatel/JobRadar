package job

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/trushilpatel/jobradar/infra/tracing"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/pipeline/ats"
	"github.com/trushilpatel/jobradar/pipeline/llm"
	"github.com/trushilpatel/jobradar/pipeline/techtag"
	"gorm.io/datatypes"
)

// EnrichedJobInput carries all data the analyzer has enriched before writing to DB.
// It bundles the original RawJob from the ATS and the LLM's structured extraction result.
type EnrichedJobInput struct {
	Raw      ats.RawJob
	Enriched *llm.EnrichResult
}

// pipelineLedger is the narrow ledger access the pipeline service needs.
type pipelineLedger interface {
	HasSeen(ctx context.Context, jobID string) (bool, error)
	UpdateSeenReason(ctx context.Context, jobID, reason string) error
}

// PipelineService is the interface the analyzer worker depends on.
// It hides all store interactions behind a single seam.
type PipelineService interface {
	// SaveEnrichedJob inserts the job into job_feed, writes to job_analytics snapshot,
	// and marks the pipeline ledger entry as done.
	// The caller is responsible for eligibility filtering before calling this method.
	SaveEnrichedJob(ctx context.Context, input EnrichedJobInput) error

	// MarkLedgerError marks a pipeline ledger entry with reason='error'.
	MarkLedgerError(ctx context.Context, jobID string) error

	// MarkLedgerIrrelevant marks a pipeline ledger entry with reason='llm_irrelevant'.
	MarkLedgerIrrelevant(ctx context.Context, jobID string) error

	// HasSeenJob returns true if the job has already been processed (dedup check).
	HasSeenJob(ctx context.Context, jobID string) (bool, error)
}

type pipelineService struct {
	jobs     JobWriter
	ledger   pipelineLedger
	snapshot SnapshotWriter
	techTags TechTagWriter
}

// NewPipelineService constructs a PipelineService backed by the provided stores.
func NewPipelineService(jobs JobWriter, ledger pipelineLedger, snapshot SnapshotWriter, techTags TechTagWriter) PipelineService {
	return &pipelineService{jobs: jobs, ledger: ledger, snapshot: snapshot, techTags: techTags}
}

// SaveEnrichedJob builds the job row, inserts it into job_feed, writes the analytics
// snapshot (non-fatal on failure), and marks the pipeline ledger as seen.
func (p *pipelineService) SaveEnrichedJob(ctx context.Context, input EnrichedJobInput) (err error) {
	ctx, span := tracing.Start(ctx, "job.pipeline.SaveEnrichedJob")
	defer tracing.End(span, &err)
	row, err := buildJobRow(input)
	if err != nil {
		return fmt.Errorf("build job row: %w", err)
	}

	if err := p.jobs.InsertJob(ctx, row); err != nil {
		return fmt.Errorf("insert job: %w", err)
	}

	// Non-fatal: snapshot write failure is logged by the caller — job is already saved.
	_ = p.snapshot.InsertSnapshot(ctx, SnapshotInput{
		ID:                row.ID,
		CompanyID:         input.Raw.CompanyID,
		Country:           input.Enriched.Country,
		State:             input.Enriched.State,
		Remote:            input.Enriched.Remote,
		Sponsorship:       input.Enriched.Sponsorship,
		RequiresCitizen:   input.Enriched.RequiresCitizen,
		SecurityClearance: input.Enriched.SecurityClearance,
		EmployType:        input.Raw.EmployType,
		ExperienceYears:   input.Enriched.ExperienceYears,
		TechStack:         []byte(row.TechStack), // already marshalled by buildJobRow
		RoleCategory:      input.Enriched.RoleCategory,
	})

	// Non-fatal: write validated+normalized tech tags for trend analysis.
	if cleaned := techtag.Clean(input.Enriched.TechStack); len(cleaned) > 0 {
		_ = p.techTags.InsertTechTags(ctx, row.ID, input.Raw.CompanyID, cleaned, time.Now())
	}

	if err := p.ledger.UpdateSeenReason(ctx, input.Raw.ID, model.SeenReasonFetch); err != nil {
		return fmt.Errorf("mark ledger seen: %w", err)
	}

	return nil
}

// MarkLedgerError marks the pipeline ledger entry for jobID with reason='error'.
func (p *pipelineService) MarkLedgerError(ctx context.Context, jobID string) (err error) {
	ctx, span := tracing.Start(ctx, "job.pipeline.MarkLedgerError")
	defer tracing.End(span, &err)
	return p.ledger.UpdateSeenReason(ctx, jobID, model.SeenReasonError)
}

// MarkLedgerIrrelevant marks the pipeline ledger entry for jobID with reason='llm_irrelevant'.
func (p *pipelineService) MarkLedgerIrrelevant(ctx context.Context, jobID string) (err error) {
	ctx, span := tracing.Start(ctx, "job.pipeline.MarkLedgerIrrelevant")
	defer tracing.End(span, &err)
	return p.ledger.UpdateSeenReason(ctx, jobID, model.SeenReasonLLMIrrelevant)
}

// HasSeenJob returns true if the job has already been processed.
func (p *pipelineService) HasSeenJob(ctx context.Context, jobID string) (_ bool, err error) {
	ctx, span := tracing.Start(ctx, "job.pipeline.HasSeenJob")
	defer tracing.End(span, &err)
	return p.ledger.HasSeen(ctx, jobID)
}

// buildJobRow maps EnrichedJobInput into a model.Job ready for DB insert.
// The field mapping mirrors buildJob in the analyzer worker exactly.
func buildJobRow(input EnrichedJobInput) (*model.Job, error) {
	raw := input.Raw
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

	return &model.Job{
		ID:                uuid.New().String(),
		JobPostID:         raw.ID,
		CompanyID:         raw.CompanyID,
		Title:             raw.Title,
		Location:          nullableStr(raw.Location),
		Remote:            enriched.Remote,
		Country:           nullableStr(enriched.Country),
		State:             nullableStr(enriched.State),
		URL:               raw.URL,
		EmployType:        nullableStr(raw.EmployType),
		ExperienceYears:   enriched.ExperienceYears,
		TechStack:         datatypes.JSON(techStack),
		SalaryRange:       datatypes.JSON(salaryRange),
		LLMSummary:        nullableStr(enriched.Summary),
		Sponsorship:       enriched.Sponsorship,
		RequiresCitizen:   enriched.RequiresCitizen,
		SecurityClearance: enriched.SecurityClearance,
		RoleBrief:         nullableStr(enriched.RoleBrief),
		Responsibilities:  datatypes.JSON(responsibilities),
		Status:            model.JobStatusNew,
	}, nil
}

// nullableStr returns nil if s is empty, otherwise returns a pointer to s.
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
