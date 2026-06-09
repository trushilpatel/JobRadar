package api

// ── Listing response types ────────────────────────────────────────────────────

// HydrationData is the top-level shape of window.__staticRouterHydrationData
// on listing pages.
type HydrationData struct {
	LoaderData LoaderData `json:"loaderData"`
}

// LoaderData holds the search section on listing pages.
type LoaderData struct {
	Search SearchData `json:"search"`
}

// SearchData contains the paginated search results from the listing endpoint.
type SearchData struct {
	SearchResults []Job `json:"searchResults"`
	TotalRecords  int   `json:"totalRecords"`
}

// Job represents one entry in searchResults on the listing page.
type Job struct {
	ID                      string     `json:"id"`
	PositionID              string     `json:"positionId"`
	PostingTitle            string     `json:"postingTitle"`
	TransformedPostingTitle string     `json:"transformedPostingTitle"`
	JobSummary              string     `json:"jobSummary"`
	Locations               []Location `json:"locations"`
	PostDateInGMT           string     `json:"postDateInGMT"`
	Team                    Team       `json:"team"`
	HomeOffice              bool       `json:"homeOffice"`
	WorkHours               string     `json:"workHours"`
	Type                    string     `json:"type"`
	ReqID                   string     `json:"reqId"`
	IsMultiLocation         bool       `json:"isMultiLocation"`
}

// Location captures location fields from both listing and detail responses.
// Listing payloads populate name, countryID, and countryName.
// Detail payloads populate the full structure (city, stateProvince, zipCode, etc.).
// Older payloads may carry stateCode/countryCode — both shapes are tolerated.
type Location struct {
	PostLocationID string `json:"postLocationId"`
	Name           string `json:"name"`
	City           string `json:"city"`
	Code           string `json:"code"`
	StateProvince  string `json:"stateProvince"`
	StateCode      string `json:"stateCode"`
	CountryName    string `json:"countryName"`
	CountryCode    string `json:"countryCode"`
	CountryID      string `json:"countryID"`
	Metro          string `json:"metro"`
	Region         string `json:"region"`
	ZipCode        string `json:"zipCode"`
	Level          int    `json:"level"`
}

// Team holds the team metadata attached to a listing job.
type Team struct {
	TeamName string `json:"teamName"`
	TeamCode string `json:"teamCode"`
	TeamID   string `json:"teamID"`
}

// ── Detail response types ─────────────────────────────────────────────────────

// DetailHydrationData is the top-level hydration shape on a job detail page.
type DetailHydrationData struct {
	LoaderData DetailLoaderData `json:"loaderData"`
}

// DetailLoaderData holds the job detail section.
type DetailLoaderData struct {
	JobDetails JobDetails `json:"jobDetails"`
}

// JobDetails wraps the jobsData payload on the detail page.
type JobDetails struct {
	JobsData JobDetail `json:"jobsData"`
}

// JobDetail mirrors loaderData.jobDetails.jobsData on the detail page.
type JobDetail struct {
	ID                      string     `json:"id"`
	JobNumber               string     `json:"jobNumber"`
	PositionID              string     `json:"positionId"`
	ReqID                   string     `json:"reqId"`
	PostingTitle            string     `json:"postingTitle"`
	TransformedPostingTitle string     `json:"transformedPostingTitle"`
	JobSummary              string     `json:"jobSummary"`
	BoardDescription        string     `json:"boardDescription"`
	Description             string     `json:"description"`
	Responsibilities        string     `json:"responsibilities"`
	MinimumQualifications   string     `json:"minimumQualifications"`
	PreferredQualifications string     `json:"preferredQualifications"`
	HighJobTitle            string     `json:"highJobTitle"`
	LowJobTitle             string     `json:"lowJobTitle"`
	TeamNames               []string   `json:"teamNames"`
	Locations               []Location `json:"locations"`
	EmploymentType          string     `json:"employmentType"`
	JobType                 string     `json:"jobType"`
	Type                    string     `json:"type"`
	HomeOffice              bool       `json:"homeOffice"`
	PostDateInGMT           string     `json:"postDateInGMT"`
	PostingFooters          []Footer   `json:"postingFooters"`
}

// Footer is one entry in jobsData.postingFooters.
// The pay-range string lives in the entry whose Type starts with "POSTING_FOOTER_RULES".
type Footer struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}
