package dto

// CrawledJobContent is full job detail populated by fat adapters at scan time.
type CrawledJobContent struct {
	Description      string `json:"description"`
	Responsibilities string `json:"responsibilities"`
	Requirements     string `json:"requirements"`
	Benefits         string `json:"benefits"`
	Salary           string `json:"salary"`
	Department       string `json:"department"`
}

// CrawledJob is a single job within a ResultsRequest.
type CrawledJob struct {
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	URL            string             `json:"url"`
	Location       string             `json:"location"`
	EmploymentType string             `json:"employment_type"`
	LocationKnown  bool               `json:"location_known"`
	AllLocations   []string           `json:"all_locations"`
	FullContent    *CrawledJobContent `json:"full_content,omitempty"`
}

// ResultsRequest is the body of POST /node/results.
type ResultsRequest struct {
	AssignmentID     string       `json:"assignment_id"`
	CompanyID        string       `json:"company_id"`
	AdapterType      string       `json:"adapter_type"`
	Jobs             []CrawledJob `json:"jobs"`
	PreviousKnownIDs []string     `json:"previous_known_ids"`
	FetchedCount     int          `json:"fetched_count"`
	DurationMs       int64        `json:"duration_ms"`
	Final            bool         `json:"final"`
}

// ResultsResponse is returned by POST /node/results.
type ResultsResponse struct {
	Published int  `json:"published"` // jobs successfully published to NATS
	New       int  `json:"new"`       // jobs not previously seen
	Removed   int  `json:"removed"`   // jobs in previous_known_ids but absent from current batch
	Completed bool `json:"completed"` // true when final:true and queue row deleted
}

// ScanResult carries the outcome summary written to company_scan_log.
type ScanResult struct {
	FetchedCount int
	NewCount     int
	DurationMs   int64
	ErrMsg       string // empty means success
}
