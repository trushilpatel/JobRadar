package adapter

import "context"

// RawJob is the raw job payload fetched from a careers page.
// This is what gets published to the jobs.raw Kafka topic.
type RawJob struct {
	ID         string `json:"id"`      // e.g. "greenhouse-12345"
	Adapter    string `json:"adapter"` // the adapter type (e.g. "greenhouse", "lever", "ashby", "workday", "workable", "chromedp")
	Title      string `json:"title"`
	Location   string `json:"location"`
	URL        string `json:"url"` // the individual job posting URL (used by enricher)
	Department string `json:"department"`
	EmployType string `json:"employ_type"` // full-time, contract, part-time
}

// Adapter fetches the current job listings for a company.
type Adapter interface {
	FetchJobs(ctx context.Context, companyURL string) ([]RawJob, error)
}
