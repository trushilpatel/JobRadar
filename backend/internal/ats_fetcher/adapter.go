package ats_fetcher

import "context"

// RawJob is the raw job payload fetched from a careers page.
// This is what gets published to the jobs.raw Kafka topic.
type RawJob struct {
	ID         string `json:"id"`         // e.g. "greenhouse-12345"
	CompanyID  string `json:"company_id"` // UUID from companies table — set by scan engine before publishing
	Adapter    string `json:"adapter"`    // the adapter type (e.g. "greenhouse", "lever", "ashby", "workday", "workable", "chromedp")
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

// ContentFetcher is an optional interface for adapters that can fetch the full
// content of a single job posting via their native API.
//
// Adapters that implement this return structured text (typically JSON) which is
// cleaner for LLM enrichment than raw HTML. Adapters that don't implement this
// fall back to a plain HTTP GET on job.URL.
//
// Implementing this interface is optional — only do it when the adapter has a
// better data source than scraping the hosted job URL.
type ContentFetcher interface {
	FetchJobContent(ctx context.Context, job RawJob) (string, error)
}
