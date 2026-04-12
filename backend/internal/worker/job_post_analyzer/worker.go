package job_post_analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/trushilpatel/jobradar/internal/ats_fetcher"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/llm"
	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
	"gorm.io/datatypes"
)

// enrichConcurrency is the number of jobs processed in parallel.
// Page fetches (up to 15s each) dominate latency, so concurrency here
// is the main throughput lever. LLM calls are still rate-limited globally.
const enrichConcurrency = 5

// Worker consumes jobs.raw_detail, fetches the full job page,
// runs LLM enrichment, and writes the final row to the job table.
type Worker struct {
	consumer    *kafka.Consumer
	scorer      llm.Scorer
	jobs        store.JobWriter
	ledger      store.SeenLedgerWriter
	snapshot    store.SnapshotWriter
	client      *http.Client
	log         logger.Logger
	maxExpYears int
}

func NewWorker(
	consumer *kafka.Consumer,
	scorer llm.Scorer,
	jobs store.JobWriter,
	ledger store.SeenLedgerWriter,
	snapshot store.SnapshotWriter,
	log logger.Logger,
	maxExpYears int,
) *Worker {
	return &Worker{
		consumer:    consumer,
		scorer:      scorer,
		jobs:        jobs,
		ledger:      ledger,
		snapshot:    snapshot,
		client:      &http.Client{Timeout: 15 * time.Second},
		log:         log,
		maxExpYears: maxExpYears,
	}
}

// Run starts consuming jobs.raw_detail and blocks until ctx is cancelled.
// Jobs are processed concurrently up to enrichConcurrency — page fetches
// dominate latency so parallelism here is the main throughput lever.
func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker started", logger.Int("concurrency", enrichConcurrency))

	sem := make(chan struct{}, enrichConcurrency)
	var wg sync.WaitGroup

	err := w.consumer.Poll(ctx, func(_ string, value []byte) error {
		var job ats_fetcher.RawJob
		if err := json.Unmarshal(value, &job); err != nil {
			w.log.Warn("malformed message, skipping", logger.Err(err))
			return nil
		}

		w.log.Debug("job received",
			logger.Str(logger.FieldJobID, job.ID),
			logger.Str("title", job.Title),
		)

		sem <- struct{}{} // block if all workers are busy
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			w.enrich(ctx, job)
		}()

		return nil
	})

	wg.Wait() // drain in-flight enrichments before returning
	return err
}

// enrich runs the full enrichment pipeline for a single job.
func (w *Worker) enrich(ctx context.Context, job ats_fetcher.RawJob) {
	jobLog := w.log.With(
		logger.Str(logger.FieldJobID, job.ID),
		logger.Str("title", job.Title),
	)

	// 1. Get full job content — use pre-fetched data if available,
	// otherwise fetch via native API or HTTP fallback.
	var content ats_fetcher.FetchJobContentResponse
	if job.FullContent != nil {
		content = fullContentToResponse(job)
	} else {
		var err error
		content, err = w.fetchContent(ctx, job)
		if err != nil {
			jobLog.Error("fetch content failed", err)
			w.markError(job.ID)
			return
		}
	}

	// Serialize the structured content to JSON for the LLM.
	// JSON gives the LLM all available fields (description, compensation, location, etc.)
	// in a compact, consistent format regardless of which platform fetched the job.
	rawText, err := json.Marshal(content)
	if err != nil {
		jobLog.Error("marshal content failed", err)
		w.markError(job.ID)
		return
	}

	// 2. Send to LLM for structured extraction
	result, err := w.scorer.Enrich(ctx, llm.EnrichInput{
		ID:      job.ID,
		RawText: string(rawText),
	})
	if err != nil {
		jobLog.Error("LLM enrich failed", err)
		w.markError(job.ID)
		return
	}

	// 3. Drop ineligible jobs before writing to DB
	if reason := w.dropJob(result); reason != "" {
		jobLog.Info("job dropped", logger.Str("reason", reason))
		w.markError(job.ID)
		return
	}

	// 4. Build the DB row
	row, err := buildJob(job, result)
	if err != nil {
		jobLog.Error("build job failed", err)
		w.markError(job.ID)
		return
	}

	// 5. Insert into DB
	if err := w.jobs.InsertJob(row); err != nil {
		jobLog.Error("insert job failed", err)
		w.markError(job.ID)
		return
	}

	// 5. Write snapshot (analytics record — survives job deletion)
	techBytes, _ := json.Marshal(result.TechStack)
	if err := w.snapshot.InsertSnapshot(store.SnapshotInput{
		ID:                row.ID,
		CompanyID:         job.CompanyID,
		Country:           result.Country,
		State:             result.State,
		Remote:            result.Remote,
		Sponsorship:       result.Sponsorship,
		RequiresCitizen:   result.RequiresCitizen,
		SecurityClearance: result.SecurityClearance,
		EmployType:        job.EmployType,
		ExperienceYears:   result.ExperienceYears,
		TechStack:         techBytes,
		RoleCategory:      result.RoleCategory,
	}); err != nil {
		// Non-fatal: log and continue — job is already saved
		jobLog.Warn("write snapshot failed (non-fatal)", logger.Err(err))
	}

	jobLog.Info("enriched and saved")
}

// markError updates the seen_ledger so failed jobs aren't re-processed endlessly.
func (w *Worker) markError(jobID string) {
	if err := w.ledger.UpdateSeenReason(jobID, model.SeenReasonError); err != nil {
		w.log.Error("failed to mark error in seen_ledger", err, logger.Str(logger.FieldJobID, jobID))
	}
}

// fetchContent fetches the full content of a job posting as a structured response.
// If the adapter implements ContentFetcher, its native API is used (structured JSON).
// Otherwise falls back to a plain HTTP GET and wraps the stripped HTML in a FetchJobContentResponse.
func (w *Worker) fetchContent(ctx context.Context, job ats_fetcher.RawJob) (ats_fetcher.FetchJobContentResponse, error) {
	adp, err := ats_fetcher.Registry(job.Adapter)
	if err == nil {
		if cf, ok := adp.(ats_fetcher.ContentFetcher); ok {
			return cf.FetchJobContent(ctx, job)
		}
	}
	// Fallback: scrape the hosted HTML page and populate only the fields we can infer
	html, err := w.fetchPage(ctx, job.URL)
	if err != nil {
		return ats_fetcher.FetchJobContentResponse{}, err
	}
	return ats_fetcher.FetchJobContentResponse{
		ID:          job.ID,
		Title:       job.Title,
		Location:    job.Location,
		URL:         job.URL,
		Description: ats_fetcher.StripHTML(html),
	}, nil
}

// fetchPage does a simple HTTP GET and returns the raw response body.
func (w *Worker) fetchPage(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; JobRadar/1.0)")

	resp, err := w.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	// Cap at 50KB — LLM context windows are finite
	body, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(body), nil
}

// dropJob returns a non-empty reason string if the job should be discarded.
// It never writes to the DB — the caller decides what to do with the reason.
func (w *Worker) dropJob(e *llm.EnrichResult) string {
	if !e.Sponsorship {
		return "no sponsorship"
	}
	if e.RequiresCitizen {
		return "requires citizenship"
	}
	if e.SecurityClearance {
		return "requires security clearance"
	}
	if w.maxExpYears > 0 && e.ExperienceYears != nil && *e.ExperienceYears > w.maxExpYears {
		return fmt.Sprintf("experience required (%d yrs) exceeds max (%d yrs)", *e.ExperienceYears, w.maxExpYears)
	}
	return ""
}

// buildJob maps ats_fetcher.RawJob + llm.EnrichResult into a model.Job ready for DB insert.
func buildJob(raw ats_fetcher.RawJob, enriched *llm.EnrichResult) (*model.Job, error) {
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
		Location:          nullableString(raw.Location),
		Remote:            enriched.Remote,
		Country:           nullableString(enriched.Country),
		State:             nullableString(enriched.State),
		URL:               raw.URL,
		EmployType:        nullableString(raw.EmployType),
		ExperienceYears:   enriched.ExperienceYears,
		TechStack:         datatypes.JSON(techStack),
		SalaryRange:       datatypes.JSON(salaryRange),
		LLMSummary:        nullableString(enriched.Summary),
		Sponsorship:       enriched.Sponsorship,
		RequiresCitizen:   enriched.RequiresCitizen,
		SecurityClearance: enriched.SecurityClearance,
		RoleBrief:         nullableString(enriched.RoleBrief),
		Responsibilities:  datatypes.JSON(responsibilities),
		Status:            model.JobStatusNew,
	}, nil
}

// fullContentToResponse converts a pre-fetched FullJobContent into the
// FetchJobContentResponse shape that the rest of the enrich pipeline expects.
func fullContentToResponse(job ats_fetcher.RawJob) ats_fetcher.FetchJobContentResponse {
	fc := job.FullContent
	return ats_fetcher.FetchJobContentResponse{
		ID:             job.ID,
		Title:          job.Title,
		Location:       job.Location,
		URL:            job.URL,
		Description:    fc.Description,
		EmploymentType: job.EmployType,
		Department:     fc.Department,
		Compensation:   fc.Compensation,
		WorkplaceType:  fc.WorkplaceType,
		Requirements:   fc.Requirements,
		PublishedDate:  fc.PublishedDate,
	}
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
