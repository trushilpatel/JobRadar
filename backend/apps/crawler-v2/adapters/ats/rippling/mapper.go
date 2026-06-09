package rippling

import (
	"fmt"
	"strings"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats"
	rapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/rippling/api"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

// mapJobListing converts listing items to a core.JobListing, filtering to US-only jobs.
func mapJobListing(items []rapi.ListingJob, atsBaseURL string) core.JobListing {
	jobs := make([]core.JobSummary, 0, len(items))
	seen := make(map[string]bool)

	for _, item := range items {
		if !hasUSLocation(item.Locations) {
			continue
		}
		id := "rippling-" + item.ID
		if seen[id] {
			continue
		}
		seen[id] = true
		jobs = append(jobs, mapJobSummary(item, atsBaseURL))
	}
	return core.JobListing{Jobs: jobs}
}

// mapJobSummary converts a single listing item to core.JobSummary.
// Caller must ensure item has at least one US location.
func mapJobSummary(item rapi.ListingJob, atsBaseURL string) core.JobSummary {
	jobURL := item.URL
	if jobURL == "" {
		jobURL = fmt.Sprintf("%s/rippling/jobs/%s", atsBaseURL, item.ID)
	}

	usLocations := usLocationDisplayNames(item.Locations)
	location := ""
	if len(usLocations) > 0 {
		location = usLocations[0]
	}

	return core.JobSummary{
		ID:            "rippling-" + item.ID,
		Adapter:       "rippling",
		Title:         item.Name,
		Location:      location,
		URL:           jobURL,
		EmployType:    core.EmployFullTime, // Rippling doesn't expose employment type in listing
		LocationKnown: true,
		AllLocations:  usLocations,
	}
}

// mapJobPost converts a detail item to core.JobPost.
func mapJobPost(detail rapi.DetailJob, postURL string) core.JobPost {
	// Description: try fields in priority order, then strip HTML.
	description := detail.DescriptionHTML
	if description == "" {
		description = detail.Description
	}
	if description == "" {
		description = detail.JobDescription
	}
	if description == "" {
		description = detail.Body
	}
	description = ats.StripHTML(description)

	// Compensation: try fields in priority order.
	compensation := detail.Compensation
	if compensation == "" {
		compensation = detail.CompensationRng
	}
	if compensation == "" {
		compensation = detail.Salary
	}

	// Workplace type: try fields in priority order.
	workplaceType := detail.WorkplaceType
	if workplaceType == "" {
		workplaceType = detail.WorkplaceTypAlt
	}

	// Department: try struct field then string field.
	department := detail.Department.Name
	if department == "" {
		department = detail.DepartmentName
	}

	// Location: build from all locations in the detail.
	location := formatAllLocations(detail.Locations)

	return core.JobPost{
		ID:            "rippling-" + detail.ID,
		Title:         detail.Title,
		Location:      location,
		URL:           postURL,
		Description:   description,
		Department:    department,
		Compensation:  compensation,
		WorkplaceType: workplaceType,
		PublishedDate: detail.CreatedAt,
	}
}

// ── Location helpers ──────────────────────────────────────────────────────────

// hasUSLocation reports whether any location has countryCode == "US".
func hasUSLocation(locations []rapi.Location) bool {
	for _, loc := range locations {
		if loc.CountryCode == "US" {
			return true
		}
	}
	return false
}

// usLocationDisplayNames returns the display name for each US location.
// Format: "City, StateCode". Falls back to Name if city/state are missing.
func usLocationDisplayNames(locations []rapi.Location) []string {
	var names []string
	for _, loc := range locations {
		if loc.CountryCode != "US" {
			continue
		}
		names = append(names, locationDisplayName(loc))
	}
	return names
}

// formatAllLocations builds a semicolon-joined location string from all locations.
func formatAllLocations(locations []rapi.Location) string {
	var parts []string
	for _, loc := range locations {
		parts = append(parts, locationDisplayName(loc))
	}
	return strings.Join(parts, "; ")
}

// locationDisplayName returns a human-readable display name for a location.
func locationDisplayName(loc rapi.Location) string {
	if loc.City != "" && loc.StateCode != "" {
		return fmt.Sprintf("%s, %s", loc.City, loc.StateCode)
	}
	if loc.City != "" {
		return loc.City
	}
	if loc.Name != "" {
		return loc.Name
	}
	return loc.Country
}
