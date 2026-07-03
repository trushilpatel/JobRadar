package workday

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats"
	wdapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/workday/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

// hostPattern matches a canonical Workday tenant host such as
// `nvidia.wd5.myworkdayjobs.com` or `polar-semi.wd501.myworkdayjobs.com`.
var hostPattern = regexp.MustCompile(`^[a-z0-9-]+\.wd[0-9]+\.myworkdayjobs\.com$`)

// ── adapterConfig ─────────────────────────────────────────────────────────────

// adapterConfig is the per-company JSON blob stored in the adapter config
// and supplied to FetchJobListingWithConfig.
type adapterConfig struct {
	Host           string `json:"host"`                        // e.g. "nvidia.wd5.myworkdayjobs.com"
	Site           string `json:"site"`                        // e.g. "NVIDIAExternalCareerSite"
	Locale         string `json:"locale,omitempty"`            // optional, e.g. "en-US"
	MaxJobsPerScan int    `json:"max_jobs_per_scan,omitempty"` // optional cap; 0 → default
}

// validate enforces the host/site invariants. Returns a wrapped error suitable
// for surfacing through the engine.
func (c adapterConfig) validate() error {
	host := strings.TrimSpace(c.Host)
	if host == "" {
		return errors.New("host is required")
	}
	if !hostPattern.MatchString(host) {
		return fmt.Errorf("host %q does not match pattern <tenant>.wd<N>.myworkdayjobs.com", host)
	}
	site := strings.TrimSpace(c.Site)
	if site == "" {
		return errors.New("site is required")
	}
	if strings.ContainsRune(site, '/') {
		return fmt.Errorf("site %q must not contain '/'", site)
	}
	return nil
}

// tenant extracts the leftmost subdomain of host — e.g.
// "nvidia.wd5.myworkdayjobs.com" → "nvidia".
func (c adapterConfig) tenant() string {
	host := strings.TrimSpace(c.Host)
	idx := strings.Index(host, ".")
	if idx <= 0 {
		return ""
	}
	return host[:idx]
}

// ── Mappers ───────────────────────────────────────────────────────────────────

// mapJobSummary converts one CXS listing row into a core.JobSummary.
// The CXS listing carries minimal data; EmployType defaults to full_time and
// JobContent is left nil — the enricher fetches the rest via FetchJobPost.
//
// LocationKnown is intentionally left false (the zero value): the CXS listing
// exposes only a coarse LocationsText with no structured country, so it cannot
// be trusted for filter-stage string-based non-USA rejection. The real location
// decision is deferred to enrich-time, where FetchJobPost supplies a reliable
// CountryCode. This matches v1 parity (jobToItem never set LocationKnown).
func mapJobSummary(jp wdapi.JobPosting, cfg adapterConfig) core.JobSummary {
	reqID := extractRequisitionID(jp)
	return core.JobSummary{
		ID:         fmt.Sprintf("workday-%s", reqID),
		Title:      jp.Title,
		Location:   jp.LocationsText,
		URL:        fmt.Sprintf("https://%s/%s%s", cfg.Host, cfg.Site, jp.ExternalPath),
		EmployType: core.EmployFullTime,
	}
}

// mapJobPost converts a CXS detail response into a core.JobPost.
// jobURL is the original summary URL used for reconstruction context.
func mapJobPost(info wdapi.JobPostingInfo, jobURL string) core.JobPost {
	// PublishedDate prefers `startDate` (ISO yyyy-mm-dd) over `postedOn`
	// (display string like "Posted Today") so downstream enrichment has a real
	// date to anchor recency on.
	publishedDate := info.StartDate
	if publishedDate == "" {
		publishedDate = info.PostedOn
	}

	return core.JobPost{
		ID:             buildID(info.JobReqID, info.ExternalURL),
		Title:          info.Title,
		Location:       info.Location,
		URL:            jobURL,
		Description:    ats.StripHTML(info.JobDescription),
		EmploymentType: mapEmploymentType(info.TimeType),
		PublishedDate:  publishedDate,
		CountryCode:    extractCountryCode(info.Country),
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// extractRequisitionID resolves a stable identifier for a posting. Prefers
// bulletFields[0] (which carries the requisition number in every tenant we
// have seen), falls back to the trailing `_<id>` suffix on externalPath, and
// finally to externalPath itself.
func extractRequisitionID(jp wdapi.JobPosting) string {
	for _, b := range jp.BulletFields {
		if s := strings.TrimSpace(b); s != "" {
			return s
		}
	}
	path := strings.TrimSpace(jp.ExternalPath)
	if idx := strings.LastIndex(path, "_"); idx >= 0 && idx+1 < len(path) {
		return path[idx+1:]
	}
	if path != "" {
		return path
	}
	return "unknown"
}

// tenantFromHost returns the leftmost subdomain of a Workday host. Returns
// empty string on a malformed host.
func tenantFromHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	idx := strings.Index(host, ".")
	if idx <= 0 {
		return ""
	}
	return host[:idx]
}

// parseJobURL extracts (host, site, externalPath) from a Workday job page URL
// of the form `https://<host>/<site>/job<externalPath>`.
// externalPath begins with `/job/…` — concatenate directly to the API base.
func parseJobURL(raw string) (host, site, externalPath string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "", fmt.Errorf("parse url: %w", err)
	}
	if u.Host == "" || !hostPattern.MatchString(u.Host) {
		return "", "", "", fmt.Errorf("host %q is not a Workday tenant", u.Host)
	}
	path := strings.TrimPrefix(u.Path, "/")
	idx := strings.Index(path, "/job/")
	if idx < 0 {
		return "", "", "", fmt.Errorf("path %q missing `/job/` segment", u.Path)
	}
	site = path[:idx]
	if site == "" || strings.ContainsRune(site, '/') {
		return "", "", "", fmt.Errorf("site segment %q invalid", site)
	}
	externalPath = path[idx:]
	return u.Host, site, externalPath, nil
}

// buildID composes the global job ID. Prefers an explicit requisition ID;
// falls back to the trailing path segment.
func buildID(requisitionID, externalURL string) string {
	id := strings.TrimSpace(requisitionID)
	if id == "" {
		// Try to extract from externalURL path trailing segment
		if u, err := url.Parse(externalURL); err == nil {
			path := u.Path
			if idx := strings.LastIndex(path, "_"); idx >= 0 && idx+1 < len(path) {
				id = path[idx+1:]
			}
		}
	}
	if id == "" {
		id = "unknown"
	}
	return fmt.Sprintf("workday-%s", id)
}

// usDescriptors is the set of Workday `country.descriptor` values that map to
// the ISO alpha-2 code "US". Keys are pre-normalized (lower-cased, trimmed) so
// matching is done against normalize(descriptor). A small explicit set is used
// rather than substring matching so the accepted values are exact and testable.
var usDescriptors = map[string]struct{}{
	"united states of america": {},
	"united states":            {},
	"usa":                      {},
	"u.s.":                     {},
	"us":                       {},
}

// extractCountryCode derives the ISO alpha-2 country code from Workday's
// `country` detail field. The field is a JSON object shaped like:
//
//	{"descriptor": "United States of America", "id": "bc33aa3152ec42d4995f4791a106ed09"}
//
// IMPORTANT: `country.id` is a tenant-internal Workday ID (WID) — an opaque
// 32-hex hash unique to each tenant — NOT an ISO country code. It MUST NEVER be
// used as a country code. Only `descriptor` carries the human-readable country
// name, which we normalize and match against the US descriptor set.
//
// A US descriptor returns "US". Any non-US descriptor returns "" (empty) rather
// than guessing a foreign ISO code — the analyzer's string-based location
// classifier owns the non-US decision via post.Location. An unset, null,
// malformed, or empty-object field also returns "".
func extractCountryCode(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var obj struct {
		Descriptor string `json:"descriptor"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	normalized := strings.ToLower(strings.TrimSpace(obj.Descriptor))
	if _, ok := usDescriptors[normalized]; ok {
		return "US"
	}
	return ""
}

// mapEmploymentType normalises Workday's `timeType` strings ("Full time",
// "Part time", "Contract", "Intern") onto the core constants.
// Unknown values default to full_time, consistent with every other adapter.
func mapEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(strings.ReplaceAll(t, "-", " "))) {
	case "full time", "full_time", "fulltime", "regular":
		return core.EmployFullTime
	case "part time", "part_time", "parttime":
		return core.EmployPartTime
	case "contract", "contractor", "consultant", "fixed term":
		return core.EmployContract
	case "intern", "internship":
		return core.EmployInternship
	default:
		return core.EmployFullTime
	}
}
