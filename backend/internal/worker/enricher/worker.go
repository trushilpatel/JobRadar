package enricher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/trushilpatel/jobradar/internal/adapter"
	"github.com/trushilpatel/jobradar/internal/kafka"
	"github.com/trushilpatel/jobradar/internal/llm"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
	"gorm.io/datatypes"
)

// Worker consumes jobs.raw_detail, fetches the full job page,
// runs LLM enrichment, and writes the final row to the job table.
type Worker struct {
	consumer *kafka.Consumer
	scorer   llm.Scorer
	jobs     store.JobWriter
	ledger   store.SeenLedgerWriter
}

func NewWorker(
	consumer *kafka.Consumer,
	scorer llm.Scorer,
	jobs store.JobWriter,
	ledger store.SeenLedgerWriter,
) *Worker {
	return &Worker{
		consumer: consumer,
		scorer:   scorer,
		jobs:     jobs,
		ledger:   ledger,
	}
}

// Run starts consuming jobs.raw_detail and blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	log.Println("[enricher] worker started")

	return w.consumer.Poll(ctx, func(topic string, value []byte) error {
		var job adapter.RawJob
		if err := json.Unmarshal(value, &job); err != nil {
			log.Printf("[enricher] malformed message: %v", err)
			return nil // skip, don't stop the consumer
		}

		w.enrich(ctx, job)
		return nil
	})
}

// enrich runs the full enrichment pipeline for a single job.
func (w *Worker) enrich(ctx context.Context, job adapter.RawJob) {
	// 1. Fetch the full job page
	rawText, err := fetchPage(ctx, job.URL)
	if err != nil {
		log.Printf("[enricher] %s: fetch page failed: %v", job.ID, err)
		w.markError(job.ID)
		return
	}

	// 2. Send to LLM for structured extraction
	result, err := w.scorer.Enrich(ctx, llm.EnrichInput{
		ID:      job.ID,
		RawText: rawText,
	})
	if err != nil {
		log.Printf("[enricher] %s: LLM enrich failed: %v", job.ID, err)
		w.markError(job.ID)
		return
	}

	// 3. Build the DB row
	row, err := buildJob(job, result)
	if err != nil {
		log.Printf("[enricher] %s: build job failed: %v", job.ID, err)
		w.markError(job.ID)
		return
	}

	// 4. Insert into DB
	if err := w.jobs.InsertJob(row); err != nil {
		log.Printf("[enricher] %s: insert job failed: %v", job.ID, err)
		w.markError(job.ID)
		return
	}

	// 5. Confirm in seen_ledger
	if err := w.ledger.UpdateSeenReason(job.ID, model.SeenReasonFetch); err != nil {
		log.Printf("[enricher] %s: update seen_ledger failed: %v", job.ID, err)
	}

	log.Printf("[enricher] %s: enriched and saved — %q", job.ID, job.Title)
}

// markError updates the seen_ledger so failed jobs aren't re-processed endlessly.
func (w *Worker) markError(jobID string) {
	if err := w.ledger.UpdateSeenReason(jobID, model.SeenReasonError); err != nil {
		log.Printf("[enricher] %s: failed to mark error in seen_ledger: %v", jobID, err)
	}
}

// fetchPage does a simple HTTP GET and returns the response body as a string.
// Claude handles both raw HTML and plain text — no stripping needed.
func fetchPage(ctx context.Context, url string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	// Mimic a browser so job pages don't block us
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; JobRadar/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	// Cap at 50KB — job pages don't need more and LLM context windows are finite
	body, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	return string(body), nil
}

// buildJob maps adapter.RawJob + llm.EnrichResult into a model.Job ready for DB insert.
func buildJob(raw adapter.RawJob, enriched *llm.EnrichResult) (*model.Job, error) {
	// JSON-encode the array/map fields
	techStack, err := json.Marshal(enriched.TechStack)
	if err != nil {
		return nil, fmt.Errorf("marshal tech_stack: %w", err)
	}

	salaryRange, err := json.Marshal(map[string]any{
		"min":      enriched.SalaryMin,
		"max":      enriched.SalaryMax,
		"currency": enriched.SalaryCurrency,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal salary_range: %w", err)
	}

	otherCriteria, err := json.Marshal(enriched.OtherCriteria)
	if err != nil {
		return nil, fmt.Errorf("marshal other_criteria: %w", err)
	}

	employType := raw.EmployType // may be empty — that's fine, column is nullable

	return &model.Job{
		ID:            uuid.New().String(),
		JobPostID:     raw.ID,
		CompanyID:     extractCompanyID(raw.ID),
		Title:         raw.Title,
		Location:      nullableString(raw.Location),
		URL:           raw.URL,
		Department:    nullableString(raw.Department),
		EmployType:    nullableString(employType),
		YearsExpMin:   enriched.YearsExpMin,
		YearsExpMax:   enriched.YearsExpMax,
		TechStack:     datatypes.JSON(techStack),
		SalaryRange:   datatypes.JSON(salaryRange),
		LLMSummary:    nullableString(enriched.Summary),
		OtherCriteria: datatypes.JSON(otherCriteria),
		Status:        model.JobStatusNew,
	}, nil
}

// extractCompanyID pulls the company ID prefix from a job ID like "greenhouse-12345".
// The scan engine always uses the format "<adapter>-<id>" so we can reverse it here.
func extractCompanyID(jobID string) string {
	for i, c := range jobID {
		if c == '-' {
			return jobID[:i]
		}
	}
	return jobID
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
