// Package api holds the raw JSON response types for the Eightfold AI SmartApply
// API (/api/apply/v2). The same Position struct decodes both list and detail
// responses because the detail endpoint returns a flat position object at the
// top level.
package api

// ListResponse is the top-level JSON from /api/apply/v2/jobs.
type ListResponse struct {
	Positions []Position `json:"positions"`
	Count     int        `json:"count"`
}

// Position mirrors a single entry in the Eightfold positions array.
// IDs and timestamps are numbers in the real API — not strings.
// Some hosts return t_create as 0 for unpublished/draft postings.
type Position struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"name"`
	Location           string   `json:"location"`
	Locations          []string `json:"locations"`
	Department         string   `json:"department"`
	TCreate            int64    `json:"t_create"`
	WorkLocationOption string   `json:"work_location_option"`
	CanonicalURL       string   `json:"canonicalPositionUrl"`
	JobDescription     string   `json:"job_description"`
}
