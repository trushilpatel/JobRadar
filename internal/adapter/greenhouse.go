package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Greenhouse struct{}

type greenhouseResponse struct {
	Jobs []struct {
		ID         int    `json:"id"`
		Title      string `json:"title"`
		Location   struct {
			Name string `json:"name"`
		} `json:"location"`
		AbsoluteURL string `json:"absolute_url"`
		Departments []struct {
			Name string `json:"name"`
		} `json:"departments"`
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
		department := ""
		if len(j.Departments) > 0 {
			department = j.Departments[0].Name
		}

		jobs = append(jobs, RawJob{
			ID:         fmt.Sprintf("greenhouse-%d", j.ID),
			Title:      j.Title,
			Location:   j.Location.Name,
			URL:        j.AbsoluteURL,
			Department: department,
		})
	}

	return jobs, nil
}
