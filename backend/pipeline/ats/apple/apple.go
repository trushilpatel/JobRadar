package apple

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/pipeline/ats/types"
)

const (
	defaultBaseURL = "https://jobs.apple.com"

	// listingPath is the search endpoint with US filter applied.
	listingPath = "/en-us/search?location=united-states-USA&page=%d"

	// userAgent mimics a real browser to avoid blocking.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

	// publicJobURLTemplate constructs the public job detail page from id and transformedPostingTitle.
	publicJobURLTemplate = "%s/en-us/details/%s/%s"
)

// Apple implements types.Adapter by scraping jobs.apple.com listing pages.
//
// FetchJobs:
//  1. Paginates through /en-us/search?location=united-states-USA&page={n}
//  2. Extracts window.__staticRouterHydrationData from each page's HTML
//  3. Parses job details including full jobSummary (always provided)
//  4. Returns all jobs with FullContent always populated (fat adapter)
//
// No ContentFetcher is implemented — the listing page contains the complete
// job description in jobSummary, so a separate detail fetch is not needed.
type Apple struct {
	// baseURL overrides the Apple host — used in tests.
	baseURL string
}

// New creates an Apple adapter with default production settings.
func New() *Apple {
	return &Apple{baseURL: defaultBaseURL}
}

// CareersURL returns the public Apple careers page URL.
func (a *Apple) CareersURL(_ string) string {
	return "https://www.apple.com/careers/us/"
}

// ── Response types ──────────────────────────────────────────────────────────

// hydrationData is the structure of window.__staticRouterHydrationData from Apple's pages.
type hydrationData struct {
	LoaderData loaderData  `json:"loaderData"`
	ActionData interface{} `json:"actionData"`
	Errors     interface{} `json:"errors"`
}

type loaderData struct {
	Search searchData `json:"search"`
}

type searchData struct {
	SearchResults []appleJob `json:"searchResults"`
	TotalRecords  int        `json:"totalRecords"`
}

// appleJob represents a single job in the searchResults array.
type appleJob struct {
	ID                      string          `json:"id"`                      // e.g. "200647207-3543"
	PositionID              string          `json:"positionId"`              // numeric ID
	PostingTitle            string          `json:"postingTitle"`            // e.g. "Software Engineer"
	TransformedPostingTitle string          `json:"transformedPostingTitle"` // e.g. "software-engineer" (URL slug)
	JobSummary              string          `json:"jobSummary"`              // HTML description
	Locations               []appleLocation `json:"locations"`               // job locations
	PostDateInGMT           string          `json:"postDateInGMT"`           // e.g. "2026-03-15 00:00:00"
	Team                    appleTeam       `json:"team"`                    // department info
	HomeOffice              bool            `json:"homeOffice"`
	StandardWeeklyHours     int             `json:"standardWeeklyHours"`
	Type                    string          `json:"type"`  // "Regular", "Part-Time", "Contract", "Intern"
	ReqID                   string          `json:"reqId"` // requirement ID
	IsMultiLocation         bool            `json:"isMultiLocation"`
}

// appleLocation represents a single location in the job's locations array.
type appleLocation struct {
	City        string `json:"city"`
	StateCode   string `json:"stateCode"`
	CountryCode string `json:"countryCode"`
}

// appleTeam represents the team/department information.
type appleTeam struct {
	TeamName string `json:"teamName"`
	TeamCode string `json:"teamCode"`
}

// ── Adapter implementation ───────────────────────────────────────────────────

// FetchJobs returns all current Apple job listings from the US careers page.
// Each job's full description is included via jobSummary — FullContent is always set.
// companyID is ignored (Apple adapter targets a fixed host).
func (a *Apple) FetchJobs(ctx context.Context, _ string) (types.FetchJobsResponse, error) {
	var allJobs []types.FetchJobsItem
	page := 1

	for {
		select {
		case <-ctx.Done():
			return types.FetchJobsResponse{}, ctx.Err()
		default:
		}

		pageJobs, err := a.fetchJobsPage(ctx, page)
		if err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("apple: fetch page %d: %w", page, err)
		}

		// Stop when we get an empty page
		if len(pageJobs) == 0 {
			break
		}

		allJobs = append(allJobs, pageJobs...)
		page++
	}

	return types.FetchJobsResponse{Jobs: allJobs}, nil
}

// fetchJobsPage fetches a single page of job listings and returns FetchJobsItems.
func (a *Apple) fetchJobsPage(ctx context.Context, page int) ([]types.FetchJobsItem, error) {
	url := a.resolveBaseURL() + fmt.Sprintf(listingPath, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	a.setHeaders(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// Extract and parse the hydration data from the HTML
	hydrationStr, err := extractHydrationData(string(body))
	if err != nil {
		return nil, fmt.Errorf("extract hydration data: %w", err)
	}

	var data hydrationData
	if err := json.Unmarshal([]byte(hydrationStr), &data); err != nil {
		return nil, fmt.Errorf("decode hydration JSON: %w", err)
	}

	// Convert appleJob to FetchJobsItem
	var items []types.FetchJobsItem
	for _, job := range data.LoaderData.Search.SearchResults {
		items = append(items, jobToItem(job))
	}

	return items, nil
}

// setHeaders applies required headers to every request.
func (a *Apple) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
}

// ── Converters and helpers ──────────────────────────────────────────────────

// extractHydrationData finds and returns the JSON string from window.__staticRouterHydrationData.
// Handles both "= {...};</script>" (same line) and "= {...};\n</script>" (newline) variants.
func extractHydrationData(body string) (string, error) {
	const marker = "window.__staticRouterHydrationData = "
	idx := strings.Index(body, marker)
	if idx == -1 {
		return "", fmt.Errorf("window.__staticRouterHydrationData not found in page")
	}

	start := idx + len(marker)
	// Find the closing </script> tag after the start position.
	// The JSON ends with ";" followed by optional whitespace then "</script>".
	endIdx := strings.Index(body[start:], "</script>")
	if endIdx == -1 {
		return "", fmt.Errorf("closing </script> not found after hydration marker")
	}

	// Trim trailing whitespace and the trailing semicolon from the JSON
	raw := strings.TrimRight(body[start:start+endIdx], " \t\r\n;")
	return raw, nil
}

// jobToItem converts an appleJob to a FetchJobsItem with FullContent populated.
func jobToItem(job appleJob) types.FetchJobsItem {
	return types.FetchJobsItem{
		ID:             fmt.Sprintf("%s-%s", model.AdapterApple, job.ID),
		Title:          job.PostingTitle,
		URL:            fmt.Sprintf(publicJobURLTemplate, defaultBaseURL, job.ID, job.TransformedPostingTitle),
		Location:       formatLocation(job.Locations),
		EmploymentType: mapJobType(job.Type),
		FullContent: &types.FullJobContent{
			Description:   types.StripHTML(job.JobSummary),
			Department:    job.Team.TeamName,
			PublishedDate: job.PostDateInGMT,
			WorkplaceType: "", // not available in listing API
			Compensation:  "", // not available in listing API
		},
	}
}

// formatLocation builds a human-readable location string from the job's locations array.
// Single location: "City, State". Multiple: "City, State; City, State; ...".
func formatLocation(locations []appleLocation) string {
	if len(locations) == 0 {
		return ""
	}

	parts := make([]string, 0, len(locations))
	for _, loc := range locations {
		if s := formatSingleLocation(loc); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "; ")
}

// formatSingleLocation renders an appleLocation as "City, State".
func formatSingleLocation(loc appleLocation) string {
	parts := make([]string, 0, 2)
	if loc.City != "" {
		parts = append(parts, loc.City)
	}
	if loc.StateCode != "" {
		parts = append(parts, loc.StateCode)
	}
	return strings.Join(parts, ", ")
}

// mapJobType converts Apple's type string to a model.EmployType constant.
func mapJobType(jobType string) string {
	switch strings.ToLower(strings.TrimSpace(jobType)) {
	case "regular", "":
		return model.EmployFullTime
	case "part-time":
		return model.EmployPartTime
	case "contract":
		return model.EmployContract
	case "intern":
		return model.EmployInternship
	default:
		return model.EmployFullTime
	}
}

func (a *Apple) resolveBaseURL() string {
	if a.baseURL != "" {
		return a.baseURL
	}
	return defaultBaseURL
}

// Compile-time interface checks.
var (
	_ types.Adapter     = (*Apple)(nil)
	_ types.URLProvider = (*Apple)(nil)
)
