package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/llm/dto"
)

// JobsRelevanceFilter scores a batch of jobs and returns the IDs of relevant ones.
// Used by the filter worker.
type JobsRelevanceFilter interface {
	ScoreBatch(ctx context.Context, jobs []dto.JobsRelevanceFilterInput) (relevantIDs []string, err error)
}

// JobPostEnricher extracts structured data from a single job posting.
// Used by the analyzer worker.
type JobPostEnricher interface {
	Enrich(ctx context.Context, input dto.JobPostEnrichInput) (*dto.JobPostEnrichResult, error)
}

// Scorer combines both interfaces. Returned by the factory — callers receive
// the narrower interface they actually need.
type Scorer interface {
	JobsRelevanceFilter
	JobPostEnricher
}
