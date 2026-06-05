package netflix

import (
	"fmt"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/jobingest/adapters/ats"
	nfapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/netflix/api"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

// mapJobSummary converts a Position from the listing response to a core.JobSummary.
func mapJobSummary(p nfapi.Position) core.JobSummary {
	loc := resolveLocation(p.Location, p.Locations)
	allLocs := resolveAllLocations(p.Location, p.Locations)
	return core.JobSummary{
		ID:            fmt.Sprintf("netflix-%d", p.ID),
		Title:         p.Name,
		Location:      loc,
		URL:           resolveURL(p.CanonicalURL, p.ID),
		EmployType:    core.EmployFullTime,
		LocationKnown: true,
		AllLocations:  allLocs,
	}
}

// mapJobPost converts a Position from the detail response to a core.JobPost.
func mapJobPost(p nfapi.Position) core.JobPost {
	return core.JobPost{
		ID:            fmt.Sprintf("netflix-%d", p.ID),
		Title:         p.Name,
		Location:      resolveLocation(p.Location, p.Locations),
		URL:           resolveURL(p.CanonicalURL, p.ID),
		Description:   ats.StripHTML(p.JobDescription),
		Department:    p.Department,
		WorkplaceType: p.WorkLocationOption,
		PublishedDate: unixToRFC3339(p.TCreate),
	}
}

// resolveLocation returns Location if non-empty, otherwise joins Locations with "; ".
func resolveLocation(location string, locations []string) string {
	if location != "" {
		return location
	}
	return strings.Join(locations, "; ")
}

// resolveAllLocations returns a slice with all distinct locations.
func resolveAllLocations(location string, locations []string) []string {
	if location != "" {
		return []string{location}
	}
	return locations
}

// resolveURL returns canonicalURL if non-empty, otherwise constructs a fallback URL.
func resolveURL(canonicalURL string, id int64) string {
	if canonicalURL != "" {
		return canonicalURL
	}
	return fmt.Sprintf("%s/careers/job/%d", defaultBaseURL, id)
}

// unixToRFC3339 converts a Unix timestamp (seconds) to an RFC 3339 string.
// Returns empty string if ts is zero.
func unixToRFC3339(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}
