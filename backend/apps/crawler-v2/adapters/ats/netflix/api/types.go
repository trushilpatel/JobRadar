package api

// ListResponse is the top-level JSON response from /api/apply/v2/jobs.
type ListResponse struct {
	Positions []Position `json:"positions"`
	Count     int        `json:"count"`
}

// Position mirrors a single entry in the Eightfold positions array.
// The same struct decodes both list and detail responses since the detail
// endpoint returns a flat position object at the top level.
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
