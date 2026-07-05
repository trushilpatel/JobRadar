package dto

// ResumeKeywordResult is the structured data the LLM extracts from a resume
// body — raw technology/skill keywords as named by the model. Callers run
// this through service/job/techtag.Clean for canonical normalization before
// persisting; this type carries the pre-normalization raw result.
type ResumeKeywordResult struct {
	Keywords []string `json:"keywords"`
}
