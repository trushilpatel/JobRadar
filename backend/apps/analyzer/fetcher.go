package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
)

// fetchAll runs HTTP fetches for every record in parallel, capped by
// fetchConc and additionally rate-limited per adapter. Records that fail
// to parse, fetch, or whose adapter is unknown are routed to the retry
// ladder / DLQ here — they never reach stage 2.
//
// The returned slice contains only successfully-fetched jobs in arbitrary
// order. Callers must not assume input/output order matches.
func (w *Worker) fetchAll(ctx context.Context, records []*kgo.Record) []fetchedJob {
	sem := make(chan struct{}, w.fetchConc)
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		fetched = make([]fetchedJob, 0, len(records))
	)

	for _, r := range records {
		wg.Add(1)
		sem <- struct{}{}
		go func(rec *kgo.Record) {
			defer wg.Done()
			defer func() { <-sem }()

			result, ok := w.fetchOne(ctx, rec)
			if !ok {
				return
			}
			mu.Lock()
			fetched = append(fetched, result)
			mu.Unlock()
		}(r)
	}

	wg.Wait()
	return fetched
}

// fetchOne handles all error routing for a single record so the caller can
// just collect the success cases. Returns ok=false when the record was
// dropped (parse error, malformed, fetch failed → already routed).
func (w *Worker) fetchOne(ctx context.Context, r *kgo.Record) (fetchedJob, bool) {
	msgCtx := tracing.ExtractKafka(ctx, r.Headers)
	var job ats.RawJob
	if err := json.Unmarshal(r.Value, &job); err != nil {
		w.log.Warn("malformed message, skipping", logger.Err(err))
		return fetchedJob{}, false
	}
	retryCount := kafka.RetryCountFromHeaders(r.Headers)
	jobLog := w.log.With(
		logger.Str(logger.FieldJobID, job.ID),
		logger.Str("adapter", job.Adapter),
		logger.Int("retry_count", retryCount),
	)
	jobLog.Debug("fetching content")

	content, err := w.fetchContent(msgCtx, job)
	if err != nil {
		jobLog.Error("fetch content failed", err)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		w.handleFailure(msgCtx, job, r.Value, retryCount, fmt.Errorf("fetch content: %w", err))
		return fetchedJob{}, false
	}

	jobLog.Debug("content fetched", logger.Int("description_bytes", len(content.Description)))
	return fetchedJob{
		job:        job,
		content:    content,
		value:      r.Value,
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
