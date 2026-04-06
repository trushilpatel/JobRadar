package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

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
	DescriptionHtml string `json:"descriptionHtml"` // Only present in single-job fetch
}

func (l *Lever) FetchJobs(ctx context.Context, companyID string) ([]RawJob, error) {
	// The public API endpoint for list
	url := fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", companyID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

// FetchJobByID retrieves the full details for a single Lever posting.
func (l *Lever) FetchJobByID(ctx context.Context, companyID, jobID string) (*RawJob, error) {
	// URL pattern: https://api.lever.co/v0/postings/{company}/{id}?mode=json
	url := fmt.Sprintf("https://api.lever.co/v0/postings/%s/%s?mode=json", companyID, jobID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("lever: build single job request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lever: fetch single job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lever: single job unexpected status %d", resp.StatusCode)
	}

	// Lever single job returns a single JSON object
	var j struct {
		ID         string `json:"id"`
		Text       string `json:"text"`
		HostedURL  string `json:"hostedUrl"`
		Categories struct {
			Location   string `json:"location"`
			Team       string `json:"team"`
			Commitment string `json:"commitment"`
		} `json:"categories"`
		DescriptionHtml string `json:"descriptionHtml"` // Detailed content
		AdditionalHtml  string `json:"additionalHtml"`  // Often contains "About Us"
	}

	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, fmt.Errorf("lever: decode single job: %w", err)
	}

	return &RawJob{
		Adapter:  model.AdapterLever,
		ID:       fmt.Sprintf("%s-%s", model.AdapterLever, j.ID),
		Title:    j.Text,
		Location: j.Categories.Location,
		URL:      j.HostedURL,
		// You can map j.DescriptionHtml to your internal model if needed
	}, nil
}
