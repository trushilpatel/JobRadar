// Package deel implements the ATS adapter for jobs.deel.com — Deel's hosted
// careers-page product for HR customers (Klarna, Dott, Mattilda, Sequence, …).
//
// Unlike gem.com (which exposes a clean public REST JSON API), Deel renders
// every job board with Next.js Server Components. The public API on
// api-prod.letsdeel.com is auth-gated, but the SSR page can be requested with
// the `RSC: 1` header to return the React Server Components stream directly —
// no JS execution required. The stream contains JSON job records embedded in
// numbered "T" (text) chunks; we extract them with balanced-brace parsing and
// resolve the `$<hex>` description references against the chunk map.
//
// FetchJobs issues a single HTTP request and emits every job with FullContent
// populated, so the analyzer skips the per-job detail fetch (see ats/CLAUDE.md
// — full-detail adapter path).
package deel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

const (
	defaultBaseURL = "https://jobs.deel.com"

	// rscHeader, set to "1", tells Next.js to return the raw React Server
	// Components stream instead of the wrapping HTML document.
	rscHeader = "RSC"

	// userAgent — Cloudfront in front of jobs.deel.com sometimes 403s the
	// default Go user agent on heavy traffic; a real browser UA is safer.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Deel implements types.Adapter against jobs.deel.com.
type Deel struct {
	// baseURL overrides the host — used in tests to point at httptest.
	baseURL string
}

// New constructs a Deel adapter with production defaults.
func New() *Deel {
	return &Deel{baseURL: defaultBaseURL}
}

// CareersURL returns the public hosted job board URL for a Deel vanity slug.
func (d *Deel) CareersURL(slug string) string {
	return defaultBaseURL + "/" + slug
}

// ── RSC payload types ───────────────────────────────────────────────────────
// Mirrors only the fields we use; Deel includes a lot more (applicationFormId,
// updatedAt, isCompensationVisible, …) which we ignore.

// rawJobPosting is one entry in the embedded RSC JSON stream. The outer `id`
// is the job posting id (used in the public detail URL); `jobId` is the
// internal job id from Deel's schema. Title and richtextDescription live at
// the outer level; everything else is on the nested `job` object.
type rawJobPosting struct {
	ID                  string             `json:"id"`
	JobID               string             `json:"jobId"`
	Title               string             `json:"title"`
	RichtextDescription json.RawMessage    `json:"richtextDescription"`
	Job                 rawJob             `json:"job"`
	JobPostingPubs      []rawJobPostingPub `json:"jobPostingPublications"`
}

type rawJob struct {
	JobEmploymentTypes  []rawEmploymentType `json:"jobEmploymentTypes"`
	JobLocations        []rawJobLocation    `json:"jobLocations"`
	JobDepartments      []rawJobDepartment  `json:"jobDepartments"`
	JobTeams            []rawJobTeam        `json:"jobTeams"`
	CurrentCompensation *rawCompensation    `json:"currentCompensation"`
}

type rawEmploymentType struct {
	EmploymentType rawNamed `json:"employmentType"`
}

type rawJobLocation struct {
	Location rawNamed `json:"location"`
}

type rawJobDepartment struct {
	Department rawNamed `json:"department"`
}

type rawJobTeam struct {
	Team rawNamed `json:"team"`
}

type rawNamed struct {
	Name string `json:"name"`
}

type rawCompensation struct {
	CurrencyIsoCode string  `json:"currencyIsoCode"`
	MinAmount       float64 `json:"minAmount"`
	MaxAmount       float64 `json:"maxAmount"`
}

type rawJobPostingPub struct {
	CurrentState rawPubState `json:"currentState"`
}

type rawPubState struct {
	CreatedAt string `json:"createdAt"`
}

// ── Adapter implementation ──────────────────────────────────────────────────

// FetchJobs returns every open posting for the given Deel vanity slug.
// companyID is the slug as stored in companies.url (e.g. "klarna").
func (d *Deel) FetchJobs(ctx context.Context, companyID string) (types.FetchJobsResponse, error) {
	slug := strings.TrimSpace(companyID)
	if slug == "" {
		return types.FetchJobsResponse{}, fmt.Errorf("deel: empty company slug")
	}

	body, err := d.fetchRSC(ctx, slug)
	if err != nil {
		return types.FetchJobsResponse{}, err
	}

	chunks := parseChunks(body)
	records := extractJobRecords(body)

	items := make([]types.FetchJobsItem, 0, len(records))
	for _, rec := range records {
		items = append(items, jobToItem(rec, slug, chunks))
	}
	return types.FetchJobsResponse{Jobs: items}, nil
}

// fetchRSC issues a GET with the RSC header set so Next.js returns the raw
// Server Components stream instead of the wrapping HTML document.
func (d *Deel) fetchRSC(ctx context.Context, slug string) (string, error) {
	url := d.resolveBaseURL() + "/" + slug
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("deel: build request: %w", err)
	}
	req.Header.Set(rscHeader, "1")
	req.Header.Set("Accept", "text/x-component")
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("deel: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("deel: unexpected status %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("deel: read body: %w", err)
	}
	return string(b), nil
}

// ── RSC stream parsing ──────────────────────────────────────────────────────

// chunkHeader matches the RSC text-chunk header `<hexId>:T<lenHex>,` that
// prefixes every streamed text payload. We use it both to build a map of
// chunkID → text content and to bound the data we scan for job records.
var chunkHeader = regexp.MustCompile(`([0-9a-f]+):T([0-9a-f]+),`)

// parseChunks walks the RSC stream and returns a map from chunk ID (lowercase
// hex, no leading zeros) to the chunk's text payload. T-chunks can be either
// CRLF-separated at the top level or concatenated together inside a single
// physical line, so we scan by header position rather than by `\n`.
func parseChunks(body string) map[string]string {
	out := make(map[string]string)
	headers := chunkHeader.FindAllStringSubmatchIndex(body, -1)
	for _, h := range headers {
		// h: [0]=match start, [1]=match end (after comma),
		//    [2..3]=hexId, [4..5]=lenHex
		id := body[h[2]:h[3]]
		lenStr := body[h[4]:h[5]]
		var n int
		if _, err := fmt.Sscanf(lenStr, "%x", &n); err != nil || n < 0 {
			continue
		}
		start := h[1]
		end := start + n
		if end > len(body) {
			end = len(body)
		}
		out[id] = body[start:end]
	}
	return out
}

// jobRecordStart matches the prefix of each job posting record in the stream.
// `id` is always followed by `jobId` — both UUIDs — and that pair is unique to
// JobPosting records (career-page-settings records use `id` + `organizationId`
// instead, so the prefix discriminates cleanly).
var jobRecordStart = regexp.MustCompile(`\{"id":"[a-f0-9-]{36}","jobId":"[a-f0-9-]{36}",`)

// extractJobRecords scans the entire RSC body for job-posting objects and
// returns each one decoded. Records are concatenated inside text chunks, so we
// locate each start position via regex and then balance braces to find the
// exact end of the object.
func extractJobRecords(body string) []rawJobPosting {
	starts := jobRecordStart.FindAllStringIndex(body, -1)
	out := make([]rawJobPosting, 0, len(starts))
	for _, s := range starts {
		end := matchBalanced(body, s[0])
		if end < 0 {
			continue
		}
		var rec rawJobPosting
		if err := json.Unmarshal([]byte(body[s[0]:end]), &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out
}

// matchBalanced returns the index past the closing `}` that pairs with the
// opening `{` at start, or -1 if the braces are unbalanced. String contents
// (and their escape sequences) are skipped so curly braces inside JSON
// strings don't throw the counter off.
func matchBalanced(s string, start int) int {
	if start >= len(s) || s[start] != '{' {
		return -1
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if esc {
			esc = false
			continue
		}
		if c == '\\' {
			esc = true
			continue
		}
		if c == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

// ── Converters ──────────────────────────────────────────────────────────────

func jobToItem(r rawJobPosting, slug string, chunks map[string]string) types.FetchJobsItem {
	return types.FetchJobsItem{
		ID:             fmt.Sprintf("%s-%s", companycommon.AdapterDeel, r.ID),
		Title:          r.Title,
		URL:            fmt.Sprintf("%s/%s/job-details/%s", defaultBaseURL, slug, r.ID),
		Location:       formatLocations(r.Job.JobLocations),
		EmploymentType: mapEmploymentType(firstEmploymentTypeName(r.Job.JobEmploymentTypes)),
		FullContent: &types.FullJobContent{
			Description:   resolveDescription(r.RichtextDescription, chunks),
			Department:    firstDepartmentName(r.Job.JobDepartments),
			WorkplaceType: "", // not provided in Deel structure
			Compensation:  formatCompensation(r.Job.CurrentCompensation),
			PublishedDate: firstPublishedAt(r.JobPostingPubs),
		},
	}
}

// formatLocations joins all location names with "; " — same convention used by
// the Uber adapter for multi-location postings.
func formatLocations(locs []rawJobLocation) string {
	if len(locs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(locs))
	for _, l := range locs {
		if s := strings.TrimSpace(l.Location.Name); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "; ")
}

func firstEmploymentTypeName(et []rawEmploymentType) string {
	if len(et) == 0 {
		return ""
	}
	return et[0].EmploymentType.Name
}

func firstDepartmentName(d []rawJobDepartment) string {
	if len(d) == 0 {
		return ""
	}
	return d[0].Department.Name
}

func firstPublishedAt(pubs []rawJobPostingPub) string {
	if len(pubs) == 0 {
		return ""
	}
	return pubs[0].CurrentState.CreatedAt
}

// formatCompensation renders Deel's structured pay range into the same
// human-readable form other adapters use.
func formatCompensation(c *rawCompensation) string {
	if c == nil || (c.MinAmount == 0 && c.MaxAmount == 0) {
		return ""
	}
	cur := c.CurrencyIsoCode
	if cur == "" {
		cur = "USD"
	}
	return fmt.Sprintf("%.0f – %.0f %s", c.MinAmount, c.MaxAmount, cur)
}

// resolveDescription dereferences `richtextDescription` against the RSC chunk
// map. The value is normally `"$<hex>"` (an RSC chunk reference), but legacy
// records can carry the HTML inline. Null/missing → empty string.
func resolveDescription(raw json.RawMessage, chunks map[string]string) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	// Try as a string first
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	if strings.HasPrefix(s, "$") {
		id := strings.TrimPrefix(s, "$")
		if html, ok := chunks[id]; ok {
			return types.StripHTML(html)
		}
		return ""
	}
	return types.StripHTML(s)
}

// mapEmploymentType normalizes Deel's free-form employmentType names onto the
// internal jobcommon constants. Deel uses titles like "Full-time", "Part-time",
// "Contract", "Consultant", "Internship" — we map known values and fall through
// to full_time for anything else, consistent with every other adapter.
func mapEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "full-time", "full time", "full_time":
		return jobcommon.EmployFullTime
	case "part-time", "part time", "part_time":
		return jobcommon.EmployPartTime
	case "contract", "contractor", "consultant", "freelance", "freelancer":
		return jobcommon.EmployContract
	case "intern", "internship":
		return jobcommon.EmployInternship
	default:
		return jobcommon.EmployFullTime
	}
}

func (d *Deel) resolveBaseURL() string {
	if d.baseURL != "" {
		return d.baseURL
	}
	return defaultBaseURL
}

// Compile-time interface checks.
var (
	_ types.Adapter     = (*Deel)(nil)
	_ types.URLProvider = (*Deel)(nil)
)
