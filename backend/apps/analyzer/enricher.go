package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	"github.com/trushilpatel/jobradar/common/tracing"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
)

// enrichGroup runs one EnrichBatch call for a group of fetched jobs and
// processes the per-job results.
//
// Failure modes:
//
//   - Whole batch fails  → every job in the group is routed via handleFailure
//     using its own retryCount; the retry consumer naturally re-batches them
//     with whatever else arrives next.
//   - Result missing for a job → that job is treated as a transient failure
//     and routed to retry. We do not silently drop it.
//   - Extra unknown job_ids in the response → ignored with a warning log.
//   - Per-job save error → logged as error, ledger marked, other jobs in the
//     same batch are unaffected.
func (w *Worker) enrichGroup(ctx context.Context, group []fetchedJob) {
	if len(group) == 0 {
		return
	}

	groupCtx, span := tracing.Start(ctx, "analyzer.worker.enrichGroup")
	defer tracing.End(span, nil)

	jobIDs := make([]string, len(group))
	for i, fj := range group {
		jobIDs[i] = fj.job.ID
	}
	w.log.Info("LLM batch sent",
		logger.Int("batch_size", len(group)),
		logger.Str("job_ids", strings.Join(jobIDs, ",")),
	)

	inputs, err := w.buildEnrichInputs(group)
	if err != nil {
		w.log.Error("build enrich inputs failed", err)
		w.routeBatchFailure(groupCtx, group, err)
		return
	}

	llmStart := time.Now()
	results, err := w.scorer.EnrichBatch(groupCtx, inputs)
	metrics.EnricherLLMDuration.Observe(time.Since(llmStart).Seconds())

	if err != nil {
		w.log.Error("EnrichBatch failed, routing whole batch", err,
			logger.Int("batch_size", len(group)),
		)
		w.routeBatchFailure(groupCtx, group, err)
		return
	}

	w.log.Info("LLM batch returned",
		logger.Int("batch_size", len(group)),
		logger.Int("result_count", len(results)),
		logger.Dur("duration", time.Since(llmStart)),
	)

	w.distributeResults(groupCtx, group, results)
}

// buildEnrichInputs marshals each fetched job's content into the input shape
// the LLM expects.
func (w *Worker) buildEnrichInputs(group []fetchedJob) ([]llmdto.JobPostEnrichInput, error) {
	inputs := make([]llmdto.JobPostEnrichInput, 0, len(group))
	for _, fj := range group {
		raw, err := marshalContent(fj.content)
		if err != nil {
			return nil, fmt.Errorf("marshal content for %s: %w", fj.job.ID, err)
		}
		inputs = append(inputs, llmdto.JobPostEnrichInput{
			ID:      fj.job.ID,
			RawText: raw,
		})
	}
	return inputs, nil
}

// distributeResults walks the LLM response and routes each job to save,
// drop, or retry. Results are matched by JobID — order from the model is
// not trusted.
func (w *Worker) distributeResults(ctx context.Context, group []fetchedJob, results []*llmdto.JobPostEnrichResult) {
	resultByID := make(map[string]*llmdto.JobPostEnrichResult, len(results))
	for _, r := range results {
		if r == nil {
			continue
		}
		if _, dup := resultByID[r.JobID]; dup {
			w.log.Warn("LLM returned duplicate job_id, keeping first",
				logger.Str(logger.FieldJobID, r.JobID),
			)
			continue
		}
		resultByID[r.JobID] = r
	}

	groupIDs := make(map[string]struct{}, len(group))
	for _, fj := range group {
		groupIDs[fj.job.ID] = struct{}{}
	}
	for id := range resultByID {
		if _, ok := groupIDs[id]; !ok {
			w.log.Warn("LLM returned unknown job_id, ignoring",
				logger.Str(logger.FieldJobID, id),
			)
		}
	}

	for _, fj := range group {
		result, ok := resultByID[fj.job.ID]
		if !ok {
			w.log.Error("LLM result missing for job, routing to retry",
				fmt.Errorf("missing job_id %s in EnrichBatch response", fj.job.ID),
				logger.Str(logger.FieldJobID, fj.job.ID),
			)
			metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
			w.handleFailure(fj.ctx, fj.job, fj.value, fj.retryCount,
				fmt.Errorf("LLM result missing for job_id %s", fj.job.ID))
			continue
		}
		w.processResult(fj.ctx, fj.job, result)
	}
}

// processResult applies the drop rules and either saves the job or marks
// the ledger row appropriately. Mirrors the original single-job path.
func (w *Worker) processResult(ctx context.Context, job ats.RawJob, result *llmdto.JobPostEnrichResult) {
	jobLog := w.log.With(
		logger.Str(logger.FieldJobID, job.ID),
		logger.Str("title", job.Title),
	)

	if !result.RoleMatch {
		jobLog.Debug("job dropped: role mismatch")
		w.markIrrelevant(ctx, job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("dropped").Inc()
		return
	}

	if reason := w.dropJob(result); reason != "" {
		jobLog.Info("job dropped", logger.Str("drop_reason", reason))
		w.markError(ctx, job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("dropped").Inc()
		return
	}

	input := toEnrichedInput(job, result)
	if err := w.pipeline.SaveEnrichedJob(ctx, input); err != nil {
		jobLog.Error("save enriched job failed", err)
		w.markError(ctx, job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		return
	}

	metrics.EnricherJobsTotal.WithLabelValues("saved").Inc()
	jobLog.Debug("enriched and saved",
		logger.Bool("role_match", result.RoleMatch),
		logger.Int("tech_stack_count", len(result.TechStack)),
	)
}

// routeBatchFailure routes every job in a failed batch via handleFailure.
// Each job carries its own retryCount, so the retry tier per job depends on
// how many times it has previously failed (not how many times the current
// batch failed).
func (w *Worker) routeBatchFailure(ctx context.Context, group []fetchedJob, err error) {
	for _, fj := range group {
		w.handleFailure(fj.ctx, fj.job, fj.value, fj.retryCount, err)
	}
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

// toEnrichedInput builds a jobdto.EnrichedJobInput from the raw job and LLM result.
func toEnrichedInput(job ats.RawJob, result *llmdto.JobPostEnrichResult) jobdto.EnrichedJobInput {
	return jobdto.EnrichedJobInput{
		Raw:      job,
		Enriched: result,
	}
}

// marshalContent serializes the structured content to JSON for the LLM.
// JSON gives the LLM all available fields (description, compensation, etc.)
// in a compact, consistent format regardless of which platform fetched it.
func marshalContent(content ats.FetchJobContentResponse) (string, error) {
	bytes, err := json.Marshal(content)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
