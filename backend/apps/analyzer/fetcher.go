package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
)

// fetchAll runs HTTP fetches for every record in parallel, capped by
// fetchConc and additionally rate-limited per adapter. Records that fail
// to parse, fetch, or whose adapter is unknown are routed to the retry
// ladder / DLQ here (stage="fetch") — they never reach stage 2.
//
// The returned slice contains only successfully-fetched jobs in arbitrary
// order; processFetch sorts by (partition, offset) before grouping.
func (w *Worker) fetchAll(ctx context.Context, fetchLog logger.Logger, records []*kgo.Record) []fetchedJob {
	ctx, span := tracing.Start(ctx, "analyzer.worker.fetchAll")
	defer span.End()
	span.SetAttributes(attribute.Int("records", len(records)))

	sem := make(chan struct{}, w.fetchConc)
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		fetched = make([]fetchedJob, 0, len(records))
		failed  atomic.Int64
	)

	for _, r := range records {
		wg.Add(1)
		sem <- struct{}{}
		go func(rec *kgo.Record) {
			defer wg.Done()
			defer func() { <-sem }()

			result, ok := w.fetchOne(ctx, fetchLog, rec)
			if !ok {
				failed.Add(1)
				return
			}
			mu.Lock()
			fetched = append(fetched, result)
			mu.Unlock()
		}(r)
	}

	wg.Wait()
	span.SetAttributes(
		attribute.Int("fetched", len(fetched)),
		attribute.Int("failed", int(failed.Load())),
	)
	return fetched
}

// fetchOne handles all error routing for a single record so the caller can
// just collect the success cases. Returns ok=false when the record was
// dropped (parse error, malformed, fetch failed → already routed via
// handleFailure with stage="fetch").
func (w *Worker) fetchOne(ctx context.Context, fetchLog logger.Logger, r *kgo.Record) (fetchedJob, bool) {
	msgCtx := tracing.ExtractKafka(ctx, r.Headers)

	msgCtx, span := tracing.Start(msgCtx, "analyzer.worker.fetchOne")
	defer span.End()

	var job ats.RawJob
	if err := json.Unmarshal(r.Value, &job); err != nil {
		span.SetAttributes(attribute.String("outcome", "malformed"))
		fetchLog.Warn("malformed message, routing to DLQ", logger.Err(err))
		headers := []kgo.RecordHeader{
			{Key: kafka.HeaderReason, Value: []byte("malformed")},
			{Key: kafka.HeaderLastError, Value: []byte(err.Error())},
		}
		if perr := w.publisher.PublishWithHeaders(ctx, kafka.TopicJobListingsFilteredDLQ, r.Value, headers); perr != nil {
			fetchLog.Error("publish malformed message to DLQ failed", perr)
		}
		if cerr := w.committer.CommitRecords(ctx, r); cerr != nil {
			fetchLog.Error("commit malformed record failed", cerr)
		}
		return fetchedJob{}, false
	}

	span.SetAttributes(
		attribute.String("job_id", job.ID),
		attribute.String("adapter", job.Adapter),
	)

	retryCount := kafka.RetryCountFromHeaders(r.Headers)
	jobLog := fetchLog.With(
		logger.Str(logger.FieldJobID, job.ID),
		logger.Str("adapter", job.Adapter),
		logger.Int(logger.FieldRetryCount, retryCount),
	)
	jobLog.Debug("fetching content")

	fetchStart := time.Now()
	content, err := w.fetchContent(msgCtx, job)
	span.SetAttributes(attribute.Int64("wait_ms", time.Since(fetchStart).Milliseconds()))
	if err != nil {
		span.SetAttributes(attribute.String("outcome", "fetch_failed"))
		jobLog.Error("fetch content failed", err)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		w.handleFailure(msgCtx, fetchLog, job, r.Value, retryCount, "fetch", fmt.Errorf("fetch content: %w", err))
		return fetchedJob{}, false
	}

	// Thin adapters (e.g. Meta) publish RawJobs with empty Title/Location at
	// listing time — the real values are only available after fetching the
	// detail page. Backfill so buildJobRow writes the correct title to `jobs`.
	if job.Title == "" && content.Title != "" {
		job.Title = content.Title
	}
	if job.Location == "" && content.Location != "" {
		job.Location = content.Location
	}

	span.SetAttributes(attribute.String("outcome", "ok"))
	jobLog.Debug("content fetched", logger.Int("description_bytes", len(content.Description)))
	return fetchedJob{
		job:        job,
		content:    content,
		record:     r,
		retryCount: retryCount,
		ctx:        msgCtx,
	}, true
}

// fetchContent returns the full content of a job posting. If the Kafka
// message already carries FullContent (full-detail adapters like Google or
// Uber), no HTTP call is made. Otherwise it tries the adapter's native
// ContentFetcher first and falls back to a plain HTTP GET on the hosted URL.
//
// Per-adapter rate limiting is applied before any outbound network call so
// concurrent fetches for the same ATS don't trip rate limits.
func (w *Worker) fetchContent(ctx context.Context, job ats.RawJob) (ats.FetchJobContentResponse, error) {
	if job.FullContent != nil {
		return fullContentToResponse(job), nil
	}

	if w.rateLimiter != nil && job.Adapter != "" {
		if err := w.rateLimiter.Wait(ctx, job.Adapter); err != nil {
			return ats.FetchJobContentResponse{}, fmt.Errorf("rate limit wait: %w", err)
		}
	}

	adp, err := ats.Registry(job.Adapter)
	if err == nil {
		if cf, ok := adp.(ats.ContentFetcher); ok {
			return cf.FetchJobContent(ctx, job)
		}
	}

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

// fetchPage GETs the hosted URL and returns the raw response body (capped
// at 50KB so a runaway page can't blow the LLM context).
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(body), nil
}

// fullContentToResponse converts a pre-fetched FullJobContent into the
// FetchJobContentResponse shape the enrich stage expects.
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
