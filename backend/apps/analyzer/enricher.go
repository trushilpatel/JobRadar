package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/trushilpatel/jobradar/common/ats/core"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
)

// processGroup runs one EnrichBatch call for a group of fetched jobs and
// processes the per-job results. Returns routed=true if every job in the group
// reached a terminal routing (saved, dropped, retry-Nak'd, DLQ-Term'd).
// Returns routed=false if any job was left un-routed (ctx cancel mid-route).
//
// Failure modes:
//
//   - Whole batch fails  → every job in the group is routed via handleFailure
//     using its own retryCount, stage="enrich". NATS redelivers via BackOff.
//   - Result missing for a job → routed to Nak (within budget) or DLQ (exhausted).
//   - Extra unknown job_ids in the response → ignored with a warning log.
//   - Per-job save error → Ack'd with ledger marked; other jobs in the batch unaffected.
func (w *Worker) processGroup(ctx context.Context, fetchLog logger.Logger, group []fetchedJob, fetchID string, groupIdx, groupCount int) (routed bool) {
	if len(group) == 0 {
		return true
	}

	retryMax := 0
	for _, fj := range group {
		if fj.retryCount > retryMax {
			retryMax = fj.retryCount
		}
	}

	var spanErr error
	groupCtx, span := tracing.Start(ctx, "analyzer.worker.processGroup")
	span.SetAttributes(
		attribute.Int("group.index", groupIdx),
		attribute.Int("group.size", len(group)),
		attribute.Int("group.retry_count_max", retryMax),
	)
	defer tracing.End(span, &spanErr)

	groupLog := fetchLog.With(
		logger.Int(logger.FieldChunkIndex, groupIdx),
		logger.Int(logger.FieldChunkCount, groupCount),
	)

	start := w.clock.Now()
	stats := &groupOutcomeStats{}
	defer func() {
		metrics.EnricherGroupDuration.
			WithLabelValues(stats.label()).
			Observe(w.clock.Now().Sub(start).Seconds())
	}()

	jobIDs := make([]string, len(group))
	for i, fj := range group {
		jobIDs[i] = fj.summary.ID
	}
	groupLog.Info("LLM batch sent",
		logger.Int(logger.FieldBatch, len(group)),
		logger.Str(logger.FieldJobIDs, strings.Join(jobIDs, ",")),
	)

	inputs, err := w.buildEnrichInputs(group)
	if err != nil {
		spanErr = err
		groupLog.Error("build enrich inputs failed", err)
		stats.markRouteFailure()
		return w.routeBatchFailure(groupCtx, groupLog, group, err)
	}

	enrichCtx, enrichSpan := tracing.Start(groupCtx, "analyzer.worker.enrichBatch")
	enrichSpan.SetAttributes(attribute.Int("batch.size", len(group)))

	llmStart := time.Now()
	results, err := w.scorer.EnrichBatch(enrichCtx, inputs)
	metrics.EnricherLLMDuration.Observe(time.Since(llmStart).Seconds())

	if err != nil {
		spanErr = err
		enrichSpan.SetAttributes(attribute.String("outcome", "error"))
		enrichSpan.RecordError(err)
		enrichSpan.End()
		groupLog.Error("EnrichBatch failed, routing whole batch", err,
			logger.Int(logger.FieldBatch, len(group)),
		)
		stats.markRouteFailure()
		return w.routeBatchFailure(groupCtx, groupLog, group, err)
	}
	enrichSpan.SetAttributes(attribute.String("outcome", "ok"))
	enrichSpan.End()

	groupLog.Info("LLM batch returned",
		logger.Int(logger.FieldBatch, len(group)),
		logger.Int(logger.FieldResultCount, len(results)),
		logger.Dur(logger.FieldDuration, time.Since(llmStart)),
	)

	return w.distributeResults(groupCtx, groupLog, group, results, stats)
}

// buildEnrichInputs marshals each fetched job's content into the input shape
// the LLM expects.
func (w *Worker) buildEnrichInputs(group []fetchedJob) ([]llmdto.JobPostEnrichInput, error) {
	inputs := make([]llmdto.JobPostEnrichInput, 0, len(group))
	for _, fj := range group {
		raw, err := marshalContent(fj.post)
		if err != nil {
			return nil, fmt.Errorf("marshal content for %s: %w", fj.summary.ID, err)
		}
		inputs = append(inputs, llmdto.JobPostEnrichInput{
			ID:      fj.summary.ID,
			RawText: raw,
		})
	}
	return inputs, nil
}

// distributeResults walks the LLM response and routes each job to save, drop,
// or retry. Returns false if any job was left un-routed (ctx cancel mid-route).
func (w *Worker) distributeResults(ctx context.Context, groupLog logger.Logger, group []fetchedJob, results []*llmdto.JobPostEnrichResult, stats *groupOutcomeStats) bool {
	resultByID := make(map[string]*llmdto.JobPostEnrichResult, len(results))
	for _, r := range results {
		if r == nil {
			continue
		}
		if _, dup := resultByID[r.JobID]; dup {
			groupLog.Warn("LLM returned duplicate job_id, keeping first",
				logger.Str(logger.FieldJobID, r.JobID),
			)
			continue
		}
		resultByID[r.JobID] = r
	}

	groupIDs := make(map[string]struct{}, len(group))
	for _, fj := range group {
		groupIDs[fj.summary.ID] = struct{}{}
	}
	for id := range resultByID {
		if _, ok := groupIDs[id]; !ok {
			groupLog.Warn("LLM returned unknown job_id, ignoring",
				logger.Str(logger.FieldJobID, id),
			)
		}
	}

	allRouted := true
	for _, fj := range group {
		result, ok := resultByID[fj.summary.ID]
		if !ok {
			groupLog.Error("LLM result missing for job, routing to retry",
				fmt.Errorf("missing job_id %s in EnrichBatch response", fj.summary.ID),
				logger.Str(logger.FieldJobID, fj.summary.ID),
			)
			metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
			stats.markRouteFailure()
			missingErr := fmt.Errorf("llm_result_missing: %s", fj.summary.ID)
			if !w.routeMissingResult(fj.ctx, groupLog, fj, missingErr) {
				allRouted = false
			}
			continue
		}
		w.processResult(fj.ctx, fj, result, stats)
	}
	return allRouted
}

// routeMissingResult routes a missing-result job to Nak (within budget) or
// DLQ (exhausted, cause="llm_result_missing"). stage="enrich".
func (w *Worker) routeMissingResult(ctx context.Context, fetchLog logger.Logger, fj fetchedJob, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	if fj.retryCount >= analyzerMaxRetries {
		jobLog := fetchLog.With(
			logger.Str(logger.FieldJobID, fj.summary.ID),
			logger.Int(logger.FieldRetryCount, fj.retryCount),
		)
		return w.sendToDLQ(ctx, jobLog, fj, "llm_result_missing", err)
	}
	return w.handleFailure(ctx, fetchLog, fj, stageEnrich, err)
}

// processResult applies the drop rules and either saves the job or marks the
// ledger row appropriately. All terminal paths call msg.Ack() to acknowledge
// the NATS message.
func (w *Worker) processResult(ctx context.Context, fj fetchedJob, result *llmdto.JobPostEnrichResult, stats *groupOutcomeStats) {
	jobLog := w.log.With(
		logger.Str(logger.FieldJobID, fj.summary.ID),
		logger.Str(logger.FieldTitle, jobTitle(fj)),
	)

	if !result.RoleMatch {
		jobLog.Debug("job dropped: role mismatch")
		w.markIrrelevant(ctx, fj.summary.ID)
		if aerr := fj.msg.Ack(); aerr != nil {
			jobLog.Warn("msg ack failed", logger.Err(aerr))
		}
		metrics.EnricherJobsTotal.WithLabelValues("dropped").Inc()
		stats.markDropped()
		return
	}

	if reason := w.dropJob(result); reason != "" {
		jobLog.Info("job dropped", logger.Str(logger.FieldDropReason, reason))
		w.markIrrelevant(ctx, fj.summary.ID)
		if aerr := fj.msg.Ack(); aerr != nil {
			jobLog.Warn("msg ack failed", logger.Err(aerr))
		}
		metrics.EnricherJobsTotal.WithLabelValues("dropped").Inc()
		stats.markDropped()
		return
	}

	input := toEnrichedInput(fj.summary, fj.post, result)
	if err := w.pipeline.SaveEnrichedJob(ctx, input); err != nil {
		jobLog.Error("save enriched job failed", err)
		w.markError(ctx, fj.summary.ID)
		if aerr := fj.msg.Ack(); aerr != nil {
			jobLog.Warn("msg ack failed", logger.Err(aerr))
		}
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		stats.markRouteFailure()
		return
	}

	if aerr := fj.msg.Ack(); aerr != nil {
		jobLog.Warn("msg ack failed", logger.Err(aerr))
	}
	metrics.EnricherJobsTotal.WithLabelValues("saved").Inc()
	stats.markSaved()
	jobLog.Debug("enriched and saved",
		logger.Bool(logger.FieldRoleMatch, result.RoleMatch),
		logger.Int(logger.FieldTechStackCount, len(result.TechStack)),
	)
}

// routeBatchFailure routes every job in a failed batch via handleFailure.
// Each job carries its own retryCount, so the retry tier per job depends on
// how many times it has previously failed. Returns true if every job reached
// a terminal routing.
func (w *Worker) routeBatchFailure(ctx context.Context, fetchLog logger.Logger, group []fetchedJob, err error) bool {
	routeCtx, span := tracing.Start(ctx, "analyzer.worker.routeBatchFailure")
	defer span.End()

	var retried, dlqed, publishFailed int
	for _, fj := range group {
		ok := w.handleFailure(routeCtx, fetchLog, fj, stageEnrich, err)
		if !ok {
			publishFailed++
			continue
		}
		if fj.retryCount >= analyzerMaxRetries {
			dlqed++
		} else {
			retried++
		}
	}
	span.SetAttributes(
		attribute.Int("retried", retried),
		attribute.Int("dlqed", dlqed),
		attribute.Int("publish_failed", publishFailed),
	)
	return publishFailed == 0
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

// toEnrichedInput builds a jobdto.EnrichedJobInput from the canonical summary,
// the fetched post, and the LLM result.
func toEnrichedInput(summary core.JobSummary, post core.JobPost, result *llmdto.JobPostEnrichResult) jobdto.EnrichedJobInput {
	return jobdto.EnrichedJobInput{
		Summary:  summary,
		Post:     post,
		Enriched: result,
	}
}

// jobTitle returns the authoritative title for log context: the fetched
// Post.Title when non-empty, otherwise the Summary.Title.
func jobTitle(fj fetchedJob) string {
	if t := strings.TrimSpace(fj.post.Title); t != "" {
		return t
	}
	return fj.summary.Title
}

// marshalContent serializes the fetched JobPost to JSON for the LLM.
func marshalContent(post core.JobPost) (string, error) {
	bytes, err := json.Marshal(post)
	if err != nil {
		return "", fmt.Errorf("json marshal JobPost: %w", err)
	}
	return string(bytes), nil
}

// groupOutcomeStats tracks per-group routing outcomes for the
// enricher_group_duration_seconds{outcome} histogram.
type groupOutcomeStats struct {
	saved         int
	dropped       int
	routeFailures int
}

func (s *groupOutcomeStats) markSaved()        { s.saved++ }
func (s *groupOutcomeStats) markDropped()      { s.dropped++ }
func (s *groupOutcomeStats) markRouteFailure() { s.routeFailures++ }

func (s *groupOutcomeStats) label() string {
	if s.routeFailures > 0 {
		return "error"
	}
	if s.saved > 0 && s.dropped > 0 {
		return "mixed"
	}
	if s.saved > 0 {
		return "saved"
	}
	if s.dropped > 0 {
		return "dropped"
	}
	return "error"
}
