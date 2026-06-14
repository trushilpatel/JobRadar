package uber

import (
	"fmt"
	"strings"

	uberapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/uber/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

// mapJobSummary converts a single UberJob (from the search-results API) into
// a core.JobSummary. JobContent is always set — this is a fat adapter.
func mapJobSummary(job uberapi.UberJob, baseURL string) core.JobSummary {
	loc := buildPrimaryLocation(job)
	allLocs := buildAllLocations(job)

	return core.JobSummary{
		ID:            fmt.Sprintf("uber-%d", job.ID),
		Title:         job.Title,
		Location:      loc,
		URL:           fmt.Sprintf("%s/careers/list/%d", baseURL, job.ID),
		EmployType:    normalizeUberTimeType(job.TimeType),
		LocationKnown: true,
		AllLocations:  allLocs,
		JobContent: &core.JobContent{
			Description:   job.Description,
			Department:    job.Department,
			PublishedDate: job.CreationDate,
		},
	}
}

// buildPrimaryLocation produces a human-readable location string for the primary
// location of a job. Strategy:
//   - If allLocations has multiple entries, join them with "; "
//   - If allLocations has exactly one entry, use that
//   - Fall back to job.Location (primary location field)
func buildPrimaryLocation(job uberapi.UberJob) string {
	if len(job.AllLocations) > 1 {
		parts := make([]string, 0, len(job.AllLocations))
		for _, loc := range job.AllLocations {
			if s := formatSingleLocation(loc); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "; ")
	}
	if len(job.AllLocations) == 1 {
		return formatSingleLocation(job.AllLocations[0])
	}
	return formatSingleLocation(job.Location)
}

// buildAllLocations returns a slice of location strings — one per entry in
// job.AllLocations. Falls back to a single-element slice of the primary location.
func buildAllLocations(job uberapi.UberJob) []string {
	if len(job.AllLocations) == 0 {
		s := formatSingleLocation(job.Location)
		if s == "" {
			return nil
		}
		return []string{s}
	}
	result := make([]string, 0, len(job.AllLocations))
	for _, loc := range job.AllLocations {
		if s := formatSingleLocation(loc); s != "" {
			result = append(result, s)
		}
	}
	return result
}

// formatSingleLocation renders an UberLocation as "City, Region, CountryName",
// skipping empty segments. Falls back to Country (ISO alpha-3) if CountryName
// is also empty.
func formatSingleLocation(loc uberapi.UberLocation) string {
	if loc.City != "" {
		parts := []string{loc.City}
		if loc.Region != "" {
			parts = append(parts, loc.Region)
		}
		if loc.CountryName != "" {
			parts = append(parts, loc.CountryName)
		} else if loc.Country != "" {
			parts = append(parts, loc.Country)
		}
		return strings.Join(parts, ", ")
	}
	if loc.CountryName != "" {
		return loc.CountryName
	}
	return loc.Country
}

// normalizeUberTimeType maps Uber's TimeType string to a core employment-type
// constant. Comparison is case-insensitive; unknown values default to full_time.
func normalizeUberTimeType(tt string) string {
	switch strings.ToLower(strings.TrimSpace(tt)) {
	case "full-time":
		return core.EmployFullTime
	case "part-time":
		return core.EmployPartTime
	case "contract":
		return core.EmployContract
	case "internship":
		return core.EmployInternship
	default:
		return core.EmployFullTime
	}
}
