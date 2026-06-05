package deel

import (
	"encoding/json"
	"fmt"
	"strings"

	ats "github.com/trushilpatel/jobradar/jobingest/adapters/ats"
	deelapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/deel/api"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

// mapJobSummary converts a RawJobPosting to a core.JobSummary.
// content is the already-resolved JobContent (nil for non-fat paths, always set here).
// slug is the Deel vanity slug used to construct the canonical job URL.
func mapJobSummary(r deelapi.RawJobPosting, slug string, content *core.JobContent) core.JobSummary {
	return core.JobSummary{
		ID:            "deel-" + r.ID,
		Title:         r.Title,
		Location:      formatLocations(r.Job.JobLocations),
		URL:           jobURL(r.ID, slug),
		EmployType:    normalizeDeelEmployment(firstEmploymentTypeName(r.Job.JobEmploymentTypes)),
		LocationKnown: true,
		JobContent:    content,
	}
}

// jobURL constructs the canonical public job detail URL.
// jobs.deel.com/{slug}/job-details/{id}
func jobURL(id, slug string) string {
	return fmt.Sprintf("%s/%s/job-details/%s", defaultBaseURL, slug, id)
}

// buildJobContent assembles the JobContent for a raw posting, resolving any
// RSC chunk references for the description.
func buildJobContent(r deelapi.RawJobPosting, chunks map[string]string) *core.JobContent {
	return &core.JobContent{
		Description:   resolveDescription(r.RichtextDescription, chunks),
		Department:    firstDepartmentName(r.Job.JobDepartments),
		PublishedDate: firstPublishedAt(r.JobPostingPubs),
	}
}

// ── Field helpers ─────────────────────────────────────────────────────────────

// formatLocations joins all location names with "; " — same convention used by
// multi-location postings across other adapters.
func formatLocations(locs []deelapi.RawJobLocation) string {
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

func firstEmploymentTypeName(et []deelapi.RawEmploymentType) string {
	if len(et) == 0 {
		return ""
	}
	return et[0].EmploymentType.Name
}

func firstDepartmentName(d []deelapi.RawJobDepartment) string {
	if len(d) == 0 {
		return ""
	}
	return d[0].Department.Name
}

func firstPublishedAt(pubs []deelapi.RawJobPostingPub) string {
	if len(pubs) == 0 {
		return ""
	}
	return pubs[0].CurrentState.CreatedAt
}

// formatCompensation renders Deel's structured pay range into a human-readable
// form consistent with other adapters. Returns empty string if no range present.
func formatCompensation(c *deelapi.RawCompensation) string {
	if c == nil || (c.MinAmount == 0 && c.MaxAmount == 0) {
		return ""
	}
	cur := c.CurrencyIsoCode
	if cur == "" {
		cur = "USD"
	}
	return fmt.Sprintf("%.0f – %.0f %s", c.MinAmount, c.MaxAmount, cur)
}

// resolveDescription dereferences richtextDescription against the RSC chunk
// map. The value is normally `"$<hexId>"` (an RSC chunk reference), but legacy
// records may carry the HTML inline. Null/missing → empty string.
func resolveDescription(raw json.RawMessage, chunks map[string]string) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	if strings.HasPrefix(s, "$") {
		id := strings.TrimPrefix(s, "$")
		if html, ok := chunks[id]; ok {
			return ats.StripHTML(html)
		}
		return ""
	}
	return ats.StripHTML(s)
}

// normalizeDeelEmployment maps Deel's free-form employment type names onto the
// internal core constants. Unknown values fall through to full_time, consistent
// with all other adapters.
func normalizeDeelEmployment(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "full-time", "full time", "full_time":
		return core.EmployFullTime
	case "part-time", "part time", "part_time":
		return core.EmployPartTime
	case "contract", "contractor", "consultant", "freelance", "freelancer":
		return core.EmployContract
	case "intern", "internship":
		return core.EmployInternship
	default:
		return core.EmployFullTime
	}
}
