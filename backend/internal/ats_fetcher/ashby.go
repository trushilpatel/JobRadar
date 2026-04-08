package ats_fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/trushilpatel/jobradar/internal/model"
)

type Ashby struct {
	// baseURL overrides the Ashby GraphQL endpoint — used in tests to point at a fake server.
	// Leave empty for production (defaults to https://jobs.ashbyhq.com).
	baseURL string
}

func (a *Ashby) graphqlURL(op string) string {
	base := "https://jobs.ashbyhq.com"
	if a.baseURL != "" {
		base = a.baseURL
	}
	return base + "/api/non-user-graphql?op=" + op
}

type ashbyResponse struct {
	Data struct {
		JobBoardWithTeams struct {
			JobPostings []struct {
				ID                      string `json:"id"`
				Title                   string `json:"title"`
				LocationName            string `json:"locationName"`
				WorkplaceType           string `json:"workplaceType"`           // Remote, Hybrid, Onsite
				EmploymentType          string `json:"employmentType"`          // FullTime, PartTime, Contract, etc.
				CompensationTierSummary string `json:"compensationTierSummary"` // e.g. "$288K – $425K • Offers Equity"
			} `json:"jobPostings"`
		} `json:"jobBoardWithTeams"`
	} `json:"data"`
}

// slugFromURL extracts the company slug from an Ashby URL.
// Handles both full URLs (https://api.ashbyhq.com/posting-api/job-board/openai)
// and bare slugs (openai).
func slugFromURL(rawURL string) string {
	if !strings.Contains(rawURL, "/") {
		return rawURL // already a bare slug
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	// Use TrimRight only — a trailing slash means the last segment is empty (malformed)
	parts := strings.Split(strings.TrimLeft(u.Path, "/"), "/")
	return parts[len(parts)-1]
}

// FetchJobs for Ashby. companies.url can be either a bare slug (e.g. "openai")
// or a full URL (https://api.ashbyhq.com/posting-api/job-board/openai).
func (a *Ashby) FetchJobs(ctx context.Context, companyID string) ([]RawJob, error) {
	slug := slugFromURL(companyID)
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
		return nil, fmt.Errorf("ashby: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.graphqlURL("ApiJobBoardWithTeams"),
		bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("ashby: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ashby: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ashby: unexpected status %d", resp.StatusCode)
	}

	var result ashbyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ashby: decode: %w", err)
	}

	postings := result.Data.JobBoardWithTeams.JobPostings
	jobs := make([]RawJob, 0, len(postings))
	for _, j := range postings {
		location := j.LocationName
		if j.WorkplaceType != "" {
			location = fmt.Sprintf("%s (%s)", j.LocationName, j.WorkplaceType)
		}
		jobs = append(jobs, RawJob{
			Adapter:    model.AdapterAshby,
			ID:         fmt.Sprintf("%s-%s", model.AdapterAshby, j.ID),
			Title:      j.Title,
			Location:   location,
			URL:        fmt.Sprintf("https://jobs.ashbyhq.com/%s/%s", slug, j.ID),
			EmployType: normalizeAshbyEmploymentType(j.EmploymentType),
		})
	}

	return jobs, nil
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

// FetchJobContent implements ContentFetcher.
// job.URL is the hosted web URL (e.g. https://jobs.ashbyhq.com/{company}/{uuid}).
// We call the Ashby GraphQL API directly to get clean JSON for LLM enrichment.
func (a *Ashby) FetchJobContent(ctx context.Context, job RawJob) (string, error) {
	u, err := url.Parse(job.URL)
	if err != nil {
		return "", fmt.Errorf("ashby: parse job URL: %w", err)
	}
	// path: /{company}/{uuid}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("ashby: unexpected URL path %q", u.Path)
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
				descriptionHtml
				compensationTierSummary
				publishedDate
			}
		}`,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("ashby: marshal content payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.graphqlURL("ApiJobPosting"),
		bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("ashby: build content request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ashby: fetch content: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ashby: content status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			JobPosting json.RawMessage `json:"jobPosting"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ashby: decode content: %w", err)
	}
	if len(result.Data.JobPosting) == 0 || string(result.Data.JobPosting) == "null" {
		return "", fmt.Errorf("ashby: job not found")
	}
	return string(result.Data.JobPosting), nil
}
