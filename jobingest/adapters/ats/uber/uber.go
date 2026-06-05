package uber

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	uberapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/uber/api"
	"github.com/trushilpatel/jobradar/jobingest/common/logger"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

const (
	defaultBaseURL = "https://www.uber.com"

	// pathFilterOptions returns the list of departments (and other filter options).
	pathFilterOptions = "/api/loadFilterOptions?localeCode=en"

	// pathSearchJobs is the main job listing endpoint; paginated per department.
	pathSearchJobs = "/api/loadSearchJobsResults?localeCode=en"

	// csrfHeaderValue is the static value required for the x-csrf-token header.
	// Uber validates header presence, not value, using a custom-header CSRF strategy:
	// browsers cannot set arbitrary headers cross-origin.
	csrfHeaderValue = "x"

	// userAgent mimics a real browser — Uber's CDN blocks the Go-http-client UA with 403.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Uber calls Uber's internal Next.js API to retrieve all job listings.
//
// FetchJobListing:
//  1. Calls /api/loadFilterOptions to get all current department names.
//  2. Calls /api/loadSearchJobsResults once per department (each < 1000 jobs).
//  3. Returns all jobs de-duplicated by numeric ID with JobContent populated.
//
// This is a fat adapter — JobContent is always set; no PostFetcher is implemented.
// companyID is ignored: the adapter targets a fixed Uber host.
type Uber struct {
	baseURL    string // overridden in tests; empty → defaultBaseURL
	log        logger.Logger
	httpClient *http.Client
}

// New constructs an Uber adapter with default production settings.
func New(log logger.Logger, httpClient *http.Client) *Uber {
	return &Uber{
		log:        log.With(logger.Str(logger.FieldAdapter, "uber")),
		httpClient: httpClient,
	}
}

// ── ListingFetcher ────────────────────────────────────────────────────────────

// FetchJobListing returns all current Uber job listings by iterating departments.
// Each job's full description is included — JobContent is always set.
// companyID is ignored (Uber adapter targets a fixed host).
func (u *Uber) FetchJobListing(ctx context.Context, _ string) (core.JobListing, error) {
	log := u.log.WithContext(ctx)
	start := time.Now()

	departments, err := u.fetchDepartments(ctx)
	if err != nil {
		log.Error("fetch departments failed", err)
		return core.JobListing{}, fmt.Errorf("uber: fetch departments: %w", err)
	}

	// Collect unique jobs across departments. Each job belongs to exactly one
	// department; de-dup by numeric ID is a safety net.
	seen := make(map[int]struct{})
	var summaries []core.JobSummary

	for _, dept := range departments {
		jobs, err := u.fetchJobsForDepartment(ctx, dept)
		if err != nil {
			log.Error("fetch jobs for department failed", err, logger.Str("department", dept))
			return core.JobListing{}, fmt.Errorf("uber: fetch jobs for department %q: %w", dept, err)
		}
		for _, job := range jobs {
			if _, dup := seen[job.ID]; dup {
				continue
			}
			seen[job.ID] = struct{}{}
			summaries = append(summaries, mapJobSummary(job, u.resolveBaseURL()))
		}
	}

	log.Info("listing fetched",
		logger.Int(logger.FieldJobCount, len(summaries)),
		logger.Dur(logger.FieldDuration, time.Since(start)),
	)
	return core.JobListing{Jobs: summaries}, nil
}

// ── Private helpers ───────────────────────────────────────────────────────────

// fetchDepartments calls /api/loadFilterOptions and returns the sorted department names.
func (u *Uber) fetchDepartments(ctx context.Context) ([]string, error) {
	endpoint := u.resolveBaseURL() + pathFilterOptions

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString("{}"))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	u.setHeaders(req)

	var result uberapi.FilterOptionsResponse
	if err := u.doJSON(req, &result); err != nil {
		return nil, fmt.Errorf("filter options: %w", err)
	}

	departments := make([]string, 0, len(result.Data.Department))
	for dept := range result.Data.Department {
		departments = append(departments, dept)
	}
	sort.Strings(departments) // deterministic order for predictable logs + tests
	return departments, nil
}

// fetchJobsForDepartment calls /api/loadSearchJobsResults filtered to one department.
func (u *Uber) fetchJobsForDepartment(ctx context.Context, dept string) ([]uberapi.UberJob, error) {
	endpoint := u.resolveBaseURL() + pathSearchJobs

	body := uberapi.SearchJobsRequest{
		Params: uberapi.SearchJobsParams{
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	u.setHeaders(req)

	var result uberapi.SearchJobsResponse
	if err := u.doJSON(req, &result); err != nil {
		return nil, fmt.Errorf("search jobs: %w", err)
	}

	return result.Data.Results, nil
}

// setHeaders applies required headers to every Uber API request.
func (u *Uber) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	// Custom-header CSRF strategy: header must be present; value is not validated.
	req.Header.Set("x-csrf-token", csrfHeaderValue)
}

// doJSON executes req, checks the status, and JSON-decodes the response body into dst.
func (u *Uber) doJSON(req *http.Request, dst any) error {
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

func (u *Uber) resolveBaseURL() string {
	if u.baseURL != "" {
		return u.baseURL
	}
	return defaultBaseURL
}

// ── Interface check ───────────────────────────────────────────────────────────

var _ core.ListingFetcher = (*Uber)(nil)
