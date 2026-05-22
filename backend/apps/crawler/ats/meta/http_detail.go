package meta

import (
	"bytes"
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

// detailURL is the canonical detail-page URL for a Meta job. Verified via
// browser fetch: returns 200 with full schema.org JobPosting JSON-LD inline.
//
// The shorter `/jobs/{id}/` form returns 302 to this canonical path; some
// HTTP clients (notably Python's urllib) error on the 302 chain, so we hit
// the canonical path directly.
const detailURLPrefix = "https://www.metacareers.com/profile/job_details/"

// userAgent — pinned Chrome 121 on macOS. Meta validates the request header
// set fairly strictly; a bare urllib User-Agent gets a 400.
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"

const acceptHeader = "text/html,application/xhtml+xml,application/xml;q=0.9," +
	"image/webp,*/*;q=0.8"
const acceptLangHeader = "en-US,en;q=0.9"

// reJSONLD finds every <script type="application/ld+json"> block in a Meta
// detail page. Greedy match across newlines because the JSON-LD payload
// contains real newlines inside quoted strings. We pick the JobPosting one.
var reJSONLD = regexp.MustCompile(
	`(?is)<script\s+type=["']application/ld\+json["'][^>]*>(.*?)</script>`,
)

// reMetaContent extracts the content attribute from a <meta> tag.
var reMetaContent = regexp.MustCompile(`(?i)content=["']([^"']*)["']`)

// extractMetaTag returns the content attribute of the first <meta> tag whose
// name or property attribute matches the given value (case-insensitive).
func extractMetaTag(body []byte, attr string) string {
	re := regexp.MustCompile(`(?i)<meta[^>]+(?:name|property)=["']` + regexp.QuoteMeta(attr) + `["'][^>]*>`)
	tag := re.Find(body)
	if tag == nil {
		return ""
	}
	m := reMetaContent.FindSubmatch(tag)
	if m == nil {
		return ""
	}
	return string(m[1])
}

// flexString decodes any of the string-shaped variants Meta emits in
// schema.org address fields:
//   - plain string                 ("USA")
//   - object with .name            ({"@type":"Country","name":"USA"})
//   - array of any of the above    (["USA","USA",…])
//   - object whose .name is itself a list (real Meta case)
//
// Recurses through every container until it lands on plain strings, then
// joins deduped values with ", ".
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	// Try plain string first — overwhelmingly the common case.
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	// Object with a "name" field — recurse into name.
	var d map[string]json.RawMessage
	if err := json.Unmarshal(b, &d); err == nil {
		if name, ok := d["name"]; ok {
			var inner flexString
			if err := inner.UnmarshalJSON(name); err == nil {
				*f = inner
				return nil
			}
		}
		*f = ""
		return nil
	}
	// Array — recurse on each element, dedupe.
	var arr []json.RawMessage
	if err := json.Unmarshal(b, &arr); err == nil {
		seen := map[string]struct{}{}
		var parts []string
		for _, item := range arr {
			var inner flexString
			if err := inner.UnmarshalJSON(item); err == nil {
				s := string(inner)
				if s == "" {
					continue
				}
				if _, dup := seen[s]; dup {
					continue
				}
				seen[s] = struct{}{}
				parts = append(parts, s)
			}
		}
		*f = flexString(strings.Join(parts, ", "))
		return nil
	}
	// Fall through silently — unknown shape becomes empty.
	*f = ""
	return nil
}

func (f flexString) String() string { return string(f) }

// jobLocations handles JSON-LD's quirk: `jobLocation` may be a single object
// or an array of them. Normalises to an array internally.
type jobLocations []jobLocation

func (jl *jobLocations) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*jl = nil
		return nil
	}
	if b[0] == '[' {
		var arr []jobLocation
		if err := json.Unmarshal(b, &arr); err != nil {
			return err
		}
		*jl = arr
		return nil
	}
	var single jobLocation
	if err := json.Unmarshal(b, &single); err != nil {
		return err
	}
	*jl = []jobLocation{single}
	return nil
}

type jobLocation struct {
	Type    string  `json:"@type,omitempty"`
	Address address `json:"address"`
	Name    string  `json:"name,omitempty"`
}

type address struct {
	Locality flexString `json:"addressLocality,omitempty"`
	Region   flexString `json:"addressRegion,omitempty"`
	Country  flexString `json:"addressCountry,omitempty"`
}

type hiringOrganization struct {
	Type string `json:"@type,omitempty"`
	Name string `json:"name,omitempty"`
	Logo string `json:"logo,omitempty"`
	URL  string `json:"sameAs,omitempty"`
}

// JobPosting mirrors the schema.org JobPosting payload Meta embeds in every
// detail page. Required fields (verified 11/11 across a diverse sample) are
// plain Go types. Optional fields use flexString or RawMessage so unexpected
// shapes don't break decoding.
type JobPosting struct {
	Context          string             `json:"@context,omitempty"`
	Type             string             `json:"@type,omitempty"`
	Title            string             `json:"title,omitempty"`
	Description      string             `json:"description,omitempty"`
	Responsibilities string             `json:"responsibilities,omitempty"`
	Qualifications   string             `json:"qualifications,omitempty"`
	EmploymentType   string             `json:"employmentType,omitempty"`
	DatePosted       string             `json:"datePosted,omitempty"`
	ValidThrough     string             `json:"validThrough,omitempty"`
	DirectApply      bool               `json:"directApply,omitempty"`
	HiringOrg        hiringOrganization `json:"hiringOrganization,omitempty"`
	JobLocation      jobLocations       `json:"jobLocation,omitempty"`

	// Optional fields — none observed in the 11-job sample but kept for
	// completeness. Decoded into RawMessage so unexpected nested shapes
	// never break the parse.
	Industry             flexString      `json:"industry,omitempty"`
	OccupationalCategory flexString      `json:"occupationalCategory,omitempty"`
	BaseSalary           json.RawMessage `json:"baseSalary,omitempty"`
	Skills               flexString      `json:"skills,omitempty"`
	JobBenefits          flexString      `json:"jobBenefits,omitempty"`
}

// extractJobPosting pulls the first <script type="application/ld+json"> block
// whose @type is "JobPosting" out of a Meta detail-page HTML body.
//
// Meta typically embeds exactly one JSON-LD block per detail page; we still
// scan all of them and return the first JobPosting in case Meta later adds
// other structured-data scripts (BreadcrumbList, etc.).
func extractJobPosting(body []byte) (*JobPosting, error) {
	matches := reJSONLD.FindAllSubmatch(body, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("meta/http: no application/ld+json script block")
	}
	for _, m := range matches {
		// schema.org JSON-LD can be a single object, a list of objects, or
		// wrapped in {"@graph": [...]}. Try the simple cases in order.
		raw := bytes.TrimSpace(m[1])

		// Plain object.
		var single JobPosting
		if err := json.Unmarshal(raw, &single); err == nil && single.Type == "JobPosting" {
			return &single, nil
		}

		// Array of objects.
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err == nil {
			for _, item := range arr {
				var jp JobPosting
				if err := json.Unmarshal(item, &jp); err == nil && jp.Type == "JobPosting" {
					return &jp, nil
				}
			}
		}

		// Object with @graph.
		var wrapper struct {
			Graph []json.RawMessage `json:"@graph"`
		}
		if err := json.Unmarshal(raw, &wrapper); err == nil {
			for _, item := range wrapper.Graph {
				var jp JobPosting
				if err := json.Unmarshal(item, &jp); err == nil && jp.Type == "JobPosting" {
					return &jp, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("meta/http: no JobPosting found in JSON-LD blocks")
}

// Locations returns the deduped list of "City, Region, Country" strings
// for this posting. Multiple jobLocation entries that resolve to the same
// joined string collapse to one — Meta sometimes emits per-building copies.
func (jp *JobPosting) Locations() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, l := range jp.JobLocation {
		var parts []string
		if s := strings.TrimSpace(string(l.Address.Locality)); s != "" {
			parts = append(parts, s)
		}
		if s := strings.TrimSpace(string(l.Address.Region)); s != "" {
			parts = append(parts, s)
		}
		if s := strings.TrimSpace(string(l.Address.Country)); s != "" {
			parts = append(parts, s)
		}
		joined := strings.Join(parts, ", ")
		if joined == "" {
			continue
		}
		if _, dup := seen[joined]; dup {
			continue
		}
		seen[joined] = struct{}{}
		out = append(out, joined)
	}
	return out
}

// Compose builds the canonical multi-section description string the
// downstream LLM enrichment expects. Mirrors the format produced by the
// Google adapter and the original rod-based extractJobDetail:
//
//	<title>
//	<location line>
//	<metadata block>
//	Description / Responsibilities / Qualifications sections
func (jp *JobPosting) Compose() string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(jp.Title))
	b.WriteString("\n\n")

	locs := jp.Locations()
	if len(locs) > 0 {
		const cap = 5
		shown := locs
		if len(locs) > cap {
			shown = locs[:cap]
		}
		b.WriteString(strings.Join(shown, "; "))
		if len(locs) > cap {
			fmt.Fprintf(&b, " (+%d more)", len(locs)-cap)
		}
	}
	b.WriteString("\n\n")

	// Metadata block — one signal per line; downstream LLM ingests it as-is.
	wroteMeta := false
	writeMeta := func(label, value string) {
		if value == "" {
			return
		}
		fmt.Fprintf(&b, "%s: %s\n", label, value)
		wroteMeta = true
	}
	writeMeta("Organization", jp.HiringOrg.Name)
	writeMeta("Employment type", jp.EmploymentType)
	writeMeta("Industry", string(jp.Industry))
	writeMeta("Occupational category", string(jp.OccupationalCategory))
	writeMeta("Posted", jp.DatePosted)
	writeMeta("Valid through", jp.ValidThrough)
	if wroteMeta {
		b.WriteString("\n")
	}

	writeSection := func(label, htmlText string) {
		clean := types.StripHTML(htmlText)
		if clean == "" {
			return
		}
		fmt.Fprintf(&b, "%s\n%s\n\n", label, clean)
	}
	writeSection("Description", jp.Description)
	writeSection("Responsibilities", jp.Responsibilities)
	writeSection("Qualifications", jp.Qualifications)

	return strings.TrimSpace(b.String())
}

// metaCountryToCode converts a Meta address country value to an ISO alpha-2
// code. Meta emits "USA", "United States", plain 2-char codes (e.g. "IN"),
// or empty strings. Unknown values return "".
func metaCountryToCode(country string) string {
	c := strings.TrimSpace(country)
	switch strings.ToUpper(c) {
	case "USA", "UNITED STATES", "UNITED STATES OF AMERICA", "US":
		return "US"
	case "":
		return ""
	default:
		// Pass through if already a 2-char ISO code; ignore everything else.
		if len(c) == 2 {
			return strings.ToUpper(c)
		}
		return ""
	}
}

// toFetchJobContentResponse maps the parsed JobPosting onto the shared
// FetchJobContentResponse contract every adapter implements. Mirrors the
// Greenhouse / Lever toFetchJobContentResponse helpers.
func (jp *JobPosting) toFetchJobContentResponse(jobID string) types.FetchJobContentResponse {
	// Prefer the full "City, Region, Country" string from Locations() — it is
	// the most human-readable and what the test contract requires. Fall back to
	// the raw Locality field only when Locations() is empty (e.g. no address).
	location := ""
	if allLocs := jp.Locations(); len(allLocs) > 0 {
		location = allLocs[0]
	} else if locs := jp.JobLocation; len(locs) > 0 {
		location = strings.TrimSpace(string(locs[0].Address.Locality))
	}

	// Walk all locations to find the first non-empty country code.
	countryCode := ""
	for _, l := range jp.JobLocation {
		if code := metaCountryToCode(strings.TrimSpace(string(l.Address.Country))); code != "" {
			countryCode = code
			break
		}
	}
	return types.FetchJobContentResponse{
		ID:             jobID,
		Title:          strings.TrimSpace(jp.Title),
		Location:       location,
		URL:            "",
		Description:    jp.Compose(),
		EmploymentType: jobcommon.EmployFullTime, // Meta lists full-time only on this filter
		Department:     jp.HiringOrg.Name,
		PublishedDate:  jp.DatePosted,
		CountryCode:    countryCode,
	}
}

// fetchAndParseDetail performs the HTTP GET against the canonical detail URL
// and returns the parsed JobPosting. Pulled out so both FetchJobContent and
// the smoke-test cmd can reuse the same network path.
func (m *MetaAdapter) fetchAndParseDetail(ctx context.Context, url string) (*JobPosting, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("meta/http: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("Accept-Language", acceptLangHeader)
	// Mirror the browser-set Sec-Fetch headers Meta validates against.
	// A bare urllib request (UA + Accept + Accept-Language only) gets a 400.
	req.Header.Set("sec-ch-ua", `"Chromium";v="121", "Not A(Brand";v="99", "Google Chrome";v="121"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta/http: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("meta/http: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("meta/http: read body: %w", err)
	}
	jp, err := extractJobPosting(body)
	if err != nil {
		// Some Meta pages are fully client-side rendered and omit JSON-LD from
		// the server HTML. Fall back to og:title + og:description so the
		// enricher can still run role_match against whatever content is available.
		title := extractMetaTag(body, "og:title")
		desc := extractMetaTag(body, "og:description")
		if title == "" {
			title = extractMetaTag(body, "title")
		}
		if title != "" {
			return &JobPosting{Title: title, Description: desc}, nil
		}
		return nil, err
	}
	return jp, nil
}

// detailURLForJobID builds the canonical detail URL.
func detailURLForJobID(numericID string) string {
	return detailURLPrefix + numericID
}

// stripMetaPrefix returns the numeric job id from a "meta-<id>" string.
func stripMetaPrefix(id string) string {
	prefix := companycommon.AdapterMeta + "-"
	if strings.HasPrefix(id, prefix) {
		return strings.TrimPrefix(id, prefix)
	}
	return id
}
