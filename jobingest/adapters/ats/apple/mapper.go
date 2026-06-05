package apple

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/trushilpatel/jobradar/jobingest/adapters/ats"
	appleapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/apple/api"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

const usaCountryID = "iso-country-USA"

// payRangeRe captures "$N,NNN and $N,NNN" or "$N,NNN to $N,NNN" style ranges
// out of the job-detail postingFooters HTML blob.
var payRangeRe = regexp.MustCompile(`\$\d{1,3}(?:,\d{3})*(?:\.\d+)?\s*(?:and|to|–|—|-)\s*\$\d{1,3}(?:,\d{3})*(?:\.\d+)?`)

// ── Listing mapper ────────────────────────────────────────────────────────────

// mapJobSummary converts a listing Job to a core.JobSummary.
func mapJobSummary(job appleapi.Job, baseURL string) core.JobSummary {
	return core.JobSummary{
		ID:            fmt.Sprintf("apple-%s", job.ID),
		Title:         job.PostingTitle,
		Location:      firstUSLocation(job.Locations),
		URL:           fmt.Sprintf("%s/en-us/details/%s/%s", baseURL, job.ID, job.TransformedPostingTitle),
		EmployType:    normalizeAppleWorkHours(job.WorkHours),
		LocationKnown: hasUSALocation(job.Locations),
		AllLocations:  allLocationStrings(job.Locations),
	}
}

// ── Detail mapper ─────────────────────────────────────────────────────────────

// mapJobPost converts a detail JobDetail to a core.JobPost.
// Uses jd.ID (the listing id field) to stay consistent with mapJobSummary.
func mapJobPost(jd appleapi.JobDetail) core.JobPost {
	return core.JobPost{
		ID:           fmt.Sprintf("apple-%s", jd.ID),
		Title:        jd.PostingTitle,
		Location:     firstUSLocation(jd.Locations),
		Description:  buildDescription(jd),
		Compensation: extractCompensation(jd.PostingFooters),
		CountryCode:  primaryCountryCode(jd.Locations),
	}
}

// ── Description builder ───────────────────────────────────────────────────────

// buildDescription concatenates structured detail-page sections with labels,
// separated by blank lines. Empty sections are skipped.
func buildDescription(jd appleapi.JobDetail) string {
	type section struct {
		label string
		body  string
	}
	sections := []section{
		{"Summary", ats.StripHTML(jd.JobSummary)},
		{"Description", ats.StripHTML(jd.BoardDescription)},
		{"Responsibilities", ats.StripHTML(jd.Responsibilities)},
		{"Requirements", ats.StripHTML(jd.MinimumQualifications)},
		{"Preferred", ats.StripHTML(jd.PreferredQualifications)},
	}
	parts := make([]string, 0, len(sections))
	for _, s := range sections {
		if s.body == "" {
			continue
		}
		parts = append(parts, s.label+":\n"+s.body)
	}
	return strings.Join(parts, "\n\n")
}

// ── Compensation extractor ────────────────────────────────────────────────────

// extractCompensation pulls the first pay range out of any postingFooters content.
func extractCompensation(footers []appleapi.Footer) string {
	for _, f := range footers {
		text := ats.StripHTML(f.Content)
		if match := payRangeRe.FindString(text); match != "" {
			return strings.Join(strings.Fields(match), " ")
		}
	}
	return ""
}

// ── Location helpers ──────────────────────────────────────────────────────────

// firstUSLocation returns the formatted string for the first US location, or
// the first location when no US location is present.
func firstUSLocation(locations []appleapi.Location) string {
	for _, loc := range locations {
		if isUSALocation(loc) {
			return formatSingleLocation(loc)
		}
	}
	if len(locations) > 0 {
		return formatSingleLocation(locations[0])
	}
	return ""
}

// allLocationStrings returns one formatted string per location.
func allLocationStrings(locations []appleapi.Location) []string {
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

// formatSingleLocation renders a Location as "City, State".
// Falls back to Name when City is empty; falls back to StateCode when
// StateProvince is empty.
func formatSingleLocation(loc appleapi.Location) string {
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

// hasUSALocation returns true when at least one location is tagged with the
// USA country marker.
func hasUSALocation(locations []appleapi.Location) bool {
	return slices.ContainsFunc(locations, isUSALocation)
}

// isUSALocation reports whether a location belongs to the United States,
// tolerating the listing's countryID or the older countryCode/Name shapes.
func isUSALocation(loc appleapi.Location) bool {
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

// primaryCountryCode returns "US" for any USA location, or the first non-empty
// country code found.
func primaryCountryCode(locations []appleapi.Location) string {
	for _, loc := range locations {
		if isUSALocation(loc) {
			return "US"
		}
		if loc.CountryCode != "" {
			return strings.ToUpper(loc.CountryCode)
		}
		if loc.CountryID != "" && strings.HasPrefix(loc.CountryID, "iso-country-") {
			return strings.TrimPrefix(loc.CountryID, "iso-country-")
		}
	}
	return ""
}

// ── Employment type mapping ───────────────────────────────────────────────────

// normalizeAppleWorkHours converts Apple's workHours string to a core employ
// type constant. Anything not explicitly part_time or internship maps to full_time.
func normalizeAppleWorkHours(workHours string) string {
	switch strings.ToLower(strings.TrimSpace(workHours)) {
	case "part_time", "part-time", "parttime":
		return core.EmployPartTime
	case "internship", "intern":
		return core.EmployInternship
	default:
		return core.EmployFullTime
	}
}

// ── Utility ───────────────────────────────────────────────────────────────────

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
