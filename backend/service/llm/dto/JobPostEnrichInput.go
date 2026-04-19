package dto

// JobPostEnrichInput is the full job page text sent to the LLM for detail extraction.
type JobPostEnrichInput struct {
	ID      string
	RawText string
}
