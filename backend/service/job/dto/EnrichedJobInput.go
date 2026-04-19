package dto

import (
	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
)

// EnrichedJobInput carries all data the analyzer has enriched before writing to DB.
// It bundles the original RawJob from the ATS and the LLM's structured extraction result.
type EnrichedJobInput struct {
	Raw      ats.RawJob
	Enriched *llmdto.JobPostEnrichResult
}
