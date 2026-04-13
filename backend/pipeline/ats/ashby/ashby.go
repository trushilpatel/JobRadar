package ashby

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/trushilpatel/jobradar/pipeline/ats/types"
	"github.com/trushilpatel/jobradar/model"
)

// Documentation
// https://developers.ashbyhq.com/reference/jobinfo
// https://developers.ashbyhq.com/reference/joblist

type Ashby struct {
	// baseURL overrides the public Ashby GraphQL endpoint — used in tests to point at a fake server.
	// Leave empty for production (defaults to https://jobs.ashbyhq.com).
	baseURL string

	// apiBaseURL overrides the authenticated Ashby REST API endpoint — used in tests.
	// Leave empty for production (defaults to https://api.ashbyhq.com).
	apiBaseURL string

	// apiKey is used for authenticated Ashby REST API calls like job.list / job.info.
	apiKey string
}

func (a *Ashby) graphqlURL(op string) string {
	base := "https://jobs.ashbyhq.com"
	if a.baseURL != "" {
		base = a.baseURL
	}
	return strings.TrimRight(base, "/") + "/api/non-user-graphql?op=" + op
}

func (a *Ashby) restURL(path string) string {
	base := "https://api.ashbyhq.com"
	if a.apiBaseURL != "" {
		base = a.apiBaseURL
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

// CareersURL returns the public Ashby hosted jobs page URL for a given company slug.
func (a *Ashby) CareersURL(id string) string {
	return "https://jobs.ashbyhq.com/" + id
}

func ashbyRawID(id string) string {
	prefix := model.AdapterAshby + "-"
	return strings.TrimPrefix(id, prefix)
}

// ── Public hosted jobs GraphQL response structs ──────────────────────────────

type fetchJobsResponse struct {
	Data struct {
		JobBoardWithTeams struct {
			JobPostings []struct {
				ID                      string `json:"id"`
				Title                   string `json:"title"`
				LocationName            string `json:"locationName"`
				WorkplaceType           string `json:"workplaceType"`           // Remote, Hybrid, Onsite
				EmploymentType          string `json:"employmentType"`          // FullTime, PartTime, etc.
				CompensationTierSummary string `json:"compensationTierSummary"` // e.g. "$288K – $425K"
			} `json:"jobPostings"`
		} `json:"jobBoardWithTeams"`
	} `json:"data"`
}

func (r *fetchJobsResponse) toFetchJobsResponse(slug string) types.FetchJobsResponse {
	postings := r.Data.JobBoardWithTeams.JobPostings
	items := make([]types.FetchJobsItem, 0, len(postings))
	for _, j := range postings {
		location := j.LocationName
		if j.WorkplaceType != "" {
			location = fmt.Sprintf("%s (%s)", j.LocationName, j.WorkplaceType)
		}
		items = append(items, types.FetchJobsItem{
			ID:             fmt.Sprintf("%s-%s", model.AdapterAshby, j.ID),
			Title:          j.Title,
			Location:       location,
			URL:            fmt.Sprintf("https://jobs.ashbyhq.com/%s/%s", slug, j.ID),
			EmploymentType: normalizeAshbyEmploymentType(j.EmploymentType),
		})
	}
	return types.FetchJobsResponse{Jobs: items}
}

type fetchJobContentResponse struct {
	Data struct {
		JobPosting struct {
			ID                      string `json:"id"`
			Title                   string `json:"title"`
			LocationName            string `json:"locationName"`
			WorkplaceType           string `json:"workplaceType"`
			EmploymentType          string `json:"employmentType"`
			DescriptionHtml         string `json:"descriptionHtml"`
			CompensationTierSummary string `json:"compensationTierSummary"`
			PublishedDate           string `json:"publishedDate"`
		} `json:"jobPosting"`
	} `json:"data"`
}

func (r *fetchJobContentResponse) toFetchJobContentResponse() types.FetchJobContentResponse {
	p := r.Data.JobPosting
	location := p.LocationName
	if p.WorkplaceType != "" {
		location = fmt.Sprintf("%s (%s)", p.LocationName, p.WorkplaceType)
	}
	return types.FetchJobContentResponse{
		ID:             fmt.Sprintf("%s-%s", model.AdapterAshby, p.ID),
		Title:          p.Title,
		Location:       location,
		Description:    types.StripHTML(p.DescriptionHtml),
		EmploymentType: normalizeAshbyEmploymentType(p.EmploymentType),
		Compensation:   p.CompensationTierSummary,
		WorkplaceType:  p.WorkplaceType,
		PublishedDate:  p.PublishedDate,
	}
}

// ── Authenticated REST API response structs ──────────────────────────────────

type jobListResponse struct {
	Success           bool     `json:"success"`
	MoreDataAvailable bool     `json:"moreDataAvailable"`
	NextCursor        string   `json:"nextCursor,omitempty"`
	SyncToken         string   `json:"syncToken,omitempty"`
	Results           []apiJob `json:"results"`
}

type jobInfoResponse struct {
	Success bool   `json:"success"`
	Results apiJob `json:"results"`
}

type apiJob struct {
	ID                     string           `json:"id"`
	Title                  string           `json:"title"`
	Confidential           bool             `json:"confidential"`
	Status                 string           `json:"status"`
	EmploymentType         string           `json:"employmentType"`
	LocationID             string           `json:"locationId,omitempty"`
	DepartmentID           string           `json:"departmentId,omitempty"`
	DefaultInterviewPlanID string           `json:"defaultInterviewPlanId,omitempty"`
	InterviewPlanIDs       []string         `json:"interviewPlanIds,omitempty"`
	CustomFields           []customField    `json:"customFields,omitempty"`
	JobPostingIDs          []string         `json:"jobPostingIds,omitempty"`
	CustomRequisitionID    string           `json:"customRequisitionId,omitempty"`
	BrandID                string           `json:"brandId,omitempty"`
	HiringTeam             []hiringTeam     `json:"hiringTeam,omitempty"`
	Author                 *author          `json:"author,omitempty"`
	CreatedAt              string           `json:"createdAt,omitempty"`
	UpdatedAt              string           `json:"updatedAt,omitempty"`
	OpenedAt               string           `json:"openedAt,omitempty"`
	ClosedAt               string           `json:"closedAt,omitempty"`
	// Expanded fields — only present when expand:["location","compensation"] is requested
	Location     *apiLocation     `json:"location,omitempty"`
	Compensation *apiCompensation `json:"compensation,omitempty"`
}

type apiLocation struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ExternalName  string `json:"externalName"`
	WorkplaceType string `json:"workplaceType"` // Remote, Hybrid, OnSite
}

type apiCompensation struct {
	CompensationTiers []struct {
		TierSummary string `json:"tierSummary"`
	} `json:"compensationTiers"`
}

type customField struct {
	ID         string      `json:"id"`
	IsPrivate  bool        `json:"isPrivate,omitempty"`
	Title      string      `json:"title"`
	ValueLabel interface{} `json:"valueLabel,omitempty"`
	Value      interface{} `json:"value"`
}

type hiringTeam struct {
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Role      string `json:"role"`
	UserID    string `json:"userId"`
}

type author struct {
	ID           string        `json:"id"`
	FirstName    string        `json:"firstName"`
	LastName     string        `json:"lastName"`
	Email        string        `json:"email"`
	GlobalRole   string        `json:"globalRole"`
	IsEnabled    bool          `json:"isEnabled"`
	UpdatedAt    string        `json:"updatedAt,omitempty"`
	ManagerID    string        `json:"managerId,omitempty"`
	CustomFields []customField `json:"customFields,omitempty"`
}

func (r *jobListResponse) toAPIFetchJobsResponse() types.FetchJobsResponse {
	items := make([]types.FetchJobsItem, 0, len(r.Results))
	for _, j := range r.Results {
		location := ""
		if j.Location != nil {
			location = j.Location.ExternalName
			if location == "" {
				location = j.Location.Name
			}
		}
		items = append(items, types.FetchJobsItem{
			ID:             fmt.Sprintf("%s-%s", model.AdapterAshby, j.ID),
			Title:          j.Title,
			Location:       location,
			URL:            "",
			EmploymentType: normalizeAshbyEmploymentType(j.EmploymentType),
		})
	}
	return types.FetchJobsResponse{Jobs: items}
}

func (r *jobInfoResponse) toAPIFetchJobContentResponse() types.FetchJobContentResponse {
	j := r.Results
	location, workplaceType := "", ""
	if j.Location != nil {
		location = j.Location.ExternalName
		if location == "" {
			location = j.Location.Name
		}
		workplaceType = j.Location.WorkplaceType
	}
	comp := ""
	if j.Compensation != nil && len(j.Compensation.CompensationTiers) > 0 {
		comp = j.Compensation.CompensationTiers[0].TierSummary
	}
	return types.FetchJobContentResponse{
		ID:             fmt.Sprintf("%s-%s", model.AdapterAshby, j.ID),
		Title:          j.Title,
		Location:       location,
		WorkplaceType:  workplaceType,
		EmploymentType: normalizeAshbyEmploymentType(j.EmploymentType),
		Compensation:   comp,
		PublishedDate:  j.OpenedAt,
	}
}

// normalizeAshbyEmploymentType maps Ashby's EmploymentType enum to our model constants.
func normalizeAshbyEmploymentType(t string) string {
	switch t {
	case "FullTime":
		return model.EmployFullTime
	case "PartTime":
		return model.EmployPartTime
	case "Contract", "Contractor":
		return model.EmployContract
	case "Intern", "Internship":
		return model.EmployInternship
	default:
		return ""
	}
}

// ── Public hosted jobs adapter implementation ────────────────────────────────

// FetchJobs for Ashby public hosted jobs pages.
// companyID can be either a bare slug (e.g. "openai")
// or a full URL (https://api.ashbyhq.com/posting-api/job-board/openai).
func (a *Ashby) FetchJobs(ctx context.Context, companyID string) (types.FetchJobsResponse, error) {
	slug := companyID
	payload := map[string]any{
		"operationName": "ApiJobBoardWithTeams",
		"variables": map[string]string{
			"organizationHostedJobsPageName": slug,
		},
		"query": `query ApiJobBoardWithTeams($organizationHostedJobsPageName: String!) {
			jobBoardWithTeams(organizationHostedJobsPageName: $organizationHostedJobsPageName) {
				jobPostings {
					id
					title
					locationName
					workplaceType
					employmentType
					compensationTierSummary
				}
			}
		}`,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("ashby: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.graphqlURL("ApiJobBoardWithTeams"), bytes.NewBuffer(body))
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("ashby: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("ashby: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.FetchJobsResponse{}, fmt.Errorf("ashby: unexpected status %d", resp.StatusCode)
	}

	var raw fetchJobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("ashby: decode: %w", err)
	}

	return raw.toFetchJobsResponse(slug), nil
}

// FetchJobContent implements types.ContentFetcher using the public hosted job GraphQL endpoint.
// job.URL is the hosted web URL (e.g. https://jobs.ashbyhq.com/{company}/{uuid}).
func (a *Ashby) FetchJobContent(ctx context.Context, job types.RawJob) (types.FetchJobContentResponse, error) {
	u, err := url.Parse(job.URL)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: parse job URL: %w", err)
	}

	// path: /{company}/{uuid}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: unexpected URL path %q", u.Path)
	}
	company, id := parts[0], parts[1]

	payload := map[string]any{
		"operationName": "ApiJobPosting",
		"variables": map[string]string{
			"organizationHostedJobsPageName": company,
			"jobPostingId":                   id,
		},
		"query": `query ApiJobPosting($organizationHostedJobsPageName: String!, $jobPostingId: String!) {
			jobPosting(
				organizationHostedJobsPageName: $organizationHostedJobsPageName
				jobPostingId: $jobPostingId
			) {
				id
				title
				locationName
				workplaceType
				employmentType
				descriptionHtml
				compensationTierSummary
				publishedDate
			}
		}`,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: marshal content payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.graphqlURL("ApiJobPosting"), bytes.NewBuffer(body))
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: build content request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: fetch content: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: content status %d", resp.StatusCode)
	}

	var raw fetchJobContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: decode content: %w", err)
	}
	if raw.Data.JobPosting.ID == "" {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: job not found")
	}

	return raw.toFetchJobContentResponse(), nil
}

// ── Authenticated REST API methods ───────────────────────────────────────────

// FetchJobsViaAPI fetches jobs from the authenticated Ashby REST API (job.list).
func (a *Ashby) FetchJobsViaAPI(ctx context.Context) (types.FetchJobsResponse, error) {
	if a.apiKey == "" {
		return types.FetchJobsResponse{}, fmt.Errorf("ashby: apiKey is required for job.list")
	}

	payload := map[string]any{
		"limit":  100,
		"status": []string{"Open"},
		"expand": []string{"location", "compensation"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("ashby: marshal job.list payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.restURL("job.list"), bytes.NewBuffer(body))
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("ashby: build job.list request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(a.apiKey, "")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("ashby: fetch job.list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.FetchJobsResponse{}, fmt.Errorf("ashby: job.list unexpected status %d", resp.StatusCode)
	}

	var raw jobListResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("ashby: decode job.list: %w", err)
	}
	if !raw.Success {
		return types.FetchJobsResponse{}, fmt.Errorf("ashby: job.list returned success=false")
	}

	return raw.toAPIFetchJobsResponse(), nil
}

// FetchJobContentViaAPI fetches one job from the authenticated Ashby REST API (job.info).
func (a *Ashby) FetchJobContentViaAPI(ctx context.Context, jobID string) (types.FetchJobContentResponse, error) {
	if a.apiKey == "" {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: apiKey is required for job.info")
	}
	if jobID == "" {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: empty job id")
	}

	payload := map[string]any{
		"id":     ashbyRawID(jobID),
		"expand": []string{"location", "compensation"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: marshal job.info payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.restURL("job.info"), bytes.NewBuffer(body))
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: build job.info request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(a.apiKey, "")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: fetch job.info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: job.info unexpected status %d", resp.StatusCode)
	}

	var raw jobInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: decode job.info: %w", err)
	}
	if !raw.Success || raw.Results.ID == "" {
		return types.FetchJobContentResponse{}, fmt.Errorf("ashby: job not found")
	}

	return raw.toAPIFetchJobContentResponse(), nil
}
