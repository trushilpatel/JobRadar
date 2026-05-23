package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/job/location"
)

// fetchAll runs HTTP fetches for every message in parallel, capped by fetchConc
// and additionally rate-limited per adapter. Messages that fail to parse, fetch,
// or whose adapter is unknown are routed to Nak/Term here (stage="fetch") — they
// never reach stage 2.
//
// The returned slice contains only successfully-fetched jobs in arbitrary order.
func (w *Worker) fetchAll(ctx context.Context, fetchLog logger.Logger, msgs []*js.Msg) []fetchedJob {
	ctx, span := tracing.Start(ctx, "analyzer.worker.fetchAll")
	defer span.End()
	span.SetAttributes(attribute.Int("msgs", len(msgs)))

	sem := make(chan struct{}, w.fetchConc)
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		fetched = make([]fetchedJob, 0, len(msgs))
		failed  atomic.Int64
	)

	for _, msg := range msgs {
		wg.Add(1)
		sem <- struct{}{}
		go func(m *js.Msg) {
			defer wg.Done()
			defer func() { <-sem }()

			result, ok := w.fetchOne(ctx, fetchLog, m)
			if !ok {
				failed.Add(1)
				return
			}
			mu.Lock()
			fetched = append(fetched, result)
			mu.Unlock()
		}(msg)
	}

	wg.Wait()
	span.SetAttributes(
		attribute.Int("fetched", len(fetched)),
		attribute.Int("failed", int(failed.Load())),
	)
	return fetched
}

// fetchOne handles all error routing for a single message so the caller can
// just collect the success cases. Returns ok=false when the message was dropped
// (malformed → Term, fetch failed → Nak/Term via handleFailure).
func (w *Worker) fetchOne(ctx context.Context, fetchLog logger.Logger, msg *js.Msg) (fetchedJob, bool) {
	msgCtx, span := tracing.Start(ctx, "analyzer.worker.fetchOne")
	defer span.End()

	var job ats.RawJob
	if err := json.Unmarshal(msg.Data(), &job); err != nil {
		span.SetAttributes(attribute.String("outcome", "malformed"))
		fetchLog.Warn("malformed message, terminating", logger.Err(err))
		if terr := msg.Term(); terr != nil {
			fetchLog.Warn("msg term failed", logger.Err(terr))
		}
		return fetchedJob{}, false
	}

	span.SetAttributes(
		attribute.String("job_id", job.ID),
		attribute.String("adapter", job.Adapter),
	)

	retryCount := msg.DeliveryCount() - 1
	jobLog := fetchLog.With(
		logger.Str(logger.FieldJobID, job.ID),
		logger.Str(logger.FieldAdapter, job.Adapter),
		logger.Int(logger.FieldRetryCount, retryCount),
	)
	jobLog.Debug("fetching content")

	// Path A early gate: when LocationKnown=true the adapter already supplied
	// structured location data. Reject non-USA jobs here before any HTTP call.
	if job.LocationKnown && location.AnyNonUSA(analyzerMergedLocations(job)) {
		span.SetAttributes(attribute.String("outcome", "non_usa_early"))
		jobLog.Info("early reject: non-USA job (LocationKnown=true)",
			logger.Str(logger.FieldLocation, job.Location),
			logger.Bool(logger.FieldLocationKnown, true),
		)
		metrics.EnricherJobsTotal.WithLabelValues("non_usa").Inc()
		w.markNonUSA(msgCtx, job.ID)
		if aerr := msg.Ack(); aerr != nil {
			jobLog.Warn("msg ack failed", logger.Err(aerr))
		}
		return fetchedJob{}, false
	}

	fetchStart := time.Now()
	content, err := w.fetchContent(msgCtx, job)
	span.SetAttributes(attribute.Int64("wait_ms", time.Since(fetchStart).Milliseconds()))
	if err != nil {
		span.SetAttributes(attribute.String("outcome", "fetch_failed"))
		jobLog.Error("fetch content failed", err)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		fj := fetchedJob{job: job, msg: msg, retryCount: retryCount, ctx: msgCtx}
		w.handleFailure(msgCtx, fetchLog, fj, "fetch", fmt.Errorf("fetch content: %w", err))
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

	// Path B post-backfill gate: reject unambiguously non-USA jobs so they
	// never reach the LLM enricher. CountryCode (ISO alpha-2) from the detail
	// response is checked first — it is the most reliable signal. Falls back
	// to string-based AnyNonUSA on the merged location slice.
	// ACK the message (terminal — no retry).
	var nonUSA bool
	var rejectedBy string
	switch {
	case isUSCode(content.CountryCode):
		// Confirmed USA via country code — keep.
	case content.CountryCode != "" && !isUSCode(content.CountryCode):
		// Non-empty, non-US country code → definite non-USA.
		nonUSA = true
		rejectedBy = "country_code"
	default:
		// No country code — fall back to string matching.
		if location.AnyNonUSA(analyzerMergedLocations(job)) {
			nonUSA = true
			rejectedBy = "string_match"
		}
	}
	if nonUSA {
		span.SetAttributes(attribute.String("outcome", "non_usa"))
		jobLog.Info("rejecting non-USA job after location backfill",
			logger.Str(logger.FieldJobID, job.ID),
			logger.Str(logger.FieldLocation, job.Location),
			logger.Str(logger.FieldCountryCode, content.CountryCode),
			logger.Str(logger.FieldRejectedBy, rejectedBy),
		)
		metrics.EnricherJobsTotal.WithLabelValues("non_usa").Inc()
		w.markNonUSA(msgCtx, job.ID)
		if aerr := msg.Ack(); aerr != nil {
			jobLog.Warn("msg ack failed", logger.Err(aerr))
		}
		return fetchedJob{}, false
	}

	span.SetAttributes(attribute.String("outcome", "ok"))
	jobLog.Debug("content fetched", logger.Int(logger.FieldDescriptionBytes, len(content.Description)))
	return fetchedJob{
		job:        job,
		content:    content,
		msg:        msg,
		retryCount: retryCount,
		ctx:        msgCtx,
	}, true
}

// fetchContent returns the full content of a job posting. If the message
// already carries FullContent (full-detail adapters like Google or Uber), no
// HTTP call is made. Otherwise it tries the adapter's native ContentFetcher
// first and falls back to a plain HTTP GET on the hosted URL.
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

	adp, err := w.registryFn(job.Adapter)
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

// isUSCode reports whether code is the ISO alpha-2 code for the United States.
// Empty string returns false — absence of a code is not a USA confirmation.
func isUSCode(code string) bool {
	return strings.EqualFold(strings.TrimSpace(code), "US")
}

// analyzerMergedLocations returns AllLocations if set, otherwise wraps the
// single Location string in a slice. Mirrors the same helper in the filter
// worker so both stages apply AnyNonUSA consistently.
func analyzerMergedLocations(job ats.RawJob) []string {
	if len(job.AllLocations) > 0 {
		return job.AllLocations
	}
	if job.Location != "" {
		return []string{job.Location}
	}
	return nil
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
