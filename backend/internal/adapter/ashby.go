package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/trushilpatel/jobradar/internal/model"
)

type Ashby struct{}

type ashbyResponse struct {
	Data struct {
		JobBoard struct {
			JobPostings []struct {
				ID                      string `json:"id"`
				Title                   string `json:"title"`
				LocationName            string `json:"locationName"`
				WorkplaceType           string `json:"workplaceType"` // Remote, Hybrid, Onsite
				CompensationTierSummary string `json:"compensationTierSummary"`
			} `json:"jobPostings"`
		} `json:"jobBoard"`
	} `json:"data"`
}

// FetchJobs for Ashby requires the company slug (e.g., "openai")
func (a *Ashby) FetchJobs(ctx context.Context, companyID string) ([]RawJob, error) {
	// The public gateway endpoint
	url := "https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiJobBoardWithTeams"

	// GraphQL payload to mimic the browser request
	payload := map[string]interface{}{
		"operationName": "ApiJobBoardWithTeams",
		"variables": map[string]string{
			"organizationHostedJobsPageName": companyID,
		},
		"query": `query ApiJobBoardWithTeams($organizationHostedJobsPageName: String!) {
			jobBoard(organizationHostedJobsPageName: $organizationHostedJobsPageName) {
				jobPostings {
					id
					title
					locationName
					workplaceType
					compensationTierSummary
				}
			}
		}`,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ashby: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("ashby: build request: %w", err)
	}

	// CRITICAL: Headers to avoid 401/403 Unauthorized
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

	postings := result.Data.JobBoard.JobPostings
	jobs := make([]RawJob, 0, len(postings))
	for _, j := range postings {
		jobs = append(jobs, RawJob{
			Adapter:  model.AdapterAshby, // Ensure this constant exists in your model package
			ID:       fmt.Sprintf("%s-%s", model.AdapterAshby, j.ID),
			Title:    j.Title,
			Location: fmt.Sprintf("%s (%s)", j.LocationName, j.WorkplaceType),
			URL:      fmt.Sprintf("https://jobs.ashbyhq.com/%s/%s", companyID, j.ID),
		})
	}

	return jobs, nil
}

// FetchJobByID retrieves the full details for a single Ashby posting.
// companyID is the slug (e.g., "openai"), jobID is the UUID.
func (a *Ashby) FetchJobByID(ctx context.Context, companyID, jobID string) (*RawJob, error) {
	// The specific gateway for public single-job views
	url := "https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiJobPosting"

	// Full GraphQL payload based on the schema we discovered
	payload := map[string]interface{}{
		"operationName": "ApiJobPosting",
		"variables": map[string]string{
			"organizationHostedJobsPageName": companyID,
			"jobPostingId":                   jobID,
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
		return nil, fmt.Errorf("ashby: marshal single job payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("ashby: build single job request: %w", err)
	}

	// CRITICAL: Must mimic a browser to avoid 401 Unauthorized
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ashby: fetch single job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ashby: single job unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			JobPosting struct {
				ID                      string `json:"id"`
				Title                   string `json:"title"`
				LocationName            string `json:"locationName"`
				WorkplaceType           string `json:"workplaceType"`
				DescriptionHtml         string `json:"descriptionHtml"`
				CompensationTierSummary string `json:"compensationTierSummary"`
			} `json:"jobPosting"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ashby: decode single job: %w", err)
	}

	j := result.Data.JobPosting
	// Handle case where job might be unlisted/404 in the data layer
	if j.ID == "" {
		return nil, fmt.Errorf("ashby: job not found or unauthorized")
	}

	return &RawJob{
		Adapter:  model.AdapterAshby,
		ID:       fmt.Sprintf("%s-%s", model.AdapterAshby, j.ID),
		Title:    j.Title,
		Location: fmt.Sprintf("%s (%s)", j.LocationName, j.WorkplaceType),
		URL:      fmt.Sprintf("https://jobs.ashbyhq.com/%s/%s", companyID, jobID),
		// Description and Salary can be mapped to your RawJob fields if they exist
	}, nil
}
