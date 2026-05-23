package apple

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

const (
	defaultBaseURL = "https://jobs.apple.com"

	// listingPath is the search endpoint with US filter applied.
	listingPath = "/en-us/search?location=united-states-USA&page=%d"

	// userAgent mimics a real browser to avoid blocking.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

	// publicJobURLTemplate constructs the public job detail page from id and transformedPostingTitle.
	// Resolved at scan time using the adapter's base URL so tests can intercept the detail-page fetch.
	publicJobURLTemplate = "%s/en-us/details/%s/%s"

	// usaCountryID is the ISO country identifier the Apple API uses for the US.
	usaCountryID = "iso-country-USA"
)

// payRangeRe captures "$N,NNN and $N,NNN" or "$N,NNN to $N,NNN" style ranges
// out of the job-detail postingFooters HTML blob. Apple consistently uses the
// "between $X and $Y" phrasing.
//
// Each amount is matched as a properly grouped US number (1-3 leading digits
// then comma-separated thousands) so a trailing list comma after the upper
// bound cannot leak into the captured range.
var payRangeRe = regexp.MustCompile(`\$\d{1,3}(?:,\d{3})*(?:\.\d+)?\s*(?:and|to|–|—|-)\s*\$\d{1,3}(?:,\d{3})*(?:\.\d+)?`)

// Apple implements types.Adapter and types.ContentFetcher by scraping
// jobs.apple.com listing pages and detail pages.
//
// FetchJobs:
//  1. Paginates through /en-us/search?location=united-states-USA&page={n}
//  2. Extracts window.__staticRouterHydrationData from each page's HTML
//  3. Emits one FetchJobsItem per listing — *thin* (no FullContent)
//
// FetchJobContent (called by the analyzer for each new job):
//  1. GETs job.URL (the public detail page)
//  2. Extracts window.__staticRouterHydrationData
//  3. Reads loaderData.jobDetails.jobsData and builds the rich
//     FetchJobContentResponse (description, qualifications, salary, etc.)
type Apple struct {
	// baseURL overrides the Apple host — used in tests.
	baseURL string
}

// New creates an Apple adapter with default production settings.
func New() *Apple {
	return &Apple{baseURL: defaultBaseURL}
}

// CareersURL returns the public Apple careers page URL.
func (a *Apple) CareersURL(_ string) string {
	return "https://www.apple.com/careers/us/"
}

// ── Response types: listing ─────────────────────────────────────────────────

// hydrationData is the top-level shape of window.__staticRouterHydrationData
// on listing pages.
type hydrationData struct {
	LoaderData loaderData `json:"loaderData"`
}

type loaderData struct {
	Search searchData `json:"search"`
}

type searchData struct {
	SearchResults []appleJob `json:"searchResults"`
	TotalRecords  int        `json:"totalRecords"`
}

// appleJob represents one entry in searchResults on the listing page.
type appleJob struct {
	ID                      string          `json:"id"`
	PositionID              string          `json:"positionId"`
	PostingTitle            string          `json:"postingTitle"`
	TransformedPostingTitle string          `json:"transformedPostingTitle"`
	JobSummary              string          `json:"jobSummary"`
	Locations               []appleLocation `json:"locations"`
	PostDateInGMT           string          `json:"postDateInGMT"`
	Team                    appleTeam       `json:"team"`
	HomeOffice              bool            `json:"homeOffice"`
	StandardWeeklyHours     int             `json:"standardWeeklyHours"`
	Type                    string          `json:"type"`
	ReqID                   string          `json:"reqId"`
	IsMultiLocation         bool            `json:"isMultiLocation"`
}

// appleLocation captures location fields from both listing and detail
// responses. Apple uses `stateProvince`+`countryID` on the listing API and
// adds optional code/zip fields on the detail API. Older payloads may carry
// `stateCode`/`countryCode` — both shapes are tolerated.
type appleLocation struct {
	PostLocationID string `json:"postLocationId"`
	Name           string `json:"name"`
	City           string `json:"city"`
	Code           string `json:"code"`
	StateProvince  string `json:"stateProvince"`
	StateCode      string `json:"stateCode"`
	CountryName    string `json:"countryName"`
	CountryCode    string `json:"countryCode"`
	CountryID      string `json:"countryID"`
	Metro          string `json:"metro"`
	Region         string `json:"region"`
	ZipCode        string `json:"zipCode"`
	Level          int    `json:"level"`
}

type appleTeam struct {
	TeamName string `json:"teamName"`
	TeamCode string `json:"teamCode"`
	TeamID   string `json:"teamID"`
}

// ── Response types: detail ──────────────────────────────────────────────────

// detailHydrationData is the top-level hydration shape on a job detail page.
type detailHydrationData struct {
	LoaderData detailLoaderData `json:"loaderData"`
}

type detailLoaderData struct {
	JobDetails detailJobDetails `json:"jobDetails"`
}

type detailJobDetails struct {
	JobsData appleJobDetail `json:"jobsData"`
}

// appleJobDetail mirrors loaderData.jobDetails.jobsData on the detail page.
type appleJobDetail struct {
	ID                      string          `json:"id"`
	JobNumber               string          `json:"jobNumber"`
	PositionID              string          `json:"positionId"`
	ReqID                   string          `json:"reqId"`
	PostingTitle            string          `json:"postingTitle"`
	TransformedPostingTitle string          `json:"transformedPostingTitle"`
	JobSummary              string          `json:"jobSummary"`
	Description             string          `json:"description"`
	Responsibilities        string          `json:"responsibilities"`
	MinimumQualifications   string          `json:"minimumQualifications"`
	PreferredQualifications string          `json:"preferredQualifications"`
	HighJobTitle            string          `json:"highJobTitle"`
	LowJobTitle             string          `json:"lowJobTitle"`
	TeamNames               []string        `json:"teamNames"`
	Locations               []appleLocation `json:"locations"`
	EmploymentType          string          `json:"employmentType"`
	JobType                 string          `json:"jobType"`
	Type                    string          `json:"type"`
	HomeOffice              bool            `json:"homeOffice"`
	PostDateInGMT           string          `json:"postDateInGMT"`
	PostingFooters          []appleFooter   `json:"postingFooters"`
}

// appleFooter is one entry in jobsData.postingFooters. The pay-range string
// lives in the entry whose Type starts with "POSTING_FOOTER_RULES".
type appleFooter struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// ── Adapter implementation ──────────────────────────────────────────────────

// FetchJobs returns all current Apple job listings from the US careers page.
// Apple is a *thin* adapter — only minimal listing fields are populated; the
// analyzer triggers FetchJobContent for each new job to fetch the rich detail
// page. companyID is ignored (Apple adapter targets a fixed host).
func (a *Apple) FetchJobs(ctx context.Context, _ string) (types.FetchJobsResponse, error) {
	var allJobs []types.FetchJobsItem
	page := 1

	for {
		select {
		case <-ctx.Done():
			return types.FetchJobsResponse{}, ctx.Err()
		default:
		}

		pageJobs, err := a.fetchJobsPage(ctx, page)
		if err != nil {
			return types.FetchJobsResponse{}, fmt.Errorf("apple: fetch page %d: %w", page, err)
		}

		// Stop when we get an empty page
		if len(pageJobs) == 0 {
			break
		}

		allJobs = append(allJobs, pageJobs...)
		page++
	}

	return types.FetchJobsResponse{Jobs: allJobs}, nil
}

// fetchJobsPage fetches a single page of job listings and returns FetchJobsItems.
func (a *Apple) fetchJobsPage(ctx context.Context, page int) ([]types.FetchJobsItem, error) {
	url := a.resolveBaseURL() + fmt.Sprintf(listingPath, page)

	body, err := a.fetchHTML(ctx, url)
	if err != nil {
		return nil, err
	}

	hydrationStr, err := extractHydrationData(body)
	if err != nil {
		return nil, fmt.Errorf("extract hydration data: %w", err)
	}

	var data hydrationData
	if err := json.Unmarshal([]byte(hydrationStr), &data); err != nil {
		return nil, fmt.Errorf("decode hydration JSON: %w", err)
	}

	items := make([]types.FetchJobsItem, 0, len(data.LoaderData.Search.SearchResults))
	for _, job := range data.LoaderData.Search.SearchResults {
		items = append(items, a.jobToItem(job))
	}
	return items, nil
}

// FetchJobContent implements types.ContentFetcher.
//
// It GETs the public detail page (job.URL), extracts the React Router
// hydration data, and returns the rich content the analyzer feeds to the LLM:
// description + responsibilities + min/pref qualifications, parsed
// compensation range, workplace type from homeOffice, department from
// teamNames, and a usable country code for the non-USA filter.
func (a *Apple) FetchJobContent(ctx context.Context, job types.RawJob) (types.FetchJobContentResponse, error) {
	if job.URL == "" {
		return types.FetchJobContentResponse{}, fmt.Errorf("apple: empty job URL")
	}

	body, err := a.fetchHTML(ctx, job.URL)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("apple: fetch detail %s: %w", job.URL, err)
	}

	hydrationStr, err := extractHydrationData(body)
	if err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("apple: extract detail hydration: %w", err)
	}

	var data detailHydrationData
	if err := json.Unmarshal([]byte(hydrationStr), &data); err != nil {
		return types.FetchJobContentResponse{}, fmt.Errorf("apple: decode detail JSON: %w", err)
	}

	return jobsDataToResponse(job, data.LoaderData.JobDetails.JobsData), nil
}

// fetchHTML performs a browser-headers GET and returns the body as a string.
func (a *Apple) fetchHTML(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	a.setHeaders(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(bodyBytes), nil
}

// setHeaders applies required browser-like headers to every request.
func (a *Apple) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
}

// ── Converters and helpers ──────────────────────────────────────────────────

// extractHydrationData finds and returns the JSON string from
// window.__staticRouterHydrationData. Handles three variants Apple has used:
//   - Direct object:           = {...};
//   - JSON.parse double-quote: = JSON.parse("{...}");
//   - JSON.parse single-quote: = JSON.parse('{...}');
func extractHydrationData(body string) (string, error) {
	const marker = "window.__staticRouterHydrationData = "
	_, after, found := strings.Cut(body, marker)
	if !found {
		return "", fmt.Errorf("window.__staticRouterHydrationData not found in page")
	}

	rest := strings.TrimLeft(after, " \t\r\n")

	const jsonParseCall = "JSON.parse("
	if strings.HasPrefix(rest, jsonParseCall) {
		arg := rest[len(jsonParseCall):]
		if len(arg) == 0 {
			return "", fmt.Errorf("empty JSON.parse() call")
		}
		quoteChar := arg[0]
		if quoteChar != '"' && quoteChar != '\'' {
			return "", fmt.Errorf("JSON.parse() argument has unexpected opening character %c", quoteChar)
		}
		inner, err := scanToClosingQuote(arg[1:], quoteChar)
		if err != nil {
			return "", fmt.Errorf("JSON.parse() argument: %w", err)
		}
		if quoteChar == '"' {
			var unescaped string
			if jsonErr := json.Unmarshal([]byte(`"`+inner+`"`), &unescaped); jsonErr != nil {
				return "", fmt.Errorf("decode JSON.parse string: %w", jsonErr)
			}
			return unescaped, nil
		}
		unescaped := strings.ReplaceAll(inner, `\'`, "'")
		unescaped = strings.ReplaceAll(unescaped, `\\`, `\`)
		return unescaped, nil
	}

	before, _, found := strings.Cut(rest, "</script>")
	if !found {
		return "", fmt.Errorf("closing </script> not found after hydration marker")
	}
	return strings.TrimRight(before, " \t\r\n;"), nil
}

// scanToClosingQuote returns the raw content of a JS string literal up to (but
// not including) the first unescaped occurrence of quote.
func scanToClosingQuote(s string, quote byte) (string, error) {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == quote {
			return s[:i], nil
		}
	}
	return "", fmt.Errorf("unclosed string — closing %c not found", quote)
}

// jobToItem converts a listing appleJob to a thin FetchJobsItem.
// FullContent is intentionally nil — the analyzer triggers FetchJobContent
// to populate the rich detail-page fields.
func (a *Apple) jobToItem(job appleJob) types.FetchJobsItem {
	loc := formatLocation(job.Locations)
	all := allLocationStrings(job.Locations)
	return types.FetchJobsItem{
		ID:             fmt.Sprintf("%s-%s", companycommon.AdapterApple, job.ID),
		Title:          job.PostingTitle,
		URL:            fmt.Sprintf(publicJobURLTemplate, a.resolveBaseURL(), job.ID, job.TransformedPostingTitle),
		Location:       loc,
		EmploymentType: mapJobType(job.Type),
		LocationKnown:  hasUSACountry(job.Locations),
		AllLocations:   all,
	}
}

// jobsDataToResponse builds the rich FetchJobContentResponse from a detail
// page's jobsData payload.
func jobsDataToResponse(job types.RawJob, jd appleJobDetail) types.FetchJobContentResponse {
	location := formatLocation(jd.Locations)
	if location == "" {
		location = job.Location
	}
	return types.FetchJobContentResponse{
		ID:             job.ID,
		Title:          firstNonEmpty(jd.PostingTitle, job.Title),
		Location:       location,
		URL:            job.URL,
		Description:    buildDescription(jd),
		EmploymentType: mapJobType(firstNonEmpty(jd.Type, jd.JobType, jd.EmploymentType)),
		Department:     firstTeamName(jd.TeamNames),
		Compensation:   extractCompensation(jd.PostingFooters),
		WorkplaceType:  workplaceType(jd.HomeOffice),
		Requirements:   "",
		PublishedDate:  jd.PostDateInGMT,
		CountryCode:    primaryCountryCode(jd.Locations),
	}
}

// buildDescription concatenates the structured detail-page sections with
// labels, separated by blank lines. Empty sections are skipped so the LLM
// never sees lone labels with no body.
func buildDescription(jd appleJobDetail) string {
	sections := []struct {
		label string
		body  string
	}{
		{"", types.StripHTML(jd.JobSummary)},
		{"", types.StripHTML(jd.Description)},
		{"Responsibilities", types.StripHTML(jd.Responsibilities)},
		{"Minimum Qualifications", types.StripHTML(jd.MinimumQualifications)},
		{"Preferred Qualifications", types.StripHTML(jd.PreferredQualifications)},
	}
	parts := make([]string, 0, len(sections))
	for _, s := range sections {
		if s.body == "" {
			continue
		}
		if s.label == "" {
			parts = append(parts, s.body)
			continue
		}
		parts = append(parts, s.label+":\n"+s.body)
	}
	return strings.Join(parts, "\n\n")
}

// extractCompensation pulls the first "$X and $Y" / "$X to $Y" range out of
// any postingFooters content. Returns empty string if no range is present.
func extractCompensation(footers []appleFooter) string {
	for _, f := range footers {
		text := types.StripHTML(f.Content)
		if match := payRangeRe.FindString(text); match != "" {
			return strings.Join(strings.Fields(match), " ")
		}
	}
	return ""
}

// workplaceType maps the homeOffice bool to the canonical workplace-type
// string the LLM/analyzer expects.
func workplaceType(homeOffice bool) string {
	if homeOffice {
		return "remote"
	}
	return ""
}

// formatLocation builds a human-readable location string from the job's
// locations array. Single location: "City, State". Multiple: joined with "; ".
func formatLocation(locations []appleLocation) string {
	if len(locations) == 0 {
		return ""
	}
	parts := make([]string, 0, len(locations))
	for _, loc := range locations {
		if s := formatSingleLocation(loc); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "; ")
}

// allLocationStrings returns one rendered string per location, suitable for
// the analyzer's per-location non-USA scan.
func allLocationStrings(locations []appleLocation) []string {
	if len(locations) == 0 {
		return nil
	}
	out := make([]string, 0, len(locations))
	for _, loc := range locations {
		if s := formatSingleLocation(loc); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// formatSingleLocation renders an appleLocation as "City, State". Falls back
// to Name when City is empty (listing API often only populates Name), and
// to StateCode when StateProvince is empty (older detail payloads).
func formatSingleLocation(loc appleLocation) string {
	city := firstNonEmpty(loc.City, loc.Name)
	state := firstNonEmpty(loc.StateProvince, loc.StateCode)
	parts := make([]string, 0, 2)
	if city != "" {
		parts = append(parts, city)
	}
	if state != "" {
		parts = append(parts, state)
	}
	return strings.Join(parts, ", ")
}

// hasUSACountry returns true when at least one location is tagged with the
// USA country marker — used to set LocationKnown.
func hasUSACountry(locations []appleLocation) bool {
	return slices.ContainsFunc(locations, isUSALocation)
}

// isUSALocation reports whether a location belongs to the United States,
// tolerating either the listing's countryID or the older countryCode/Name.
func isUSALocation(loc appleLocation) bool {
	if loc.CountryID == usaCountryID {
		return true
	}
	if strings.EqualFold(loc.CountryCode, "USA") || strings.EqualFold(loc.CountryCode, "US") {
		return true
	}
	if strings.EqualFold(loc.CountryName, "United States") ||
		strings.EqualFold(loc.CountryName, "United States of America") {
		return true
	}
	return false
}

// primaryCountryCode returns the ISO alpha-2 country code of the first
// location ("US" for any USA marker; empty if unknown). The analyzer's
// fetcher uses this to skip non-USA jobs before any LLM call.
func primaryCountryCode(locations []appleLocation) string {
	for _, loc := range locations {
		if isUSALocation(loc) {
			return "US"
		}
		if loc.CountryCode != "" {
			return strings.ToUpper(loc.CountryCode)
		}
		if loc.CountryID != "" && strings.HasPrefix(loc.CountryID, "iso-country-") {
			// Apple uses 3-letter codes here (e.g. iso-country-GBR); the
			// analyzer's check tolerates non-empty non-US strings as
			// "definite non-USA" so emit whatever we have.
			return strings.TrimPrefix(loc.CountryID, "iso-country-")
		}
	}
	return ""
}

// mapJobType converts Apple's type string to a model.EmployType constant.
// Apple uses several shapes — "Regular"/"Part-Time"/"Contract"/"Intern" on
// older payloads, "PIPE"/"REQ"/"CORPORATE"/"Standard" on newer ones. Anything
// not explicitly part-time/contract/intern is mapped to full-time.
func mapJobType(jobType string) string {
	switch strings.ToLower(strings.TrimSpace(jobType)) {
	case "part-time", "part_time", "parttime":
		return jobcommon.EmployPartTime
	case "contract", "contractor":
		return jobcommon.EmployContract
	case "intern", "internship":
		return jobcommon.EmployInternship
	default:
		return jobcommon.EmployFullTime
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstTeamName(names []string) string {
	for _, n := range names {
		if n != "" {
			return n
		}
	}
	return ""
}

func (a *Apple) resolveBaseURL() string {
	if a.baseURL != "" {
		return a.baseURL
	}
	return defaultBaseURL
}

// Compile-time interface checks.
var (
	_ types.Adapter        = (*Apple)(nil)
	_ types.URLProvider    = (*Apple)(nil)
	_ types.ContentFetcher = (*Apple)(nil)
)
