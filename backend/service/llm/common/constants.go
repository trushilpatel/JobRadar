package common

const (
	ProviderGemini = "gemini"

	// ResumeKeywordMaxTextChars bounds the resume plain text sent to
	// ExtractResumeKeywords. ~4 chars/token (the same rough ratio
	// LLMRateLimitService.estimateEnrichBatchTokens uses), so this caps the
	// prompt around ~6k tokens — generous for a multi-page resume while
	// keeping a single upload's token spend bounded and predictable.
	ResumeKeywordMaxTextChars = 24000
)
