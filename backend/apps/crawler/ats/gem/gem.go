package gem

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

const (
	defaultBaseURL = "https://api.gem.com"

	// jobPostsPathTemplate is the public REST endpoint that returns every open
	// job for a vanity-slug board in a single response. No auth required.
	//
	// Reference: https://help.gem.com/databases/gem-help-center/the-job-board-api
	jobPostsPathTemplate = "/job_board/v0/%s/job_posts/"
)

// Gem implements types.Adapter against gem.com's public Job Board REST API.
//
// FetchJobs issues a single GET to /job_board/v0/{slug}/job_posts/ which returns
// every open posting for the board, including the full HTML description. Because
// the listing response already carries the complete content, each FetchJobsItem
// is emitted with FullContent populated — the analyzer skips the detail fetch
// (see ats/CLAUDE.md: full-detail adapter path).
//
// No ContentFetcher is implemented; gem.com does not expose a per-job detail
// endpoint (verified: GET .../job_posts/{id} returns 404).
type Gem struct {
	// baseURL overrides the gem.com API host — used in tests to point at httptest.
	baseURL string
}

// New constructs a Gem adapter with production defaults.
func New() *Gem {
	return &Gem{baseURL: defaultBaseURL}
}

// CareersURL returns the public hosted job board URL for a Gem vanity slug.
func (g *Gem) CareersURL(slug string) string {
	return "https://jobs.gem.com/" + slug
}

// ── Gem Job Board API response types ────────────────────────────────────────
// Mirrors the full documented response so adding a downstream field never
// requires touching the HTTP/decode path (per ats/CLAUDE.md design rule).

type gemJobPost struct {
	ID               string        `json:"id"`
	InternalJobID    string        `json:"internal_job_id"`
	Title            string        `json:"title"`
	AbsoluteURL      string        `json:"absolute_url"`
	Content          string        `json:"content"`       // full HTML
	ContentPlain     string        `json:"content_plain"` // pre-stripped plain text
	Location         gemLocation   `json:"location"`
	LocationType     string        `json:"location_type"`   // in_office | remote | hybrid
	EmploymentType   string        `json:"employment_type"` // already snake_case
	Departments      []gemTaxonomy `json:"departments"`
	Offices          []gemOffice   `json:"offices"`
	RequisitionID    string        `json:"requisition_id"`
	CreatedAt        string        `json:"created_at"`         // ISO 8601
	UpdatedAt        string        `json:"updated_at"`         // ISO 8601
	FirstPublishedAt string        `json:"first_published_at"` // ISO 8601
}

type gemLocation struct {
	Name string `json:"name"`
}

type gemTaxonomy struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	ParentID *string  `json:"parent_id"`
	ChildIDs []string `json:"child_ids"`
}

type gemOffice struct {
	ID                     string      `json:"id"`
	Name                   string      `json:"name"`
	Location               gemLocation `json:"location"`
	ParentID               *string     `json:"parent_id"`
	ChildIDs               []string    `json:"child_ids"`
	ParentOfficeExternalID *string     `json:"parent_office_external_id"`
	ChildOfficeExternalIDs []string    `json:"child_office_external_ids"`
}

// ── Adapter implementation ──────────────────────────────────────────────────

// FetchJobs returns every open posting for the given gem.com vanity slug.
// companyID is the slug exactly as stored in companies.url (e.g. "inception").
func (g *Gem) FetchJobs(ctx context.Context, companyID string) (types.FetchJobsResponse, error) {
	slug := strings.TrimSpace(companyID)
	if slug == "" {
		return types.FetchJobsResponse{}, fmt.Errorf("gem: empty company slug")
	}

	url := g.resolveBaseURL() + fmt.Sprintf(jobPostsPathTemplate, slug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("gem: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("gem: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.FetchJobsResponse{}, fmt.Errorf("gem: unexpected status %d", resp.StatusCode)
	}

	var raw []gemJobPost
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.FetchJobsResponse{}, fmt.Errorf("gem: decode: %w", err)
	}

	items := make([]types.FetchJobsItem, 0, len(raw))
	for _, j := range raw {
		items = append(items, jobToItem(j))
	}
	return types.FetchJobsResponse{Jobs: items}, nil
}

// ── Converters ──────────────────────────────────────────────────────────────

func jobToItem(j gemJobPost) types.FetchJobsItem {
	loc := resolveLocation(j)
	return types.FetchJobsItem{
		ID:             fmt.Sprintf("%s-%s", companycommon.AdapterGem, j.ID),
		Title:          j.Title,
		URL:            j.AbsoluteURL,
		Location:       loc,
		EmploymentType: mapEmploymentType(j.EmploymentType),
		FullContent: &types.FullJobContent{
			Description:   resolveDescription(j),
			Department:    firstDepartment(j.Departments),
			WorkplaceType: mapWorkplaceType(j.LocationType),
			PublishedDate: resolvePublishedDate(j),
		},
		LocationKnown: true,
		AllLocations:  []string{loc},
	}
}

// resolveLocation prefers location.name, falling back to joining all office
// locations when the primary is empty.
func resolveLocation(j gemJobPost) string {
	if s := strings.TrimSpace(j.Location.Name); s != "" {
		return s
	}
	parts := make([]string, 0, len(j.Offices))
	for _, o := range j.Offices {
		if s := strings.TrimSpace(o.Location.Name); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "; ")
}

// resolveDescription prefers the pre-stripped content_plain field when present
// (gem provides it directly), falling back to StripHTML over the HTML content.
func resolveDescription(j gemJobPost) string {
	if s := strings.TrimSpace(j.ContentPlain); s != "" {
		return s
	}
	return types.StripHTML(j.Content)
}

// resolvePublishedDate prefers first_published_at; falls back to updated_at,
// then created_at — whichever is first non-empty.
func resolvePublishedDate(j gemJobPost) string {
	for _, s := range []string{j.FirstPublishedAt, j.UpdatedAt, j.CreatedAt} {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

func firstDepartment(d []gemTaxonomy) string {
	if len(d) == 0 {
		return ""
	}
	return d[0].Name
}

// mapEmploymentType maps gem's already-snake_case enum onto our internal
// jobcommon constants. Unknown values default to full_time, consistent with
// every other adapter.
func mapEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "full_time":
		return jobcommon.EmployFullTime
	case "part_time":
		return jobcommon.EmployPartTime
	case "contract":
		return jobcommon.EmployContract
	case "intern", "internship":
		return jobcommon.EmployInternship
	default:
		return jobcommon.EmployFullTime
	}
}

// mapWorkplaceType normalizes gem's location_type values to the strings the
// rest of the pipeline already expects (remote / hybrid / onsite).
func mapWorkplaceType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "in_office", "onsite", "on_site":
		return "onsite"
	default:
		return ""
	}
}

func (g *Gem) resolveBaseURL() string {
	if g.baseURL != "" {
		return g.baseURL
	}
	return defaultBaseURL
}

// Compile-time interface checks.
var (
	_ types.Adapter     = (*Gem)(nil)
	_ types.URLProvider = (*Gem)(nil)
)
