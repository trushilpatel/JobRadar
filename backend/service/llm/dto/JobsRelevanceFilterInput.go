package dto

// JobsRelevanceFilterInput is the minimal job data sent to the LLM for batch scoring.
// Only title + location + id are sent — cheap and fast.
type JobsRelevanceFilterInput struct {
	ID       string
	Title    string
	Location string
}
