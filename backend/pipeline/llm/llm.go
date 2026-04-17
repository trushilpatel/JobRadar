package llm

import "context"

// ScorerInput is the minimal job data sent to the LLM for batch scoring.
// We send title + location + id — no full job text. Cheap and fast.
type ScorerInput struct {
	ID       string
	Title    string
	Location string
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
	// Location (structured)
	Remote  bool   // true if the role is remote (fully or partially)
	Country string // location restriction — "USA", "Canada", etc. Empty if no country restriction.
	State   string // US state abbreviation, e.g. "CA", "NY" — empty if not USA or unknown
	// Visa / clearance flags
	Sponsorship       bool
	RequiresCitizen   bool
	SecurityClearance bool
	// Role details
	RoleBrief        string
	Responsibilities []string
	RoleCategory     string // backend | frontend | fullstack | devops | ml | data | other
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
