package uber

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

const (
	defaultBaseURL = "https://www.uber.com"

	// pathFilterOptions returns the list of departments (and other filter options).
	pathFilterOptions = "/api/loadFilterOptions?localeCode=en"

	// pathSearchJobs is the main job listing endpoint; paginated per department.
	pathSearchJobs = "/api/loadSearchJobsResults?localeCode=en"

	// jobURLTemplate builds the public job-detail URL from a numeric job ID.
	jobURLTemplate = "%s/careers/list/%d"

	// csrfHeaderValue is the static value for the required x-csrf-token header.
	// Uber validates header *presence*, not value, using a custom-header CSRF
	// strategy: browsers can't set arbitrary headers cross-origin.
	csrfHeaderValue = "x"

	// userAgent mimics a real browser — Uber's CDN blocks Go-http-client UA with 403.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Uber implements types.Adapter by calling Uber's internal Next.js API.
//
// FetchJobs:
//  1. Calls /api/loadFilterOptions to get all current department names.
//  2. Calls /api/loadSearchJobsResults once per department (each < 1000 jobs).
//  3. Returns all jobs de-duplicated by ID with FullContent populated (fat adapter).
//
// No ContentFetcher is implemented — the list API returns the full Markdown
// description for every job, so a separate detail fetch is not needed.
type Uber struct {
	// baseURL overrides the Uber host — used in tests.
	baseURL string
}

// New creates an Uber adapter with default production settings.
func New() *Uber {
	return &Uber{baseURL: defaultBaseURL}
}

// CareersURL returns the public Uber careers page URL.
func (u *Uber) CareersURL(_ string) string {
	return "https://www.uber.com/us/en/careers/list/"
}

// ── Eightfold API response types ────────────────────────────────────────────

// filterOptionsResponse is the top-level JSON from /api/loadFilterOptions.
type filterOptionsResponse struct {
	Data filterOptionsData `json:"data"`
}

type filterOptionsData struct {
	// Department is a map of department name → list of sub-team names.
	Department map[string][]string `json:"department"`
	// Location is also available but not used for pagination.
	Location json.RawMessage `json:"location"`
}

// searchJobsResponse is the top-level JSON from /api/loadSearchJobsResults.
type searchJobsResponse struct {
	Status string         `json:"status"`
	Data   searchJobsData `json:"data"`
}

type searchJobsData struct {
	Results      []uberJob `json:"results"`
	TotalResults uberTotal `json:"totalResults"`
}

// uberTotal mirrors the protobuf Int64Value Uber uses for total counts.
// High is always 0 for realistic job counts; only Low is needed.
type uberTotal struct {
	Low      int  `json:"low"`
	High     int  `json:"high"`
	Unsigned bool `json:"unsigned"`
}

// uberJob is a single job entry in the /api/loadSearchJobsResults response.
// Description is full Markdown — no separate detail fetch needed.
type uberJob struct {
	ID                 int             `json:"id"`
	Title              string          `json:"title"`
	Description        string          `json:"description"` // Full Markdown
	Department         string          `json:"department"`
	Type               string          `json:"type"`
	ProgramAndPlatform string          `json:"programAndPlatform"`
	Location           uberLocation    `json:"location"`
	Featured           bool            `json:"featured"`
	Level              string          `json:"level"`
	CreationDate       string          `json:"creationDate"` // ISO 8601
	OtherLevels        json.RawMessage `json:"otherLevels"`
	Team               string          `json:"team"`
	PortalID           string          `json:"portalID"`
	IsPipeline         bool            `json:"isPipeline"`
	StatusID           string          `json:"statusID"`
	StatusName         string          `json:"statusName"`
	UpdatedDate        string          `json:"updatedDate"` // ISO 8601
	UniqueSkills       string          `json:"uniqueSkills"`
	TimeType           string          `json:"timeType"` // "Full-Time", "Part-Time", etc.
	AllLocations       []uberLocation  `json:"allLocations"`
}

// uberLocation is the structured location object in Uber job responses.
type uberLocation struct {
	Country     string `json:"country"` // ISO alpha-3, e.g. "USA"
	Region      string `json:"region"`  // state/province, may be null
	City        string `json:"city"`
	CountryName string `json:"countryName"` // human-readable, e.g. "United States"
}

// ── Request body types ───────────────────────────────────────────────────────

type searchJobsRequest struct {
	Params searchJobsParams `json:"params"`
}

type searchJobsParams struct {
	Query      string   `json:"query"`
	Limit      int      `json:"limit"`
	Page       int      `json:"page"`
	Department []string `json:"department"`
}

// ── Adapter implementation ───────────────────────────────────────────────────

// FetchJobs returns all current Uber job listings by iterating departments.
// Each job's full description is included — FullContent is always set.
// companyID is ignored (Uber adapter targets a fixed host).
func (u *Uber) FetchJobs(ctx context.Context, _ string) (types.FetchJobsResponse, error) {
	departments, err := u.fetchDepartments(ctx)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("uber: fetch departments: %w", err)
	}

	// Collect unique jobs across all departments (each job belongs to exactly one).
	seen := make(map[int]struct{})
	var allItems []types.FetchJobsItem

	for _, dept := range departments {
		jobs, err := u.fetchJobsForDepartment(ctx, dept)
		if err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("uber: fetch jobs for department %q: %w", dept, err)
		}
		for _, job := range jobs {
			if _, dup := seen[job.ID]; dup {
				continue
			}
			seen[job.ID] = struct{}{}
			allItems = append(allItems, jobToItem(job))
		}
	}

	return types.FetchJobsResponse{Jobs: allItems}, nil
}

// fetchDepartments calls /api/loadFilterOptions and returns the department names.
func (u *Uber) fetchDepartments(ctx context.Context) ([]string, error) {
	url := u.resolveBaseURL() + pathFilterOptions

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString("{}"))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	u.setHeaders(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var result filterOptionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	departments := make([]string, 0, len(result.Data.Department))
	for dept := range result.Data.Department {
		departments = append(departments, dept)
	}
	return departments, nil
}

// fetchJobsForDepartment calls /api/loadSearchJobsResults filtered by one department.
func (u *Uber) fetchJobsForDepartment(ctx context.Context, dept string) ([]uberJob, error) {
	url := u.resolveBaseURL() + pathSearchJobs

	body := searchJobsRequest{
		Params: searchJobsParams{
			Query:      "",
			Limit:      1000,
			Page:       0,
			Department: []string{dept},
		},
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	u.setHeaders(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var result searchJobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return result.Data.Results, nil
}

// setHeaders applies required headers to every Uber API request.
func (u *Uber) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	// Uber uses a custom-header CSRF strategy: the header must be present.
	// The value is not validated server-side.
	req.Header.Set("x-csrf-token", csrfHeaderValue)
}

// ── Converters ───────────────────────────────────────────────────────────────

func jobToItem(j uberJob) types.FetchJobsItem {
	description := j.Description
	if j.Level != "" {
		// Append Uber's internal level so the LLM enricher can infer seniority.
		// Level "5" = L5 = Senior SWE; "6" = L6 = Staff; "5B" = between 5 and 6.
		description += fmt.Sprintf("\n\nUber Level: %s", j.Level)
	}

	return types.FetchJobsItem{
		ID:             fmt.Sprintf("%s-%d", companycommon.AdapterUber, j.ID),
		Title:          j.Title,
		URL:            fmt.Sprintf(jobURLTemplate, defaultBaseURL, j.ID),
		Location:       formatLocation(j.AllLocations, j.Location),
		EmploymentType: mapTimeType(j.TimeType),
		FullContent: &types.FullJobContent{
			Description:   description,
			Department:    j.Department,
			WorkplaceType: "", // not available in Uber API
			PublishedDate: j.CreationDate,
		},
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// formatLocation builds a human-readable location string.
// Uses AllLocations when multiple locations exist, falls back to the primary Location.
func formatLocation(all []uberLocation, primary uberLocation) string {
	if len(all) > 1 {
		parts := make([]string, 0, len(all))
		for _, loc := range all {
			if s := formatSingleLocation(loc); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "; ")
	}
	if len(all) == 1 {
		return formatSingleLocation(all[0])
	}
	return formatSingleLocation(primary)
}

// formatSingleLocation renders a uberLocation as "City, Region, CountryName",
// skipping empty segments.
func formatSingleLocation(loc uberLocation) string {
	parts := make([]string, 0, 3)
	if loc.City != "" {
		parts = append(parts, loc.City)
	}
	if loc.Region != "" {
		parts = append(parts, loc.Region)
	}
	if loc.CountryName != "" {
		parts = append(parts, loc.CountryName)
	}
	return strings.Join(parts, ", ")
}

// mapTimeType converts Uber's TimeType string to a model.EmployType constant.
func mapTimeType(tt string) string {
	switch strings.ToLower(strings.TrimSpace(tt)) {
	case "full-time":
		return jobcommon.EmployFullTime
	case "part-time":
		return jobcommon.EmployPartTime
	case "contract":
		return jobcommon.EmployContract
	case "internship":
		return jobcommon.EmployInternship
	default:
		return jobcommon.EmployFullTime
	}
}

func (u *Uber) resolveBaseURL() string {
	if u.baseURL != "" {
		return u.baseURL
	}
	return defaultBaseURL
}

// Compile-time interface checks.
var (
	_ types.Adapter     = (*Uber)(nil)
	_ types.URLProvider = (*Uber)(nil)
)
