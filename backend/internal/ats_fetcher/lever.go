package ats_fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/trushilpatel/jobradar/internal/model"
)

type Lever struct {
	// baseURL overrides the Lever API base — used in tests to point at a fake server.
	// Leave empty for production (defaults to https://api.lever.co).
	baseURL string
}

func (l *Lever) apiURL(path string) string {
	base := "https://api.lever.co"
	if l.baseURL != "" {
		base = l.baseURL
	}
	return base + path
}

// ── Internal API response structs ────────────────────────────────────────────

type leverFetchJobsResponse []struct {
	ID        string `json:"id"`
	Text      string `json:"text"`      // job title
	HostedURL string `json:"hostedUrl"` // hosted web URL
	ApplyURL  string `json:"applyUrl"`  // direct application form URL
	Country   string `json:"country"`   // ISO 3166-1 alpha-2, nullable
	Categories struct {
		Location     string   `json:"location"`
		Commitment   string   `json:"commitment"` // e.g. "Full-time", "Contract"
		Team         string   `json:"team"`
		Department   string   `json:"department"`
		AllLocations []string `json:"allLocations"`
	} `json:"categories"`
	WorkplaceType string `json:"workplaceType"` // unspecified | on-site | remote | hybrid
}

func (r leverFetchJobsResponse) toFetchJobsResponse() FetchJobsResponse {
	items := make([]FetchJobsItem, 0, len(r))
	for _, j := range r {
		items = append(items, FetchJobsItem{
			ID:             fmt.Sprintf("%s-%s", model.AdapterLever, j.ID),
			Title:          j.Text,
			Location:       j.Categories.Location,
			URL:            j.HostedURL,
			EmploymentType: normalizeLeverCommitment(j.Categories.Commitment),
		})
	}
	return FetchJobsResponse{Jobs: items}
}

type leverSalaryRange struct {
	Currency string  `json:"currency"` // e.g. "USD"
	Interval string  `json:"interval"` // e.g. "per-year-salary"
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
}

// leverFetchJobContentResponse mirrors the complete Lever Postings API response.
// Every field from GET /v0/postings/{company}/{id}?mode=json is captured here.
// toFetchJobContentResponse() picks what is needed for the pipeline.
type leverFetchJobContentResponse struct {
	ID      string `json:"id"`
	Text    string `json:"text"`      // job title
	Country string `json:"country"`   // ISO 3166-1 alpha-2, nullable

	// URLs
	HostedURL string `json:"hostedUrl"` // hosted job posting page
	ApplyURL  string `json:"applyUrl"`  // direct application form

	// Description — Lever provides multiple variants; use the most complete plain-text ones
	Opening             string `json:"opening"`             // opening section (HTML)
	OpeningPlain        string `json:"openingPlain"`        // opening section (plain text)
	Description         string `json:"description"`         // opening + body (HTML)
	DescriptionPlain    string `json:"descriptionPlain"`    // opening + body (plain text) ← primary
	DescriptionBody     string `json:"descriptionBody"`     // body without opening (HTML)
	DescriptionBodyPlain string `json:"descriptionBodyPlain"` // body without opening (plain text)
	Additional          string `json:"additional"`          // closing content (HTML)
	AdditionalPlain     string `json:"additionalPlain"`     // closing content (plain text)

	// Structured requirement/benefit sections
	Lists []struct {
		Text    string `json:"text"`    // section heading e.g. "Requirements"
		Content string `json:"content"` // unstyled HTML <li> items
	} `json:"lists"`

	// Compensation
	SalaryRange            *leverSalaryRange `json:"salaryRange"`            // structured range, optional
	SalaryDescription      string            `json:"salaryDescription"`      // salary narrative (HTML)
	SalaryDescriptionPlain string            `json:"salaryDescriptionPlain"` // salary narrative (plain text)

	WorkplaceType string `json:"workplaceType"` // unspecified | on-site | remote | hybrid

	Categories struct {
		Location     string   `json:"location"`
		Commitment   string   `json:"commitment"` // e.g. "Full-time"
		Team         string   `json:"team"`
		Department   string   `json:"department"` // broader grouping above team
		AllLocations []string `json:"allLocations"`
	} `json:"categories"`

	CreatedAt int64 `json:"createdAt"` // Unix ms timestamp
}

func (r *leverFetchJobContentResponse) toFetchJobContentResponse() FetchJobContentResponse {
	// Full description: Lever's descriptionPlain already contains opening + body.
	// Append additionalPlain (closing content) if present.
	desc := r.DescriptionPlain
	if r.AdditionalPlain != "" {
		desc = strings.TrimSpace(desc + "\n" + r.AdditionalPlain)
	}

	// Requirement/benefit sections from lists[]
	var reqParts []string
	for _, section := range r.Lists {
		plain := StripHTML(section.Content)
		if plain != "" {
			reqParts = append(reqParts, section.Text+": "+plain)
		}
	}

	// Compensation: prefer human-readable plain text; fall back to structured range
	comp := r.SalaryDescriptionPlain
	if comp == "" && r.SalaryRange != nil {
		comp = fmt.Sprintf("%.0f – %.0f %s / %s",
			r.SalaryRange.Min, r.SalaryRange.Max,
			r.SalaryRange.Currency, r.SalaryRange.Interval)
	}

	// Department: prefer the broader department field, fall back to team
	dept := r.Categories.Department
	if dept == "" {
		dept = r.Categories.Team
	}

	publishedDate := ""
	if r.CreatedAt > 0 {
		publishedDate = fmt.Sprintf("%d", r.CreatedAt)
	}

	return FetchJobContentResponse{
		ID:             fmt.Sprintf("%s-%s", model.AdapterLever, r.ID),
		Title:          r.Text,
		Location:       r.Categories.Location,
		URL:            r.HostedURL,
		Description:    desc,
		EmploymentType: normalizeLeverCommitment(r.Categories.Commitment),
		Department:     dept,
		WorkplaceType:  r.WorkplaceType,
		Compensation:   comp,
		Requirements:   strings.Join(reqParts, "\n"),
		PublishedDate:  publishedDate,
	}
}

// normalizeLeverCommitment maps Lever's free-text Commitment to our model constants.
func normalizeLeverCommitment(c string) string {
	switch strings.ToLower(c) {
	case "full-time", "fulltime", "full time":
		return model.EmployFullTime
	case "part-time", "parttime", "part time":
		return model.EmployPartTime
	case "contract", "contractor":
		return model.EmployContract
	case "internship", "intern":
		return model.EmployInternship
	default:
		return ""
	}
}

// ── Adapter implementation ────────────────────────────────────────────────────

func (l *Lever) FetchJobs(ctx context.Context, companyID string) (FetchJobsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		l.apiURL(fmt.Sprintf("/v0/postings/%s?mode=json", companyID)), nil)
	if err != nil {
		return FetchJobsResponse{}, fmt.Errorf("lever: build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return FetchJobsResponse{}, fmt.Errorf("lever: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return FetchJobsResponse{}, fmt.Errorf("lever: unexpected status %d", resp.StatusCode)
	}

	var raw leverFetchJobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return FetchJobsResponse{}, fmt.Errorf("lever: decode: %w", err)
	}

	return raw.toFetchJobsResponse(), nil
}

// FetchJobContent implements ContentFetcher.
// job.URL is the hosted web URL (e.g. https://jobs.lever.co/{company}/{uuid}).
// Calls the Lever API directly to get structured JSON.
func (l *Lever) FetchJobContent(ctx context.Context, job RawJob) (FetchJobContentResponse, error) {
	u, err := url.Parse(job.URL)
	if err != nil {
		return FetchJobContentResponse{}, fmt.Errorf("lever: parse job URL: %w", err)
	}
	// path: /{company}/{uuid}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 {
		return FetchJobContentResponse{}, fmt.Errorf("lever: unexpected URL path %q", u.Path)
	}
	company, id := parts[0], parts[1]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		l.apiURL(fmt.Sprintf("/v0/postings/%s/%s?mode=json", company, id)), nil)
	if err != nil {
		return FetchJobContentResponse{}, fmt.Errorf("lever: build content request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return FetchJobContentResponse{}, fmt.Errorf("lever: fetch content: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return FetchJobContentResponse{}, fmt.Errorf("lever: content status %d", resp.StatusCode)
	}

	var raw leverFetchJobContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return FetchJobContentResponse{}, fmt.Errorf("lever: decode content: %w", err)
	}

	return raw.toFetchJobContentResponse(), nil
}
