package eightfold

import (
	"fmt"
	"strings"
	"time"

	"github.com/trushilpatel/jobradar/jobingest/adapters/ats"
	efapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/eightfold/api"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

// mapJobSummary converts a Position from the listing response to a core.JobSummary.
// host is the configured Eightfold customer host (used for the fallback URL when
// canonicalPositionUrl is absent).
func mapJobSummary(p efapi.Position, host string) core.JobSummary {
	loc := resolveLocation(p.Location, p.Locations)
	allLocs := resolveAllLocations(p.Location, p.Locations)
	return core.JobSummary{
		ID:            fmt.Sprintf("eightfold-%d", p.ID),
		Title:         p.Name,
		Location:      loc,
		URL:           resolveURL(p.CanonicalURL, p.ID, host),
		EmployType:    core.EmployFullTime,
		LocationKnown: true,
		AllLocations:  allLocs,
	}
}

// mapJobPost converts a Position from the detail response to a core.JobPost.
func mapJobPost(p efapi.Position) core.JobPost {
	return core.JobPost{
		ID:            fmt.Sprintf("eightfold-%d", p.ID),
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

// resolveAllLocations returns a slice containing all distinct locations.
func resolveAllLocations(location string, locations []string) []string {
	if location != "" {
		return []string{location}
	}
	return locations
}

// resolveURL returns canonicalURL when non-empty; otherwise builds a fallback
// URL from the configured host and the Eightfold position ID.
func resolveURL(canonicalURL string, id int64, host string) string {
	if canonicalURL != "" {
		return canonicalURL
	}
	return fmt.Sprintf("https://%s/careers/job/%d", host, id)
}

// unixToRFC3339 converts a Unix timestamp (seconds) to an RFC 3339 string in
// UTC. Returns empty string for zero or negative values.
func unixToRFC3339(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}
