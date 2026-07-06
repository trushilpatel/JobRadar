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

// JobPostEnricher extracts structured data from one or more job postings in a
// single LLM call. Inputs and results are correlated by JobID — the LLM is
// instructed to return the same job_id on each result, and the caller maps
// results back by ID. A batch of size 1 is the natural single-job case.
//
// Used by the analyzer worker.
type JobPostEnricher interface {
	EnrichBatch(ctx context.Context, inputs []dto.JobPostEnrichInput) ([]*dto.JobPostEnrichResult, error)
}

// Scorer combines both interfaces. Returned by the factory — callers
// receive the narrower interface they actually need.
type Scorer interface {
	JobsRelevanceFilter
	JobPostEnricher
}
