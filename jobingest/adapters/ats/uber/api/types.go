package api

// FilterOptionsResponse is the top-level response from POST /api/loadFilterOptions.
type FilterOptionsResponse struct {
	Data FilterOptionsData `json:"data"`
}

// FilterOptionsData holds the filterable facets from the filter-options endpoint.
// Only Department is used for pagination; other facets are ignored.
type FilterOptionsData struct {
	// Department maps department name → list of sub-team names.
	// The map keys are used to paginate the job-search endpoint.
	Department map[string][]string `json:"department"`
}

// SearchJobsRequest is the POST body for /api/loadSearchJobsResults.
type SearchJobsRequest struct {
	Params SearchJobsParams `json:"params"`
}

// SearchJobsParams configures the job-search query.
type SearchJobsParams struct {
	Query      string   `json:"query"`
	Limit      int      `json:"limit"`
	Page       int      `json:"page"`
	Department []string `json:"department"`
}

// SearchJobsResponse is the top-level response from POST /api/loadSearchJobsResults.
type SearchJobsResponse struct {
	Status string         `json:"status"`
	Data   SearchJobsData `json:"data"`
}

// SearchJobsData holds the job results for a single department query.
type SearchJobsData struct {
	Results      []UberJob `json:"results"`
	TotalResults UberTotal `json:"totalResults"`
}

// UberTotal mirrors the protobuf Int64Value Uber uses for total counts.
// For realistic job counts, High is always 0; only Low is needed.
type UberTotal struct {
	Low      int  `json:"low"`
	High     int  `json:"high"`
	Unsigned bool `json:"unsigned"`
}

// UberJob is a single job entry in the search-results response.
// Description is full Markdown — no separate detail fetch is needed (fat adapter).
type UberJob struct {
	ID           int            `json:"id"`
	Title        string         `json:"title"`
	Description  string         `json:"description"` // full Markdown
	Department   string         `json:"department"`
	TimeType     string         `json:"timeType"` // "Full-Time", "Part-Time", "Contract", etc.
	Location     UberLocation   `json:"location"`
	AllLocations []UberLocation `json:"allLocations"`
	CreationDate string         `json:"creationDate"` // ISO 8601
}

// UberLocation is the structured location object in Uber job responses.
type UberLocation struct {
	Country     string `json:"country"`     // ISO alpha-3, e.g. "USA"
	CountryName string `json:"countryName"` // human-readable, e.g. "United States"
	City        string `json:"city"`
	Region      string `json:"region"` // state/province
}
