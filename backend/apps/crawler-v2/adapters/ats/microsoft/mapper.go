package microsoft

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats"
	msapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/microsoft/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

// msReqIDSuffix matches Microsoft internal req-ID suffixes appended to job
// titles (e.g. "Software Engineer - 17248K", "Principal Engineer - MS-12345").
// Pattern: optional whitespace, a dash separator, then an alphanumeric token
// that contains at least one digit, at the very end of the string.
var msReqIDSuffix = regexp.MustCompile(`\s+-\s+[A-Z0-9-]*\d[A-Z0-9-]*\s*$`)

// stripReqID removes Microsoft req-ID suffixes from job title strings.
// "Software Engineer - 17248K" → "Software Engineer"
// Titles without a recognisable suffix are returned unchanged.
func stripReqID(title string) string {
	return strings.TrimSpace(msReqIDSuffix.ReplaceAllString(title, ""))
}

// extractPositionID extracts the int64 position ID from a URL like
// "https://apply.careers.microsoft.com/careers/job/1812345678".
// Returns 0 if the last path segment is not a valid int64.
func extractPositionID(rawURL string) int64 {
	parts := strings.Split(strings.TrimRight(rawURL, "/"), "/")
	if len(parts) == 0 {
		return 0
	}
	idStr := parts[len(parts)-1]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// unixToRFC3339 converts a Unix timestamp (seconds) to an RFC 3339 string.
// Returns empty string if ts is zero.
func unixToRFC3339(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

// mapEmploymentType maps Microsoft's efcustomTextEmploymentType values to
// core employment constants.
func mapEmploymentType(empTypes []string) string {
	if len(empTypes) == 0 {
		return core.EmployFullTime
	}
	switch strings.TrimSpace(empTypes[0]) {
	case "Full-Time":
		return core.EmployFullTime
	case "Part-Time":
		return core.EmployPartTime
	case "Contract":
		return core.EmployContract
	case "Intern", "Internship":
		return core.EmployInternship
	default:
		return core.EmployFullTime
	}
}

// mapJobSummary maps a Microsoft API position to a core.JobSummary.
func mapJobSummary(p msapi.Position) core.JobSummary {
	loc := strings.Join(p.Locations, "; ")
	internalURL := fmt.Sprintf("https://apply.careers.microsoft.com/careers/job/%d", p.ID)
	return core.JobSummary{
		ID:            fmt.Sprintf("microsoft-%s", p.DisplayJobID),
		Title:         stripReqID(p.Name),
		Location:      loc,
		URL:           internalURL,
		EmployType:    core.EmployFullTime,
		LocationKnown: true,
		AllLocations:  p.Locations,
	}
}

// mapJobPost maps a Microsoft API positionDetail to a core.JobPost.
func mapJobPost(p msapi.PositionDetail) core.JobPost {
	loc := p.Location
	if loc == "" {
		loc = strings.Join(p.Locations, "; ")
	}

	// Use positionUrl from the detail response. The API returns relative paths
	// like "/careers/job/123" — make them absolute.
	jobURL := p.PositionURL
	if strings.HasPrefix(jobURL, "/") {
		jobURL = "https://apply.careers.microsoft.com" + jobURL
	}
	if jobURL == "" {
		jobURL = fmt.Sprintf("https://careers.microsoft.com/v2/global/en/job/%s", p.DisplayJobID)
	}

	return core.JobPost{
		ID:             fmt.Sprintf("microsoft-%s", p.DisplayJobID),
		Title:          stripReqID(p.Name),
		Location:       loc,
		URL:            jobURL,
		Description:    ats.StripHTML(p.JobDescription),
		Department:     p.Department,
		WorkplaceType:  p.WorkLocationOption,
		EmploymentType: mapEmploymentType(p.EfcustomTextEmploymentType),
		PublishedDate:  unixToRFC3339(p.PostedTs),
	}
}
