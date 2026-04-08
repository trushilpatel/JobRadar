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

type Greenhouse struct{}

type greenhouseResponse struct {
	Jobs []struct {
		Adapter string `json:"adapter"`
		ID      int    `json:"id"`
		// InternalJobID int    `json:"internal_job_id"`
		Title string `json:"title"`
		// RequisitionID int    `json:"requisition_id"`
		Location struct {
			Name string `json:"name"`
		} `json:"location"`
		AbsoluteURL string `json:"absolute_url"`
	} `json:"jobs"`
}

func (g *Greenhouse) FetchJobs(ctx context.Context, companyURL string) ([]RawJob, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, companyURL, nil)
	if err != nil {
		return nil, fmt.Errorf("greenhouse: build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("greenhouse: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("greenhouse: unexpected status %d", resp.StatusCode)
	}

	var result greenhouseResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("greenhouse: decode: %w", err)
	}

	jobs := make([]RawJob, 0, len(result.Jobs))
	for _, j := range result.Jobs {

		jobs = append(jobs, RawJob{
			Adapter:  model.AdapterGreenhouse,
			ID:       fmt.Sprintf("%s-%d", model.AdapterGreenhouse, j.ID),
			Title:    j.Title,
			Location: j.Location.Name,
			URL:      j.AbsoluteURL,
		})
	}

	return jobs, nil
}

// FetchJobContent fetches the full job posting from the Greenhouse API.
// job.URL is the hosted web URL (e.g. https://job-boards.greenhouse.io/{slug}/jobs/{id}).
// We convert it to the JSON API endpoint: https://boards-api.greenhouse.io/v1/boards/{slug}/jobs/{id}
func (g *Greenhouse) FetchJobContent(ctx context.Context, job RawJob) (string, error) {
	u, err := url.Parse(job.URL)
	if err != nil {
		return "", fmt.Errorf("greenhouse: parse job URL: %w", err)
	}
	// path: /{slug}/jobs/{id}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 3 || parts[1] != "jobs" {
		return "", fmt.Errorf("greenhouse: unexpected URL path %q", u.Path)
	}
	slug, id := parts[0], parts[2]
	apiURL := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs/%s", slug, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("greenhouse: build content request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("greenhouse: fetch content: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("greenhouse: content status %d", resp.StatusCode)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", fmt.Errorf("greenhouse: decode content: %w", err)
	}
	return string(raw), nil
}
