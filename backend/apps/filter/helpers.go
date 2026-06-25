package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/common/ats/core"
	domainerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/worker"
)

// clock is a package-level alias for the shared worker.Clock interface so
// existing field declarations and test doubles need no changes.
type clock = worker.Clock

// realClock is the production implementation from the shared worker package.
type realClock = worker.RealClock

// splitBySize divides s into chunks of at most size elements each.
func splitBySize(s []chunkItem, size int) [][]chunkItem {
	if size < 1 {
		size = 1
	}
	chunks := make([][]chunkItem, 0, (len(s)+size-1)/size)
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[i:end])
	}
	return chunks
}

// mergedLocations returns the slice to pass to location.AnyNonUSA.
// If AllLocations is populated it is used directly; otherwise the single
// Location string is wrapped in a one-element slice for backward compatibility
// with thin-listing adapters that pre-date the AllLocations field.
func mergedLocations(job core.JobSummary) []string {
	if len(job.AllLocations) > 0 {
		return job.AllLocations
	}
	if job.Location != "" {
		return []string{job.Location}
	}
	return nil
}

// formatSlotStatuses renders per-slot exhaustion detail for log output.
func formatSlotStatuses(ss []domainerr.LLMSlotStatus) string {
	if len(ss) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ss))
	for _, s := range ss {
		cd := "-"
		if !s.CooldownUntil.IsZero() {
			cd = s.CooldownUntil.UTC().Format(time.RFC3339)
		}
		parts = append(parts, fmt.Sprintf("%s=%s(fail=%d,cd=%s)", s.SlotID, s.LastDenial, s.FailureCount, cd))
	}
	return strings.Join(parts, "; ")
}

// nakRemainingChunks Naks every message in the given chunks so NATS redelivers
// them immediately instead of waiting for AckWait. Called when processFetch is
// abandoning unprocessed chunks because the worker is shutting down.
func (w *Worker) nakRemainingChunks(chunks [][]chunkItem, fetchID, why string) {
	total := 0
	for _, ch := range chunks {
		total += len(ch)
	}
	if total == 0 {
		return
	}
	w.log.Warn("naking unprocessed chunks for redelivery",
		logger.Str(logger.FieldFetchID, fetchID),
		logger.Int(logger.FieldRecords, total),
		logger.Str(logger.FieldReason, why),
	)
	for _, ch := range chunks {
		for _, it := range ch {
			if nerr := it.msg.Nak(); nerr != nil {
				w.log.Warn("msg nak failed", logger.Err(nerr), logger.Str(logger.FieldJobID, it.job.job.ID))
			}
		}
	}
}
