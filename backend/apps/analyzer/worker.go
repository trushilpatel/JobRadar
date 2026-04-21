package main

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
	llmcore "github.com/trushilpatel/jobradar/service/llm/services"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Worker consumes jobs.raw_detail, fetches the full job page,
// runs LLM enrichment, and writes the final row to the job table.
type Worker struct {
	consumer    *kafka.Consumer
	publisher   kafka.Publisher
	scorer      llmcore.JobPostEnricher
	pipeline    jobsvc.PipelineService
	client      *http.Client
	log         logger.Logger
	maxExpYears int
	concurrency int
}

func NewWorker(
	consumer *kafka.Consumer,
	publisher kafka.Publisher,
	scorer llmcore.JobPostEnricher,
	pipeline jobsvc.PipelineService,
	log logger.Logger,
	maxExpYears int,
	concurrency int,
) *Worker {
	return &Worker{
		consumer:    consumer,
		publisher:   publisher,
		scorer:      scorer,
		pipeline:    pipeline,
		client:      &http.Client{Timeout: 15 * time.Second},
		log:         log,
		maxExpYears: maxExpYears,
		concurrency: concurrency,
	}
}

// Run starts consuming jobs.raw_detail and blocks until ctx is cancelled.
// Jobs are processed concurrently up to enrichConcurrency — page fetches
// dominate latency so parallelism here is the main throughput lever.
// Offsets are committed only after every goroutine in the batch completes,
// guaranteeing at-least-once delivery.
func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker started", logger.Int("concurrency", w.concurrency))

	sem := make(chan struct{}, w.concurrency)

	return w.consumer.PollBatch(ctx, func(records []*kgo.Record) error {
		var wg sync.WaitGroup
		for _, r := range records {
			msgCtx := tracing.ExtractKafka(ctx, r.Headers)
			var job ats.RawJob
			if err := json.Unmarshal(r.Value, &job); err != nil {
				w.log.Warn("malformed message, skipping", logger.Err(err))
				continue
			}

			retryCount := kafka.RetryCountFromHeaders(r.Headers)
			value := r.Value

			w.log.Debug("job received",
				logger.Str(logger.FieldJobID, job.ID),
				logger.Str("title", job.Title),
				logger.Int("retry_count", retryCount),
			)

			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				if err := w.enrich(msgCtx, job); err != nil {
					w.handleFailure(msgCtx, job, value, retryCount, err)
				}
			}()
		}
		wg.Wait() // all enrichments complete before returning nil → commit fires
		return nil
	})
}

// enrich runs the full enrichment pipeline for a single job.
// Returns nil on success (or on terminal states like irrelevant/drop which are
// handled inline). Returns a non-nil error for transient failures that the
// caller must route to the retry tiers or DLQ.
func (w *Worker) enrich(ctx context.Context, job ats.RawJob) error {
	ctx, span := tracing.Start(ctx, "analyzer.worker.enrich")
	defer tracing.End(span, nil)
	jobLog := w.log.With(
		logger.Str(logger.FieldJobID, job.ID),
		logger.Str("title", job.Title),
	)

	// 1. Get full job content — use pre-fetched data if available,
	// otherwise fetch via native API or HTTP fallback.
	var content ats.FetchJobContentResponse
	if job.FullContent != nil {
		content = fullContentToResponse(job)
	} else {
		var err error
		content, err = w.fetchContent(ctx, job)
		if err != nil {
			jobLog.Error("fetch content failed", err)
			metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
			return fmt.Errorf("fetch content: %w", err)
		}
	}

	// Serialize the structured content to JSON for the LLM.
	// JSON gives the LLM all available fields (description, compensation, location, etc.)
	// in a compact, consistent format regardless of which platform fetched the job.
	rawText, err := json.Marshal(content)
	if err != nil {
		jobLog.Error("marshal content failed", err)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("marshal content: %w", err)
	}

	// 2. Send to LLM for structured extraction
	llmStart := time.Now()
	result, err := w.scorer.Enrich(ctx, llmdto.JobPostEnrichInput{
		ID:      job.ID,
		RawText: string(rawText),
	})
	metrics.EnricherLLMDuration.Observe(time.Since(llmStart).Seconds())
	if err != nil {
		jobLog.Error("LLM enrich failed", err)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("llm enrich: %w", err)
	}

	// 3. Drop ineligible jobs before writing to DB
	if !result.RoleMatch {
		jobLog.Debug("job dropped: role mismatch", logger.Str("title", job.Title))
		w.markIrrelevant(ctx, job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("dropped").Inc()
		return nil
	}
	if reason := w.dropJob(result); reason != "" {
		jobLog.Debug("job dropped", logger.Str("reason", reason))
		// dropJob is a filter (not an error); keep pre-existing behavior of
		// marking ledger error so the job isn't re-processed.
		w.markError(ctx, job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("dropped").Inc()
		return nil
	}

	// 4. Save via pipeline service (builds job row, inserts, writes snapshot, marks ledger)
	input := toEnrichedInput(job, result)
	if err := w.pipeline.SaveEnrichedJob(ctx, input); err != nil {
		jobLog.Error("save enriched job failed", err)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("save enriched job: %w", err)
	}

	metrics.EnricherJobsTotal.WithLabelValues("saved").Inc()
	jobLog.Debug("enriched and saved")
	return nil
}

// handleFailure routes a failed enrichment to the appropriate Kafka topic.
// Validation errors skip retries entirely (they're our bug — retry won't help).
// Transient errors move through the retry ladder; exhausted retries go to the DLQ.
// Ledger is only marked `error` for DLQ-bound messages.
func (w *Worker) handleFailure(ctx context.Context, job ats.RawJob, value []byte, retryCount int, err error) {
	// Context cancellation during shutdown: do NOT publish to retry/DLQ.
	// Let Kafka redeliver on restart.
	if stderrors.Is(err, context.Canceled) || ctx.Err() != nil {
		return
	}

	jobLog := w.log.With(
		logger.Str(logger.FieldJobID, job.ID),
		logger.Int("retry_count", retryCount),
	)

	// Validation errors go straight to DLQ — retrying is useless, it's our own bug.
	if stderrors.Is(err, domainerr.ErrLLMValidation) {
		w.sendToDLQ(ctx, jobLog, job.ID, value, retryCount, err)
		return
	}

	tiers := kafka.AnalyzerRetryTiers()
	if retryCount >= len(tiers) {
		w.sendToDLQ(ctx, jobLog, job.ID, value, retryCount, err)
		return
	}

	tier := tiers[retryCount]
	nextAttemptAt := time.Now().Add(tier.Delay)
	headers := kafka.BuildRetryHeaders(retryCount+1, nextAttemptAt)
	if perr := w.publisher.PublishWithHeaders(ctx, tier.Topic, value, headers); perr != nil {
		jobLog.Error("publish retry failed", perr, logger.Str("topic", tier.Topic))
		return
	}
	jobLog.Info("scheduled retry",
		logger.Str("topic", tier.Topic),
		logger.Dur("delay", tier.Delay),
	)
}

// sendToDLQ publishes the original message to the DLQ topic and marks the
// ledger row as errored. Used for both validation errors and exhausted retries.
func (w *Worker) sendToDLQ(ctx context.Context, jobLog logger.Logger, jobID string, value []byte, retryCount int, err error) {
	headers := kafka.BuildDLQHeaders(retryCount, err.Error())
	if perr := w.publisher.PublishWithHeaders(ctx, kafka.TopicJobListingsFilteredDLQ, value, headers); perr != nil {
		jobLog.Error("publish DLQ failed", perr)
		return
	}
	w.markError(ctx, jobID)
	jobLog.Warn("moved to DLQ", logger.Err(err))
	metrics.EnricherJobsTotal.WithLabelValues("dlq").Inc()
}

// toEnrichedInput builds a jobdto.EnrichedJobInput from the raw job and LLM result.
func toEnrichedInput(job ats.RawJob, result *llmdto.JobPostEnrichResult) jobdto.EnrichedJobInput {
	return jobdto.EnrichedJobInput{
		Raw:      job,
		Enriched: result,
	}
}

// markError updates the seen_ledger so failed jobs aren't re-processed endlessly.
func (w *Worker) markError(ctx context.Context, jobID string) {
	if err := w.pipeline.MarkLedgerError(ctx, jobID); err != nil {
		w.log.Error("failed to mark error in seen_ledger", err, logger.Str(logger.FieldJobID, jobID))
	}
}

// markIrrelevant marks a job as llm_irrelevant so it is skipped cleanly without counting as an error.
func (w *Worker) markIrrelevant(ctx context.Context, jobID string) {
	if err := w.pipeline.MarkLedgerIrrelevant(ctx, jobID); err != nil {
		w.log.Error("failed to mark irrelevant in seen_ledger", err, logger.Str(logger.FieldJobID, jobID))
	}
}

// fetchContent fetches the full content of a job posting as a structured response.
// If the adapter implements ContentFetcher, its native API is used (structured JSON).
// Otherwise falls back to a plain HTTP GET and wraps the stripped HTML in a FetchJobContentResponse.
func (w *Worker) fetchContent(ctx context.Context, job ats.RawJob) (ats.FetchJobContentResponse, error) {
	adp, err := ats.Registry(job.Adapter)
	if err == nil {
		if cf, ok := adp.(ats.ContentFetcher); ok {
			return cf.FetchJobContent(ctx, job)
		}
	}
	// Fallback: scrape the hosted HTML page and populate only the fields we can infer
	html, err := w.fetchPage(ctx, job.URL)
	if err != nil {
		return ats.FetchJobContentResponse{}, err
	}
	return ats.FetchJobContentResponse{
		ID:          job.ID,
		Title:       job.Title,
		Location:    job.Location,
		URL:         job.URL,
		Description: ats.StripHTML(html),
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
func (w *Worker) dropJob(e *llmdto.JobPostEnrichResult) string {
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

// fullContentToResponse converts a pre-fetched FullJobContent into the
// FetchJobContentResponse shape that the rest of the enrich pipeline expects.
func fullContentToResponse(job ats.RawJob) ats.FetchJobContentResponse {
	fc := job.FullContent
	return ats.FetchJobContentResponse{
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
