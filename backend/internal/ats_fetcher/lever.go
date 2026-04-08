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

type Lever struct{}

type leverResponse []struct {
	ID         string `json:"id"`
	Text       string `json:"text"`      // Job Title
	HostedURL  string `json:"hostedUrl"` // Link to job
	Categories struct {
		Location   string `json:"location"`
		Team       string `json:"team"`
		Commitment string `json:"commitment"` // e.g. "Full-time"
	} `json:"categories"`
}

func (l *Lever) FetchJobs(ctx context.Context, companyID string) ([]RawJob, error) {
	apiURL := fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", companyID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("lever: build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lever: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lever: unexpected status %d", resp.StatusCode)
	}

	var result leverResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("lever: decode: %w", err)
	}

	jobs := make([]RawJob, 0, len(result))
	for _, j := range result {
		jobs = append(jobs, RawJob{
			Adapter:  model.AdapterLever,
			ID:       fmt.Sprintf("%s-%s", model.AdapterLever, j.ID),
			Title:    j.Text,
			Location: j.Categories.Location,
			URL:      j.HostedURL,
		})
	}

	return jobs, nil
}

// FetchJobContent implements ContentFetcher.
// job.URL is the hosted web URL (e.g. https://jobs.lever.co/{company}/{uuid}).
// We call the Lever API directly to get clean JSON for LLM enrichment.
func (l *Lever) FetchJobContent(ctx context.Context, job RawJob) (string, error) {
	u, err := url.Parse(job.URL)
	if err != nil {
		return "", fmt.Errorf("lever: parse job URL: %w", err)
	}
	// path: /{company}/{uuid}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("lever: unexpected URL path %q", u.Path)
	}
	company, id := parts[0], parts[1]
	apiURL := fmt.Sprintf("https://api.lever.co/v0/postings/%s/%s?mode=json", company, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("lever: build content request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("lever: fetch content: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lever: content status %d", resp.StatusCode)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", fmt.Errorf("lever: decode content: %w", err)
	}
	return string(raw), nil
}
