package llm

import "context"

// ScorerInput is the minimal job data sent to the LLM for batch scoring.
// We send title + id only — no full job text. Cheap and fast.
type ScorerInput struct {
	ID    string
	Title string
}

// EnrichInput is the full job text sent to the LLM for detail extraction.
type EnrichInput struct {
	ID      string
	RawText string // full scraped job page text
}

// EnrichResult is the structured data the LLM extracts from a full job page.
type EnrichResult struct {
	JobID           string
	ExperienceYears *int
	TechStack       []string
	SalaryMin       *int
	SalaryMax       *int
	SalaryCurrency  string
	Summary         string
	// Visa / clearance flags
	Sponsorship       bool
	RequiresCitizen   bool
	SecurityClearance bool
	// Role details
	RoleBrief        string
	Responsibilities string
}

// Scorer is the interface for LLM-based job relevance scoring and enrichment.
// Workers depend on this — never on a concrete provider.
type Scorer interface {
	// ScoreBatch takes a batch of jobs and returns the IDs of relevant ones.
	// Irrelevant jobs are simply absent from the returned slice.
	ScoreBatch(ctx context.Context, jobs []ScorerInput) (relevantIDs []string, err error)

	// Enrich extracts structured details from a full job page text.
	Enrich(ctx context.Context, input EnrichInput) (*EnrichResult, error)
}
