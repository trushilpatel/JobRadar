package api

// ParsedJob is the intermediate result from parsing a single listing-page card.
// It holds the fields extracted by regex from the HTML before they are mapped
// to core.JobSummary.
type ParsedJob struct {
	ID       string // numeric Google job ID, e.g. "12345"
	Slug     string // URL slug, e.g. "software-engineer-search"
	Title    string
	Location string
	Team     string // from corporate_fare icon block
	URL      string // canonical detail URL
}

// JobDetail holds the fields extracted from a detail page's AF_initDataCallback
// block before they are mapped to core.JobPost.
type JobDetail struct {
	Title       string
	Description string // plain text; HTML stripped by the caller
	Location    string
	Department  string
	PostedDate  string
}
