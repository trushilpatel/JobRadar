package main

// WorkUnit is the assignment handed to this node by the orchestrator.
// It mirrors service/crawlernode/dto.WorkUnit but is local to the binary
// so the node has no dependency on the orchestrator's service packages.
type WorkUnit struct {
	AssignmentID  string   `json:"id"`
	CompanyID     string   `json:"company_id"`
	AdapterType   string   `json:"adapter_type"`
	AdapterConfig *string  `json:"adapter_config"`
	KnownIDs      []string `json:"known_ids"`
	Source        string   `json:"source"`
}

// CrawledJobContent is full job detail from fat adapters.
type CrawledJobContent struct {
	Description      string `json:"description"`
	Responsibilities string `json:"responsibilities"`
	Requirements     string `json:"requirements"`
	Benefits         string `json:"benefits"`
	Salary           string `json:"salary"`
	Department       string `json:"department"`
}

// CrawledJob is a single job within a PageBatch.
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

// PageBatch is the payload submitted to POST /node/results.
// Set Final to true on the last batch (or the only batch for non-streaming adapters).
type PageBatch struct {
	AssignmentID     string       `json:"assignment_id"`
	CompanyID        string       `json:"company_id"`
	AdapterType      string       `json:"adapter_type"`
	Jobs             []CrawledJob `json:"jobs"`
	PreviousKnownIDs []string     `json:"previous_known_ids"`
	FetchedCount     int          `json:"fetched_count"`
	DurationMs       int64        `json:"duration_ms"`
	Final            bool         `json:"final"`
}

// WorkError is the payload submitted to POST /node/error.
type WorkError struct {
	AssignmentID string `json:"assignment_id"`
	CompanyID    string `json:"company_id"`
	AdapterType  string `json:"adapter_type"`
	ErrorMessage string `json:"error_message"`
	DurationMs   int64  `json:"duration_ms"`
}

// GetWorkResult carries both the (optional) new assignment and the heartbeat
// outcome. HeartbeatMissed contains assignment IDs the orchestrator no longer
// holds — the PollLoop must remove them from its holding list.
type GetWorkResult struct {
	Assignment      *WorkUnit // nil when queue is empty
	HeartbeatMissed []string  // assignment IDs that expired on the orchestrator
}
