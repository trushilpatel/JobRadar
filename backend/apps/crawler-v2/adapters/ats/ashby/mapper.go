package ashby

import (
	"fmt"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats"
	ashbyapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/ashby/api"
	"github.com/trushilpatel/jobradar/common/ats/core"
)

func mapJobListing(resp ashbyapi.ListingResponse, slug string) core.JobListing {
	postings := resp.Data.JobBoardWithTeams.JobPostings
	items := make([]core.JobSummary, 0, len(postings))
	for _, p := range postings {
		items = append(items, mapJobSummary(p, slug))
	}
	return core.JobListing{Jobs: items}
}

func mapJobSummary(p ashbyapi.JobPosting, slug string) core.JobSummary {
	location := formatLocation(p.LocationName, p.WorkplaceType)
	return core.JobSummary{
		ID:            "ashby-" + p.ID,
		Title:         p.Title,
		Location:      location,
		URL:           fmt.Sprintf("https://jobs.ashbyhq.com/%s/%s", slug, p.ID),
		EmployType:    normalizeAshbyEmploymentType(p.EmploymentType),
		LocationKnown: true,
		AllLocations:  []string{location},
	}
}

func mapJobPost(p ashbyapi.JobPostingDetail) core.JobPost {
	location := formatLocation(p.LocationName, p.WorkplaceType)
	return core.JobPost{
		ID:             "ashby-" + p.ID,
		Title:          p.Title,
		Location:       location,
		Description:    ats.StripHTML(p.DescriptionHtml),
		EmploymentType: normalizeAshbyEmploymentType(p.EmploymentType),
		Compensation:   p.CompensationTierSummary,
		WorkplaceType:  p.WorkplaceType,
		PublishedDate:  p.PublishedDate,
	}
}

// formatLocation builds the display location string.
// When workplaceType is non-empty it is appended in parentheses.
func formatLocation(locationName, workplaceType string) string {
	if workplaceType != "" {
		return fmt.Sprintf("%s (%s)", locationName, workplaceType)
	}
	return locationName
}

// normalizeAshbyEmploymentType maps Ashby's EmploymentType enum to the core constants.
func normalizeAshbyEmploymentType(t string) string {
	switch t {
	case "FullTime":
		return core.EmployFullTime
	case "PartTime":
		return core.EmployPartTime
	case "Contract", "Contractor", "Temporary":
		return core.EmployContract
	case "Intern", "Internship":
		return core.EmployInternship
	default:
		return ""
	}
}
