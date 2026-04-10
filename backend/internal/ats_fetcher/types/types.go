package types

import (
	"context"
	"html"
	"regexp"
	"strings"
)

// RawJob is the Kafka message published to job_listings.raw.
// Set by the scan engine after FetchJobs returns — never by the adapter itself.
type RawJob struct {
	ID         string `json:"id"`          // e.g. "greenhouse-12345"
	CompanyID  string `json:"company_id"`  // UUID from companies table
	Adapter    string `json:"adapter"`     // "greenhouse", "lever", "ashby", etc.
	Title      string `json:"title"`
	Location   string `json:"location"`
	URL        string `json:"url"`
	EmployType string `json:"employ_type"` // full_time, contract, part_time, internship
}

// FetchJobsItem is one job from a company's listing page.
// Minimal — only what's needed to dedup and publish to Kafka.
type FetchJobsItem struct {
	ID             string
	Title          string
	URL            string
	Location       string
	EmploymentType string
}

// FetchJobsResponse is returned by Adapter.FetchJobs.
type FetchJobsResponse struct {
	Jobs []FetchJobsItem
}

// FetchJobContentResponse is returned by ContentFetcher.FetchJobContent.
// Union of all fields available across Greenhouse, Lever, and Ashby.
// Fields not provided by a platform are left as empty strings.
type FetchJobContentResponse struct {
	ID             string // all platforms
	Title          string // all platforms
	Location       string // all platforms
	URL            string // Greenhouse, Lever
	Description    string // plain text — HTML stripped from all platforms
	EmploymentType string // Lever (commitment), Ashby (employmentType)
	Department     string // Greenhouse (departments[0]), Lever (department > team fallback)
	Compensation   string // Greenhouse (pay_input_ranges), Lever (salaryDescriptionPlain > salaryRange), Ashby (compensationTierSummary)
	WorkplaceType  string // Lever (workplaceType), Ashby (workplaceType)
	Requirements   string // plain text — Lever (lists[])
	PublishedDate  string // Lever (createdAt ms), Ashby (publishedDate), Greenhouse (updated_at)
}

// Adapter fetches the current job listings for a company.
// Required — implemented by all adapters.
type Adapter interface {
	FetchJobs(ctx context.Context, companyID string) (FetchJobsResponse, error)
}

// URLProvider is an optional interface for adapters that know how to construct
// a canonical careers page URL from a company slug.
//
// Implemented by Greenhouse, Lever, and Ashby. Adapters that scrape arbitrary
// URLs (Workday, Workable, Chromedp) do not implement this — the stored
// company.URL is used directly for display.
type URLProvider interface {
	CareersURL(id string) string
}

// ContentFetcher is an optional interface for adapters that can fetch the full
// content of a single job posting via their native API.
//
// Implement this when the platform has a structured JSON API cleaner than
// scraping the hosted HTML page. Adapters without this fall back to a plain
// HTTP GET on RawJob.URL in the job_post_analyzer.
type ContentFetcher interface {
	FetchJobContent(ctx context.Context, job RawJob) (FetchJobContentResponse, error)
}

// ── HTML helpers ──────────────────────────────────────────────────────────────

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

// StripHTML removes all HTML tags, decodes entities, and collapses whitespace.
func StripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}
