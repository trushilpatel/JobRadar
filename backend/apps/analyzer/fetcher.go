package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/trushilpatel/jobradar/common/ats/core"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/job/location"
)

// fetchAll runs detail fetches for every message in parallel, capped by fetchConc
// and additionally rate-limited per adapter. Messages that fail to parse, fetch,
// or whose adapter has no post-fetcher are routed to Nak/Term here (stage="fetch")
// — they never reach stage 2.
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
// (malformed → Term, fetch failed / no post-fetcher → Nak/Term via handleFailure).
func (w *Worker) fetchOne(ctx context.Context, fetchLog logger.Logger, msg *js.Msg) (fetchedJob, bool) {
	msgCtx, span := tracing.Start(ctx, "analyzer.worker.fetchOne")
	defer span.End()

	var summary core.JobSummary
	if err := json.Unmarshal(msg.Data(), &summary); err != nil {
		span.SetAttributes(attribute.String("outcome", "malformed"))
		fetchLog.Warn("malformed message, terminating", logger.Err(err))
		if terr := msg.Term(); terr != nil {
			fetchLog.Warn("msg term failed", logger.Err(terr))
		}
		return fetchedJob{}, false
	}

	span.SetAttributes(
		attribute.String("job_id", summary.ID),
		attribute.String("adapter", summary.Adapter),
	)

	retryCount := msg.DeliveryCount() - 1
	jobLog := fetchLog.With(
		logger.Str(logger.FieldJobID, summary.ID),
		logger.Str(logger.FieldAdapter, summary.Adapter),
		logger.Int(logger.FieldRetryCount, retryCount),
	)
	jobLog.Debug("fetching job post")

	// Path A early gate: when LocationKnown=true the adapter already supplied
	// structured location data. Reject non-USA jobs here before any HTTP call.
	if summary.LocationKnown && location.AnyNonUSA(analyzerMergedLocations(summary)) {
		span.SetAttributes(attribute.String("outcome", "non_usa_early"))
		jobLog.Info("early reject: non-USA job (LocationKnown=true)",
			logger.Str(logger.FieldLocation, summary.Location),
			logger.Bool(logger.FieldLocationKnown, true),
		)
		metrics.EnricherJobsTotal.WithLabelValues("non_usa").Inc()
		w.markNonUSA(msgCtx, summary.ID)
		if aerr := msg.Ack(); aerr != nil {
			jobLog.Warn("msg ack failed", logger.Err(aerr))
		}
		return fetchedJob{}, false
	}

	fetchStart := time.Now()
	post, err := w.resolveJobPost(msgCtx, summary)
	span.SetAttributes(attribute.Int64("wait_ms", time.Since(fetchStart).Milliseconds()))
	if err != nil {
		span.SetAttributes(attribute.String("outcome", "fetch_failed"))
		jobLog.Error("fetch job post failed", err)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		fj := fetchedJob{summary: summary, msg: msg, retryCount: retryCount, ctx: msgCtx}
		w.handleFailure(msgCtx, fetchLog, fj, "fetch", fmt.Errorf("fetch job post: %w", err))
		return fetchedJob{}, false
	}

	// Path B post-fetch gate: reject unambiguously non-USA jobs so they never
	// reach the LLM enricher. CountryCode (ISO alpha-2) from the fetched post is
	// checked first — it is the most reliable signal. Falls back to string-based
	// AnyNonUSA on the merged location slice. ACK the message (terminal — no retry).
	var nonUSA bool
	var rejectedBy string
	switch {
	case isUSCode(post.CountryCode):
		// Confirmed USA via country code — keep.
	case post.CountryCode != "" && !isUSCode(post.CountryCode):
		// Non-empty, non-US country code → definite non-USA.
		nonUSA = true
		rejectedBy = "country_code"
	default:
		// No country code — fall back to string matching against the location
		// authority (Post.Location preferred, else the summary locations).
		if location.AnyNonUSA(postMergedLocations(summary, post)) {
			nonUSA = true
			rejectedBy = "string_match"
		}
	}
	if nonUSA {
		span.SetAttributes(attribute.String("outcome", "non_usa"))
		jobLog.Info("rejecting non-USA job after post fetch",
			logger.Str(logger.FieldJobID, summary.ID),
			logger.Str(logger.FieldLocation, post.Location),
			logger.Str(logger.FieldCountryCode, post.CountryCode),
			logger.Str(logger.FieldRejectedBy, rejectedBy),
		)
		metrics.EnricherJobsTotal.WithLabelValues("non_usa").Inc()
		w.markNonUSA(msgCtx, summary.ID)
		if aerr := msg.Ack(); aerr != nil {
			jobLog.Warn("msg ack failed", logger.Err(aerr))
		}
		return fetchedJob{}, false
	}

	span.SetAttributes(attribute.String("outcome", "ok"))
	jobLog.Debug("job post fetched", logger.Int(logger.FieldDescriptionBytes, len(post.Description)))
	return fetchedJob{
		summary:    summary,
		post:       post,
		msg:        msg,
		retryCount: retryCount,
		ctx:        msgCtx,
	}, true
}

// resolveJobPost returns the full JobPost for a summary using the crawler-v2
// two-phase model:
//
//   - FAT adapters (gem, uber, deel): the summary already carries inline
//     JobContent — build the JobPost from it; NO FetchJobPost call.
//   - THIN adapters: resolve the adapter bundle and call FetchJobPost. If the
//     bundle has no PostFetcher (Post == nil) or the adapter is unknown, this is
//     a per-job failure — there is no generic content fetch anymore.
//
// Per-adapter rate limiting is applied before the outbound FetchJobPost call so
// concurrent fetches for the same ATS don't trip rate limits.
func (w *Worker) resolveJobPost(ctx context.Context, summary core.JobSummary) (core.JobPost, error) {
	if summary.JobContent != nil {
		return jobContentToPost(summary), nil
	}

	bundle, err := w.bundleFn(summary.Adapter, w.log, w.client)
	if err != nil {
		return core.JobPost{}, fmt.Errorf("resolve adapter %q: %w", summary.Adapter, err)
	}
	if bundle.Post == nil {
		return core.JobPost{}, fmt.Errorf("adapter %q has no post fetcher and summary carries no inline content", summary.Adapter)
	}

	if w.rateLimiter != nil && summary.Adapter != "" {
		if rerr := w.rateLimiter.Wait(ctx, summary.Adapter); rerr != nil {
			return core.JobPost{}, fmt.Errorf("rate limit wait: %w", rerr)
		}
	}

	post, err := bundle.Post.FetchJobPost(ctx, summary)
	if err != nil {
		return core.JobPost{}, fmt.Errorf("fetch job post: %w", err)
	}
	return post, nil
}

// isUSCode reports whether code is the ISO alpha-2 code for the United States.
// Empty string returns false — absence of a code is not a USA confirmation.
func isUSCode(code string) bool {
	return strings.EqualFold(strings.TrimSpace(code), "US")
}

// analyzerMergedLocations returns AllLocations if set, otherwise wraps the
// single Location string in a slice. Mirrors the same helper in the filter
// worker so both stages apply AnyNonUSA consistently.
func analyzerMergedLocations(summary core.JobSummary) []string {
	if len(summary.AllLocations) > 0 {
		return summary.AllLocations
	}
	if summary.Location != "" {
		return []string{summary.Location}
	}
	return nil
}

// postMergedLocations merges the location signal for the post-fetch non-USA gate.
// The fetched Post.Location is the most specific signal (thin adapters backfill
// it at detail time); when empty it falls back to the summary's locations.
func postMergedLocations(summary core.JobSummary, post core.JobPost) []string {
	if loc := strings.TrimSpace(post.Location); loc != "" {
		return []string{loc}
	}
	return analyzerMergedLocations(summary)
}

// jobContentToPost converts a fat adapter's inline JobContent into a core.JobPost
// for the enrich stage. Title/Location/URL/EmploymentType come from the summary;
// description and structured detail come from the inline JobContent.
func jobContentToPost(summary core.JobSummary) core.JobPost {
	jc := summary.JobContent
	return core.JobPost{
		ID:             summary.ID,
		Title:          summary.Title,
		Location:       summary.Location,
		URL:            summary.URL,
		Description:    jc.Description,
		EmploymentType: summary.EmployType,
		Department:     jc.Department,
		Compensation:   jc.Compensation,
		WorkplaceType:  jc.WorkplaceType,
		Requirements:   jc.Requirements,
		PublishedDate:  jc.PublishedDate,
	}
}
