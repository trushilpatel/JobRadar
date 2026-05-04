package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/trushilpatel/jobradar/apps/analyzer/techtag"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	"github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	"gorm.io/datatypes"
)

// pipelineLedgerSink is the narrow subset of SeenLedgerStore the pipeline
// service needs — keeping it small lets the analyzer mock a 2-method
// double instead of the full ledger surface.
type pipelineLedgerSink interface {
	HasSeen(ctx context.Context, jobID string) (bool, error)
	UpdateSeenReason(ctx context.Context, jobID, reason string) error
}

type pipelineService struct {
	jobs     JobWriter
	ledger   pipelineLedgerSink
	snapshot SnapshotWriter
	techTags TechTagWriter
}

// NewPipelineService constructs a PipelineService backed by the provided
// stores. The ledger argument can be any type satisfying the narrow
// HasSeen + UpdateSeenReason pair (the full SeenLedgerStore qualifies).
func NewPipelineService(jobs JobWriter, ledger pipelineLedgerSink, snapshot SnapshotWriter, techTags TechTagWriter) PipelineService {
	return &pipelineService{jobs: jobs, ledger: ledger, snapshot: snapshot, techTags: techTags}
}

// SaveEnrichedJob builds the job row, inserts it into job_feed, writes the analytics
// snapshot (non-fatal on failure), and marks the pipeline ledger as seen.
func (p *pipelineService) SaveEnrichedJob(ctx context.Context, input dto.EnrichedJobInput) (err error) {
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
	_ = p.snapshot.InsertSnapshot(ctx, dto.SnapshotInput{
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

	if err := p.ledger.UpdateSeenReason(ctx, input.Raw.ID, jobcommon.SeenReasonFetch); err != nil {
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

// HasSeenJob returns true if the job has already been processed.
func (p *pipelineService) HasSeenJob(ctx context.Context, jobID string) (_ bool, err error) {
	ctx, span := tracing.Start(ctx, "job.pipeline.HasSeenJob")
	defer tracing.End(span, &err)
	return p.ledger.HasSeen(ctx, jobID)
}

// buildJobRow maps EnrichedJobInput into a jobmodel.Job ready for DB insert.
func buildJobRow(input dto.EnrichedJobInput) (*jobmodel.Job, error) {
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

	return &jobmodel.Job{
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
	}, nil
}

// nullableStr returns nil if s is empty, otherwise returns a pointer to s.
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
