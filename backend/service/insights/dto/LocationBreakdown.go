package dto

// LocationBreakdown holds the country and state breakdown for the Location insight.
type LocationBreakdown struct {
	Countries []LocationCount `json:"countries"`
	States    []StateCount    `json:"states"`
}
