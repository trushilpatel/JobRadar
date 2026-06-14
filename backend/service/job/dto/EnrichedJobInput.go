package dto

import (
	"github.com/trushilpatel/jobradar/common/ats/core"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
)

// EnrichedJobInput carries all data the analyzer has enriched before writing to DB.
// It bundles the canonical JobSummary (stable identity from the crawler), the
// fetched JobPost (authoritative title/location/employment type for the row),
// and the LLM's structured extraction result.
type EnrichedJobInput struct {
	Summary  core.JobSummary
	Post     core.JobPost
	Enriched *llmdto.JobPostEnrichResult
}
