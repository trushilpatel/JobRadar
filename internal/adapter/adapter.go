package adapter

import "context"

// RawJob is the raw job payload fetched from a careers page.
// This is what gets published to the jobs.raw Kafka topic.
type RawJob struct {
	ID         string // e.g. "greenhouse-12345"
	Title      string
	Location   string
	URL        string
	Department string
	EmployType string
}

// Adapter fetches the current job listings for a company.
type Adapter interface {
	FetchJobs(ctx context.Context, companyURL string) ([]RawJob, error)
}
