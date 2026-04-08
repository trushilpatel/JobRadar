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

type Greenhouse struct {
	// contentBaseURL overrides the Greenhouse API base — used in tests to point at a fake server.
	// Leave empty for production (defaults to https://boards-api.greenhouse.io).
	contentBaseURL string
}

func (g *Greenhouse) contentURL(slug, id string) string {
	base := "https://boards-api.greenhouse.io"
	if g.contentBaseURL != "" {
		base = g.contentBaseURL
	}
	return fmt.Sprintf("%s/v1/boards/%s/jobs/%s", base, slug, id)
}

// ── Internal API response structs ────────────────────────────────────────────

type greenhouseFetchJobsResponse struct {
	Jobs []struct {
		ID       int    `json:"id"`
		Title    string `json:"title"`
		Location struct {
			Name string `json:"name"`
		} `json:"location"`
		AbsoluteURL string `json:"absolute_url"`
	} `json:"jobs"`
}

func (r *greenhouseFetchJobsResponse) toFetchJobsResponse() FetchJobsResponse {
	items := make([]FetchJobsItem, 0, len(r.Jobs))
	for _, j := range r.Jobs {
		items = append(items, FetchJobsItem{
			ID:       fmt.Sprintf("%s-%d", model.AdapterGreenhouse, j.ID),
			Title:    j.Title,
			Location: j.Location.Name,
			URL:      j.AbsoluteURL,
			// EmploymentType: not provided in Greenhouse listing API
		})
	}
	return FetchJobsResponse{Jobs: items}
}

type greenhouseFetchJobContentResponse struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
	AbsoluteURL string `json:"absolute_url"`
	Content     string `json:"content"` // full HTML job description
	Departments []struct {
		Name string `json:"name"`
	} `json:"departments"`
	UpdatedAt     string `json:"updated_at"`
	PayInputRanges []struct {
		MinCents     int    `json:"min_cents"`
		MaxCents     int    `json:"max_cents"`
		CurrencyType string `json:"currency_type"`
		Title        string `json:"title"`
		Blurb        string `json:"blurb"`
	} `json:"pay_input_ranges"`
}

func (r *greenhouseFetchJobContentResponse) toFetchJobContentResponse() FetchJobContentResponse {
	dept := ""
	if len(r.Departments) > 0 {
		dept = r.Departments[0].Name
	}
	comp := ""
	if len(r.PayInputRanges) > 0 {
		p := r.PayInputRanges[0]
		comp = fmt.Sprintf("$%d – $%d %s", p.MinCents/100, p.MaxCents/100, p.CurrencyType)
	}
	return FetchJobContentResponse{
		ID:            fmt.Sprintf("%s-%d", model.AdapterGreenhouse, r.ID),
		Title:         r.Title,
		Location:      r.Location.Name,
		URL:           r.AbsoluteURL,
		Description:   StripHTML(r.Content),
		Department:    dept,
		Compensation:  comp,
		PublishedDate: r.UpdatedAt,
	}
}

// ── Adapter implementation ────────────────────────────────────────────────────

func (g *Greenhouse) FetchJobs(ctx context.Context, companyURL string) (FetchJobsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, companyURL, nil)
	if err != nil {
		return FetchJobsResponse{}, fmt.Errorf("greenhouse: build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return FetchJobsResponse{}, fmt.Errorf("greenhouse: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return FetchJobsResponse{}, fmt.Errorf("greenhouse: unexpected status %d", resp.StatusCode)
	}

	var raw greenhouseFetchJobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return FetchJobsResponse{}, fmt.Errorf("greenhouse: decode: %w", err)
	}

	return raw.toFetchJobsResponse(), nil
}

// FetchJobContent implements ContentFetcher.
// job.URL is the hosted web URL (e.g. https://job-boards.greenhouse.io/{slug}/jobs/{id}).
// Converts it to the JSON API endpoint and returns a structured response.
func (g *Greenhouse) FetchJobContent(ctx context.Context, job RawJob) (FetchJobContentResponse, error) {
	u, err := url.Parse(job.URL)
	if err != nil {
		return FetchJobContentResponse{}, fmt.Errorf("greenhouse: parse job URL: %w", err)
	}
	// path: /{slug}/jobs/{id}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 3 || parts[1] != "jobs" {
		return FetchJobContentResponse{}, fmt.Errorf("greenhouse: unexpected URL path %q", u.Path)
	}
	slug, id := parts[0], parts[2]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.contentURL(slug, id), nil)
	if err != nil {
		return FetchJobContentResponse{}, fmt.Errorf("greenhouse: build content request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return FetchJobContentResponse{}, fmt.Errorf("greenhouse: fetch content: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return FetchJobContentResponse{}, fmt.Errorf("greenhouse: content status %d", resp.StatusCode)
	}

	var raw greenhouseFetchJobContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return FetchJobContentResponse{}, fmt.Errorf("greenhouse: decode content: %w", err)
	}

	return raw.toFetchJobContentResponse(), nil
}
