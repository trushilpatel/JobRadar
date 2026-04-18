package rippling

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
	defaultCareersBaseURL = "https://www.rippling.com"
	defaultATSBaseURL     = "https://ats.rippling.com"

	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Rippling implements types.Adapter and types.ContentFetcher by fetching
// Next.js SSG/SSR pages from Rippling's careers domain and parsing embedded JSON.
//
// FetchJobs fetches https://www.rippling.com/careers/open-roles (Next.js SSG page).
// All jobs (~716 total) are embedded in a single <script id="__NEXT_DATA__"> tag.
// Only US jobs are returned (country code filter).
//
// FetchJobContent fetches https://ats.rippling.com/rippling/jobs/{id} (Next.js SSR page)
// and extracts the full job details from __NEXT_DATA__.
type Rippling struct {
	// careersBaseURL overrides the careers domain — used in tests.
	careersBaseURL string
	// atsBaseURL overrides the ATS domain — used in tests.
	atsBaseURL string
}

// New creates a Rippling adapter with default production settings.
func New() *Rippling {
	return &Rippling{
		careersBaseURL: defaultCareersBaseURL,
		atsBaseURL:     defaultATSBaseURL,
	}
}

// CareersURL returns the public Rippling careers page URL.
func (r *Rippling) CareersURL(_ string) string {
	return fmt.Sprintf("%s/careers/open-roles", r.resolveCareersBaseURL())
}

// ── Next.js data response types ────────────────────────────────────────────

// nextDataResponse wraps the top-level JSON from __NEXT_DATA__.
type nextDataResponse struct {
	Props struct {
		PageProps struct {
			// FetchJobs endpoint
			Jobs struct {
				Items []jobListingItem `json:"items"`
			} `json:"jobs"`
			// FetchJobContent endpoint
			APIData jobDetailData `json:"apiData"`
		} `json:"pageProps"`
	} `json:"props"`
}

// jobListingItem represents one job in the listing page.
type jobListingItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	Department struct {
		Name string `json:"name"`
	} `json:"department"`
	Locations []jobLocation `json:"locations"`
	Language  string        `json:"language"`
}

// jobLocation represents a job location.
type jobLocation struct {
	Name          string `json:"name"`
	Country       string `json:"country"`
	CountryCode   string `json:"countryCode"`
	State         string `json:"state"`
	StateCode     string `json:"stateCode"`
	City          string `json:"city"`
	WorkplaceType string `json:"workplaceType"`
}

// jobDetailData represents the apiData object from the detail page.
// Uses flexible field names to handle API variations.
type jobDetailData struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	DescriptionHTML string `json:"descriptionHtml"`
	Description     string `json:"description"`
	JobDescription  string `json:"jobDescription"`
	Body            string `json:"body"`
	Compensation    string `json:"compensation"`
	CompensationRng string `json:"compensationRange"`
	Salary          string `json:"salary"`
	WorkplaceType   string `json:"workplaceType"`
	WorkplaceTyp    string `json:"workplace_type"`
	Department      struct {
		Name string `json:"name"`
	} `json:"department"`
	DepartmentName string          `json:"departmentName"`
	Locations      []jobLocation   `json:"locations"`
	CreatedAt      string          `json:"createdAt"`
	Extra          json.RawMessage `json:"-"` // capture any unknown fields
}

// ── Adapter implementation (FetchJobs) ──────────────────────────────────────

// FetchJobs fetches the careers listing page and returns all US jobs as thin items
// (FullContent is nil). companyID is unused since this adapter always targets
// the Rippling careers page.
func (r *Rippling) FetchJobs(ctx context.Context, _ string) (types.FetchJobsResponse, error) {
	url := r.CareersURL("")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("rippling: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("rippling: fetch careers page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.FetchJobsResponse{}, fmt.Errorf("rippling: unexpected status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("rippling: read response body: %w", err)
	}
	body := string(bodyBytes)

	// Extract __NEXT_DATA__ from HTML
	nextDataStr, err := extractNextData(body)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("rippling: extract next data: %w", err)
	}

	var data nextDataResponse
	if err := json.Unmarshal([]byte(nextDataStr), &data); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("rippling: parse next data: %w", err)
	}

	// Filter for US jobs and convert to pipeline types
	var items []types.FetchJobsItem
	seen := make(map[string]bool)

	for _, job := range data.Props.PageProps.Jobs.Items {
		// Filter: only US jobs
		if !hasUSLocation(job.Locations) {
			continue
		}

		itemID := fmt.Sprintf("%s-%s", model.AdapterRippling, job.ID)
		if seen[itemID] {
			continue // dedup
		}
		seen[itemID] = true

		// Build location string from US locations only
		location := formatUSLocations(job.Locations)

		items = append(items, types.FetchJobsItem{
			ID:             itemID,
			Title:          job.Name,
			URL:            job.URL,
			Location:       location,
			EmploymentType: model.EmployFullTime, // default; Rippling doesn't expose this in listing
			FullContent:    nil,                  // thin adapter — detail fetched later
		})
	}

	return types.FetchJobsResponse{Jobs: items}, nil
}

// ── ContentFetcher implementation (FetchJobContent) ─────────────────────────

// FetchJobContent fetches the full job details from the ATS detail page.
// The job ID is extracted from RawJob.ID (strip "rippling-" prefix).
func (r *Rippling) FetchJobContent(ctx context.Context, job types.RawJob) (types.FetchJobContentResponse, error) {
	jobID := stripPrefix(job.ID)
	if jobID == "" {
		return types.FetchJobContentResponse{}, fmt.Errorf("rippling: empty job ID after stripping prefix from %q", job.ID)
	}

	url := fmt.Sprintf("%s/rippling/jobs/%s", r.resolveATSBaseURL(), jobID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("rippling: build detail request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("rippling: fetch detail page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.FetchJobContentResponse{}, fmt.Errorf("rippling: detail status %d for job %s", resp.StatusCode, jobID)
	}

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("rippling: read detail response: %w", err)
	}
	body := string(bodyBytes)

	// Extract __NEXT_DATA__
	nextDataStr, err := extractNextData(body)
	if err != nil {
		// Fallback: try to strip HTML from the raw body
		return types.FetchJobContentResponse{
			ID:          job.ID,
			Title:       "",
			Location:    "",
			URL:         url,
			Description: types.StripHTML(body),
		}, nil
	}

	var data nextDataResponse
	if err := json.Unmarshal([]byte(nextDataStr), &data); err != nil {
		// Fallback: return what we have
		return types.FetchJobContentResponse{
			ID:          job.ID,
			Title:       "",
			Location:    "",
			URL:         url,
			Description: types.StripHTML(body),
		}, nil
	}

	return detailToContent(job.ID, data.Props.PageProps.APIData), nil
}

// ── URL builders ────────────────────────────────────────────────────────────

func (r *Rippling) resolveCareersBaseURL() string {
	if r.careersBaseURL != "" {
		return r.careersBaseURL
	}
	return defaultCareersBaseURL
}

func (r *Rippling) resolveATSBaseURL() string {
	if r.atsBaseURL != "" {
		return r.atsBaseURL
	}
	return defaultATSBaseURL
}

// ── Converters ──────────────────────────────────────────────────────────────

// detailToContent converts a jobDetailData to FetchJobContentResponse.
func detailToContent(jobID string, detail jobDetailData) types.FetchJobContentResponse {
	// Find description from multiple possible field names
	description := detail.DescriptionHTML
	if description == "" {
		description = detail.Description
	}
	if description == "" {
		description = detail.JobDescription
	}
	if description == "" {
		description = detail.Body
	}
	description = types.StripHTML(description)

	// Find compensation from multiple possible field names
	compensation := detail.Compensation
	if compensation == "" {
		compensation = detail.CompensationRng
	}
	if compensation == "" {
		compensation = detail.Salary
	}

	// Find workplace type
	workplaceType := detail.WorkplaceType
	if workplaceType == "" {
		workplaceType = detail.WorkplaceTyp
	}

	// Find department
	department := detail.Department.Name
	if department == "" {
		department = detail.DepartmentName
	}

	// Build location string from all locations
	location := formatAllLocations(detail.Locations)

	return types.FetchJobContentResponse{
		ID:            jobID,
		Title:         detail.Title,
		Location:      location,
		URL:           "",
		Description:   description,
		Department:    department,
		Compensation:  compensation,
		WorkplaceType: workplaceType,
		PublishedDate: detail.CreatedAt,
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// extractNextData extracts the JSON string from <script id="__NEXT_DATA__">...</script>.
func extractNextData(html string) (string, error) {
	// Pattern: <script id="__NEXT_DATA__" type="application/json">{...}</script>
	// Using a simpler approach: find the start and end markers
	start := strings.Index(html, `<script id="__NEXT_DATA__"`)
	if start == -1 {
		return "", fmt.Errorf("__NEXT_DATA__ script not found")
	}

	// Find the opening > after the script tag
	openIdx := strings.Index(html[start:], ">")
	if openIdx == -1 {
		return "", fmt.Errorf("malformed __NEXT_DATA__ script tag")
	}
	jsonStart := start + openIdx + 1

	// Find the closing </script> tag
	endIdx := strings.Index(html[jsonStart:], "</script>")
	if endIdx == -1 {
		return "", fmt.Errorf("closing </script> not found")
	}

	return strings.TrimSpace(html[jsonStart : jsonStart+endIdx]), nil
}

// hasUSLocation checks if any location in the list has countryCode == "US".
func hasUSLocation(locations []jobLocation) bool {
	for _, loc := range locations {
		if loc.CountryCode == "US" {
			return true
		}
	}
	return false
}

// formatUSLocations builds a location string from US locations only.
// Format: "City, StateCode" joined by "; ".
func formatUSLocations(locations []jobLocation) string {
	var parts []string
	for _, loc := range locations {
		if loc.CountryCode == "US" {
			parts = append(parts, fmt.Sprintf("%s, %s", loc.City, loc.StateCode))
		}
	}
	return strings.Join(parts, "; ")
}

// formatAllLocations builds a location string from all locations.
// Format: "City, StateCode, Country" joined by "; ".
func formatAllLocations(locations []jobLocation) string {
	var parts []string
	for _, loc := range locations {
		if loc.City != "" && loc.StateCode != "" {
			parts = append(parts, fmt.Sprintf("%s, %s", loc.City, loc.StateCode))
		} else if loc.City != "" {
			parts = append(parts, loc.City)
		} else if loc.Name != "" {
			parts = append(parts, loc.Name)
		}
	}
	return strings.Join(parts, "; ")
}

// stripPrefix removes the "rippling-" prefix from a RawJob.ID to get the UUID.
func stripPrefix(id string) string {
	const prefix = model.AdapterRippling + "-"
	if strings.HasPrefix(id, prefix) {
		return id[len(prefix):]
	}
	return id
}

// Compile-time interface checks.
var (
	_ types.Adapter        = (*Rippling)(nil)
	_ types.ContentFetcher = (*Rippling)(nil)
	_ types.URLProvider    = (*Rippling)(nil)
)
