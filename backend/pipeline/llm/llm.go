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
	JobID           string   `json:"-"`
	ExperienceYears *int     `json:"experience_years"`
	TechStack       []string `json:"tech_stack"`
	SalaryMin       *int     `json:"salary_min"`
	SalaryMax       *int     `json:"salary_max"`
	SalaryCurrency  string   `json:"salary_currency"`
	Summary         string   `json:"summary"`
	// Location (structured)
	Remote  bool   `json:"remote"`  // true if the role is remote (fully or partially)
	Country string `json:"country"` // location restriction — "USA", "Canada", etc. Empty if no country restriction.
	State   string `json:"state"`   // US state abbreviation, e.g. "CA", "NY" — empty if not USA or unknown
	// Visa / clearance flags
	Sponsorship       bool `json:"sponsorship"`
	RequiresCitizen   bool `json:"requires_citizen"`
	SecurityClearance bool `json:"security_clearance"`
	// Role details
	RoleBrief        string   `json:"role_brief"`
	Responsibilities []string `json:"responsibilities"`
	RoleCategory     string   `json:"role_category"` // backend | frontend | fullstack | devops | ml | data | other
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
