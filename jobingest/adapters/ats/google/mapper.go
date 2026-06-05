package google

import (
	"github.com/trushilpatel/jobradar/jobingest/adapters/ats"
	googleapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/google/api"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

// mapParsedJobToSummary converts a parsed listing card to a core.JobSummary.
// EmployType is always full_time — the listing URL filters for FULL_TIME only.
// LocationKnown is true — the listing page reliably provides a location for every card.
func mapParsedJobToSummary(p googleapi.ParsedJob) core.JobSummary {
	return core.JobSummary{
		ID:            "google-" + p.ID,
		Title:         p.Title,
		Location:      p.Location,
		URL:           p.URL,
		EmployType:    core.EmployFullTime,
		LocationKnown: true,
	}
}

// mapDetailToJobPost converts a detail page's extracted fields to a core.JobPost.
// Falls back to summary fields when detail fields are absent.
func mapDetailToJobPost(summary core.JobSummary, detail googleapi.JobDetail) core.JobPost {
	title := detail.Title
	if title == "" {
		title = summary.Title
	}
	location := detail.Location
	if location == "" {
		location = summary.Location
	}
	return core.JobPost{
		ID:            summary.ID,
		Title:         title,
		Location:      location,
		URL:           summary.URL,
		Description:   ats.StripHTML(detail.Description),
		Department:    detail.Department,
		PublishedDate: detail.PostedDate,
	}
}
