package microsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/pipeline/ats/types"
)

const (
	defaultBaseURL  = "https://apply.careers.microsoft.com"
	defaultDomain   = "microsoft.com"
	defaultPageSize = 20

	// userAgent mimics a real browser to avoid 403s on Go-http-client UA.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

	// US location filter as required by the Microsoft API
	locationFilter = "United States, Multiple Locations, Multiple Locations"
)

// Microsoft implements types.Adapter, types.ContentFetcher, and types.URLProvider
// by calling the Microsoft careers API.
//
// FetchJobs paginates through /api/pcsx/search to collect all US job listings.
// FetchJobContent hits /api/pcsx/position_details for full job details.
type Microsoft struct {
	// baseURL overrides the Microsoft careers host — used in tests.
	baseURL  string
	domain   string
	pageSize int
}

// New creates a Microsoft adapter with default production settings.
func New() *Microsoft {
	return &Microsoft{
		baseURL:  defaultBaseURL,
		domain:   defaultDomain,
		pageSize: defaultPageSize,
	}
}

// CareersURL returns the public Microsoft careers page URL.
func (m *Microsoft) CareersURL(_ string) string {
	return "https://careers.microsoft.com/us/en/careers"
}

// ── Microsoft API response types ────────────────────────────────────────────

// searchResponse is the top-level JSON from /api/pcsx/search.
type searchResponse struct {
	Status  any                   `json:"status"`
	Error   string                `json:"error"`
	Data    searchResponseData    `json:"data"`
}

type searchResponseData struct {
	Count      int         `json:"count"`
	Positions  []position  `json:"positions"`
	FilterDef  interface{} `json:"filterDef"`
	SortBy     interface{} `json:"sortBy"`
	AppliedFilters interface{} `json:"appliedFilters"`
}

// position mirrors a single entry in the Microsoft positions array.
type position struct {
	ID                   int64             `json:"id"`
	DisplayJobID         string            `json:"displayJobId"`
	Name                 string            `json:"name"`
	Locations            []string          `json:"locations"`
	StandardizedLocations map[string][]string `json:"standardizedLocations"`
	PostedTs             int64             `json:"postedTs"`
	Department           string            `json:"department"`
	WorkLocationOption   string            `json:"workLocationOption"`
	AtsJobID             string            `json:"atsJobId"`
	PositionURL          string            `json:"positionUrl"`
}

// positionDetail is the JSON returned by /api/pcsx/position_details.
// Includes all list fields plus additional detail-only fields.
type positionDetail struct {
	ID                          int64               `json:"id"`
	DisplayJobID                string              `json:"displayJobId"`
	Name                        string              `json:"name"`
	Locations                   []string            `json:"locations"`
	PostedTs                    int64               `json:"postedTs"`
	Department                  string              `json:"department"`
	WorkLocationOption          string              `json:"workLocationOption"`
	JobDescription              string              `json:"jobDescription"`
	Location                    string              `json:"location"`
	EfcustomTextWorkSite        []string            `json:"efcustomTextWorkSite"`
	EfcustomTextEmploymentType  []string            `json:"efcustomTextEmploymentType"`
	EfcustomTextRoletype        []string            `json:"efcustomTextRoletype"`
	EfcustomTextRequiredTravel  []string            `json:"efcustomTextRequiredTravel"`
	PublicURL                   string              `json:"publicUrl"`
	AtsJobID                    string              `json:"atsJobId"`
	PositionURL                 string              `json:"positionUrl"`
}

// ── Adapter implementation (FetchJobs) ──────────────────────────────────────

// FetchJobs paginates through the Microsoft search API and returns all US
// job listings as thin items (FullContent is nil). companyID is unused since
// this adapter always targets careers.microsoft.com.
func (m *Microsoft) FetchJobs(ctx context.Context, _ string) (types.FetchJobsResponse, error) {
	var allItems []types.FetchJobsItem
	start := 0
	pageSize := m.resolvePageSize()

	for {
		url := m.searchURL(start)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("microsoft: build request: %w", err)
		}
		req.Header.Set("User-Agent", userAgent)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("microsoft: fetch page start=%d: %w", start, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return types.FetchJobsResponse{}, fmt.Errorf("microsoft: unexpected status %d at start=%d", resp.StatusCode, start)
		}

		var page searchResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("microsoft: decode page start=%d: %w", start, err)
		}

		for _, p := range page.Data.Positions {
			allItems = append(allItems, positionToItem(p))
		}

		// No more pages when we got fewer results than page size
		if len(page.Data.Positions) < pageSize {
			break
		}
		start += pageSize
	}

	return types.FetchJobsResponse{Jobs: allItems}, nil
}

// ── ContentFetcher implementation (FetchJobContent) ─────────────────────────

// FetchJobContent fetches the full job description from the Microsoft detail
// API. The int64 position ID is extracted from the RawJob.URL's last path segment.
func (m *Microsoft) FetchJobContent(ctx context.Context, job types.RawJob) (types.FetchJobContentResponse, error) {
	positionID := extractPositionID(job.URL)
	if positionID == 0 {
		return types.FetchJobContentResponse{}, fmt.Errorf("microsoft: could not extract position ID from URL %q", job.URL)
	}

	url := m.positionDetailsURL(positionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("microsoft: build detail request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("microsoft: fetch detail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.FetchJobContentResponse{}, fmt.Errorf("microsoft: detail status %d for position %d", resp.StatusCode, positionID)
	}

	var detail positionDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("microsoft: decode detail: %w", err)
	}

	return detailToContent(detail), nil
}

// ── URL builders ────────────────────────────────────────────────────────────

func (m *Microsoft) resolveBaseURL() string {
	if m.baseURL != "" {
		return m.baseURL
	}
	return defaultBaseURL
}

func (m *Microsoft) resolveDomain() string {
	if m.domain != "" {
		return m.domain
	}
	return defaultDomain
}

func (m *Microsoft) resolvePageSize() int {
	if m.pageSize > 0 {
		return m.pageSize
	}
	return defaultPageSize
}

func (m *Microsoft) searchURL(start int) string {
	base := m.resolveBaseURL()
	domain := m.resolveDomain()
	// URL encode the location filter: "United States, Multiple Locations, Multiple Locations"
	// becomes "United+States%2C+Multiple+Locations%2C+Multiple+Locations"
	location := "United+States%2C+Multiple+Locations%2C+Multiple+Locations"
	return fmt.Sprintf("%s/api/pcsx/search?domain=%s&query=&location=%s&start=%d&num=%d",
		base, domain, location, start, m.resolvePageSize())
}

func (m *Microsoft) positionDetailsURL(positionID int64) string {
	base := m.resolveBaseURL()
	domain := m.resolveDomain()
	return fmt.Sprintf("%s/api/pcsx/position_details?position_id=%d&domain=%s&hl=en",
		base, positionID, domain)
}

// ── Converters ──────────────────────────────────────────────────────────────

func positionToItem(p position) types.FetchJobsItem {
	loc := ""
	if len(p.Locations) > 0 {
		loc = strings.Join(p.Locations, "; ")
	}

	// Use positionUrl from the listing; if empty, construct it
	jobURL := p.PositionURL
	if jobURL == "" {
		jobURL = fmt.Sprintf("https://careers.microsoft.com/v2/global/en/job/%s", p.DisplayJobID)
	}

	// Store the int64 ID in the URL for later extraction by FetchJobContent
	// This allows us to have the int64 ID available when we need to call position_details
	internalURL := fmt.Sprintf("https://apply.careers.microsoft.com/careers/job/%d", p.ID)

	return types.FetchJobsItem{
		ID:             fmt.Sprintf("%s-%s", model.AdapterMicrosoft, p.DisplayJobID),
		Title:          p.Name,
		URL:            internalURL, // Store internal URL for ID extraction
		Location:       loc,
		EmploymentType: model.EmployFullTime, // Default — detailed type comes from FetchJobContent
		FullContent:    nil,                  // thin adapter — detail fetched later
	}
}

func detailToContent(p positionDetail) types.FetchJobContentResponse {
	loc := p.Location
	if loc == "" && len(p.Locations) > 0 {
		loc = strings.Join(p.Locations, "; ")
	}

	// Use positionUrl from the detail response
	jobURL := p.PositionURL
	if jobURL == "" {
		jobURL = fmt.Sprintf("https://careers.microsoft.com/v2/global/en/job/%s", p.DisplayJobID)
	}

	return types.FetchJobContentResponse{
		ID:            fmt.Sprintf("%s-%s", model.AdapterMicrosoft, p.DisplayJobID),
		Title:         p.Name,
		Location:      loc,
		URL:           jobURL,
		Description:   types.StripHTML(p.JobDescription),
		Department:    p.Department,
		WorkplaceType: p.WorkLocationOption,
		EmploymentType: mapEmploymentType(p.EfcustomTextEmploymentType),
		PublishedDate: unixToRFC3339(p.PostedTs),
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// extractPositionID extracts the int64 position ID from a URL like
// "https://apply.careers.microsoft.com/careers/job/1812345678"
func extractPositionID(url string) int64 {
	parts := strings.Split(strings.TrimRight(url, "/"), "/")
	if len(parts) > 0 {
		idStr := parts[len(parts)-1]
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
			return id
		}
	}
	return 0
}

// mapEmploymentType maps Microsoft's efcustomTextEmploymentType values to
// model employment constants.
func mapEmploymentType(empTypes []string) string {
	if len(empTypes) == 0 {
		return model.EmployFullTime // default
	}

	empType := empTypes[0]
	switch strings.TrimSpace(empType) {
	case "Full-Time":
		return model.EmployFullTime
	case "Part-Time":
		return model.EmployPartTime
	case "Contract":
		return model.EmployContract
	case "Intern", "Internship":
		return model.EmployInternship
	default:
		return model.EmployFullTime
	}
}

// unixToRFC3339 converts a Unix timestamp (seconds) to an RFC 3339 string.
// Returns empty string if ts is zero.
func unixToRFC3339(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

// Compile-time interface checks.
var (
	_ types.Adapter        = (*Microsoft)(nil)
	_ types.ContentFetcher = (*Microsoft)(nil)
	_ types.URLProvider    = (*Microsoft)(nil)
)
