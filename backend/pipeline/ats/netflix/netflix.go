package netflix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/pipeline/ats/types"
)

const (
	defaultBaseURL  = "https://explore.jobs.netflix.net"
	defaultDomain   = "netflix.com"
	defaultPageSize = 10

	// userAgent mimics a real browser to avoid CloudFront 403s on Go-http-client UA.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Netflix implements types.Adapter and types.ContentFetcher by calling the
// Eightfold AI SmartApply API that powers explore.jobs.netflix.net.
//
// FetchJobs paginates through /api/apply/v2/jobs to collect all listings.
// FetchJobContent hits /api/apply/v2/jobs/{id} for full job details.
type Netflix struct {
	// baseURL overrides the Eightfold host — used in tests.
	baseURL  string
	domain   string
	pageSize int
}

// New creates a Netflix adapter with default production settings.
func New() *Netflix {
	return &Netflix{
		baseURL:  defaultBaseURL,
		domain:   defaultDomain,
		pageSize: defaultPageSize,
	}
}

// CareersURL returns the public Netflix careers page URL.
func (n *Netflix) CareersURL(_ string) string {
	return "https://explore.jobs.netflix.net/careers"
}

// ── Eightfold API response types ────────────────────────────────────────────

// listResponse is the top-level JSON from /api/apply/v2/jobs.
type listResponse struct {
	Positions []position `json:"positions"`
	Count     int        `json:"count"`
}

// position mirrors a single entry in the Eightfold positions array.
// id and timestamps are numbers in the real API — not strings.
type position struct {
	ID                  int64           `json:"id"`
	Name                string          `json:"name"`
	PostingName         string          `json:"posting_name"`
	Location            string          `json:"location"`
	Locations           []string        `json:"locations"`
	Hot                 int             `json:"hot"`
	Department          string          `json:"department"`
	BusinessUnit        string          `json:"business_unit"`
	TUpdate             int64           `json:"t_update"`
	TCreate             int64           `json:"t_create"`
	AtsJobID            string          `json:"ats_job_id"`
	DisplayJobID        string          `json:"display_job_id"`
	Type                string          `json:"type"`
	IDLocale            string          `json:"id_locale"`
	Locale              string          `json:"locale"`
	JobDescription      string          `json:"job_description"`
	Stars               float64         `json:"stars"`
	LocationFlexibility string          `json:"location_flexibility"`
	WorkLocationOption  string          `json:"work_location_option"`
	CanonicalURL        string          `json:"canonicalPositionUrl"`
	IsPrivate           bool            `json:"isPrivate"`
	MedallionProgram    json.RawMessage `json:"medallionProgram"`
}

// positionDetail is the JSON returned by /api/apply/v2/jobs/{id}.
// The response is flat — the position fields are at the top level (no wrapper).
// Includes all list fields plus a few detail-only fields.
type positionDetail struct {
	ID                  int64           `json:"id"`
	Name                string          `json:"name"`
	PostingName         string          `json:"posting_name"`
	Location            string          `json:"location"`
	Locations           []string        `json:"locations"`
	Hot                 int             `json:"hot"`
	Department          string          `json:"department"`
	BusinessUnit        string          `json:"business_unit"`
	TUpdate             int64           `json:"t_update"`
	TCreate             int64           `json:"t_create"`
	AtsJobID            string          `json:"ats_job_id"`
	DisplayJobID        string          `json:"display_job_id"`
	Type                string          `json:"type"`
	IDLocale            string          `json:"id_locale"`
	Locale              string          `json:"locale"`
	JobDescription      string          `json:"job_description"`
	Stars               float64         `json:"stars"`
	LocationFlexibility string          `json:"location_flexibility"`
	WorkLocationOption  string          `json:"work_location_option"`
	CanonicalURL        string          `json:"canonicalPositionUrl"`
	IsPrivate           bool            `json:"isPrivate"`
	MedallionProgram    json.RawMessage `json:"medallionProgram"`
	// Detail-only fields
	PreApplyInfoBanner     json.RawMessage `json:"preApplyInfoBanner"`
	CustomJD               json.RawMessage `json:"custom_JD"`
	HideUploadResumeOption bool            `json:"hideUploadResumeOption"`
	URLs                   json.RawMessage `json:"urls"`
}

// ── Adapter implementation (FetchJobs) ──────────────────────────────────────

// FetchJobs paginates through the Eightfold list API and returns all Netflix
// job listings as thin items (FullContent is nil). companyID is unused since
// this adapter always targets explore.jobs.netflix.net.
func (n *Netflix) FetchJobs(ctx context.Context, _ string) (types.FetchJobsResponse, error) {
	var allItems []types.FetchJobsItem
	start := 0

	for {
		url := n.listURL(start)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("netflix: build request: %w", err)
		}
		req.Header.Set("User-Agent", userAgent)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("netflix: fetch page start=%d: %w", start, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return types.FetchJobsResponse{}, fmt.Errorf("netflix: unexpected status %d at start=%d", resp.StatusCode, start)
		}

		var page listResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("netflix: decode page start=%d: %w", start, err)
		}

		for _, p := range page.Positions {
			allItems = append(allItems, positionToItem(p))
		}

		// No more pages when we got fewer results than page size or no results
		if len(page.Positions) < n.pageSize {
			break
		}
		start += n.pageSize
	}

	return types.FetchJobsResponse{Jobs: allItems}, nil
}

// ── ContentFetcher implementation (FetchJobContent) ─────────────────────────

// FetchJobContent fetches the full job description from the Eightfold detail
// API. The job ID is extracted from the RawJob.ID field (strip "netflix-" prefix).
// The detail endpoint returns the position object flat at the top level.
func (n *Netflix) FetchJobContent(ctx context.Context, job types.RawJob) (types.FetchJobContentResponse, error) {
	eightfoldID := stripPrefix(job.ID)
	if eightfoldID == "" {
		return types.FetchJobContentResponse{}, fmt.Errorf("netflix: empty job ID after stripping prefix from %q", job.ID)
	}

	url := n.detailURL(eightfoldID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("netflix: build detail request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("netflix: fetch detail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.FetchJobContentResponse{}, fmt.Errorf("netflix: detail status %d for job %s", resp.StatusCode, eightfoldID)
	}

	// Detail response is a flat position object — no {"position": ...} wrapper.
	var detail positionDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("netflix: decode detail: %w", err)
	}

	return detailToContent(detail), nil
}

// ── URL builders ────────────────────────────────────────────────────────────

func (n *Netflix) listURL(start int) string {
	base := n.resolveBaseURL()
	return fmt.Sprintf("%s/api/apply/v2/jobs?domain=%s&hl=en&start=%d&num=%d",
		base, n.resolveDomain(), start, n.pageSize)
}

func (n *Netflix) detailURL(id string) string {
	base := n.resolveBaseURL()
	return fmt.Sprintf("%s/api/apply/v2/jobs/%s?domain=%s", base, id, n.resolveDomain())
}

func (n *Netflix) resolveBaseURL() string {
	if n.baseURL != "" {
		return n.baseURL
	}
	return defaultBaseURL
}

func (n *Netflix) resolveDomain() string {
	if n.domain != "" {
		return n.domain
	}
	return defaultDomain
}

// ── Converters ──────────────────────────────────────────────────────────────

func positionToItem(p position) types.FetchJobsItem {
	loc := p.Location
	if loc == "" && len(p.Locations) > 0 {
		loc = strings.Join(p.Locations, "; ")
	}

	jobURL := p.CanonicalURL
	if jobURL == "" {
		jobURL = fmt.Sprintf("%s/careers/job/%d", defaultBaseURL, p.ID)
	}

	return types.FetchJobsItem{
		ID:             fmt.Sprintf("%s-%d", model.AdapterNetflix, p.ID),
		Title:          p.Name,
		URL:            jobURL,
		Location:       loc,
		EmploymentType: normalizeWorkLocation(p.WorkLocationOption),
		FullContent:    nil, // thin adapter — detail fetched later
	}
}

func detailToContent(p positionDetail) types.FetchJobContentResponse {
	loc := p.Location
	if loc == "" && len(p.Locations) > 0 {
		loc = strings.Join(p.Locations, "; ")
	}

	jobURL := p.CanonicalURL
	if jobURL == "" {
		jobURL = fmt.Sprintf("%s/careers/job/%d", defaultBaseURL, p.ID)
	}

	return types.FetchJobContentResponse{
		ID:            fmt.Sprintf("%s-%d", model.AdapterNetflix, p.ID),
		Title:         p.Name,
		Location:      loc,
		URL:           jobURL,
		Description:   types.StripHTML(p.JobDescription),
		Department:    p.Department,
		WorkplaceType: p.WorkLocationOption,
		PublishedDate: unixToRFC3339(p.TCreate),
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// stripPrefix removes the "netflix-" prefix from a RawJob.ID to get the
// Eightfold numeric position ID as a string.
func stripPrefix(id string) string {
	const prefix = model.AdapterNetflix + "-"
	if strings.HasPrefix(id, prefix) {
		return id[len(prefix):]
	}
	return id
}

// unixToRFC3339 converts a Unix timestamp (seconds) to an RFC 3339 string.
// Returns empty string if ts is zero.
func unixToRFC3339(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

// normalizeWorkLocation maps Eightfold work_location_option values to a
// human-readable string. This is NOT the EmployType (full_time, contract) —
// Eightfold doesn't expose that in listings. We store the work location
// for informational purposes.
func normalizeWorkLocation(opt string) string {
	switch strings.ToLower(opt) {
	case "remote":
		return model.EmployFullTime // default assumption for Netflix roles
	case "onsite", "on-site":
		return model.EmployFullTime
	case "hybrid":
		return model.EmployFullTime
	default:
		return model.EmployFullTime
	}
}

// Compile-time interface checks.
var (
	_ types.Adapter        = (*Netflix)(nil)
	_ types.ContentFetcher = (*Netflix)(nil)
	_ types.URLProvider    = (*Netflix)(nil)
)
