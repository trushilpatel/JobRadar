package lever

import (
	"fmt"
	"strings"

	"github.com/trushilpatel/jobradar/jobingest/adapters/ats"
	leverapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/lever/api"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

func mapJobListing(summaries []leverapi.JobSummary) core.JobListing {
	items := make([]core.JobSummary, 0, len(summaries))
	for _, j := range summaries {
		items = append(items, mapJobSummary(j))
	}
	return core.JobListing{Jobs: items}
}

func mapJobSummary(j leverapi.JobSummary) core.JobSummary {
	return core.JobSummary{
		ID:            "lever-" + j.ID,
		Title:         j.Text,
		Location:      j.Categories.Location,
		URL:           j.HostedURL,
		EmployType:    normalizeLeverCommitment(j.Categories.Commitment),
		LocationKnown: true,
		AllLocations:  j.Categories.AllLocations,
		JobContent:    nil,
	}
}

func mapJobPost(p leverapi.JobPost) core.JobPost {
	// Full description: descriptionPlain already contains opening + body.
	// Append additionalPlain (closing content) if present.
	desc := p.DescriptionPlain
	if p.AdditionalPlain != "" {
		desc = strings.TrimSpace(desc + "\n" + p.AdditionalPlain)
	}

	// Requirement/benefit sections from lists[].
	var reqParts []string
	for _, section := range p.Lists {
		plain := ats.StripHTML(section.Content)
		if plain != "" {
			reqParts = append(reqParts, section.Text+": "+plain)
		}
	}

	// Compensation: prefer human-readable plain text; fall back to structured range.
	comp := p.SalaryDescriptionPlain
	if comp == "" && p.SalaryRange != nil {
		comp = fmt.Sprintf("%.0f – %.0f %s / %s",
			p.SalaryRange.Min, p.SalaryRange.Max,
			p.SalaryRange.Currency, p.SalaryRange.Interval)
	}

	// Department: prefer the broader department field, fall back to team.
	dept := p.Categories.Department
	if dept == "" {
		dept = p.Categories.Team
	}

	publishedDate := ""
	if p.CreatedAt > 0 {
		publishedDate = fmt.Sprintf("%d", p.CreatedAt)
	}

	return core.JobPost{
		ID:             "lever-" + p.ID,
		Title:          p.Text,
		Location:       p.Categories.Location,
		URL:            p.HostedURL,
		Description:    desc,
		Requirements:   strings.Join(reqParts, "\n"),
		Compensation:   comp,
		Department:     dept,
		EmploymentType: normalizeLeverCommitment(p.Categories.Commitment),
		WorkplaceType:  p.WorkplaceType,
		PublishedDate:  publishedDate,
	}
}

// normalizeLeverCommitment maps Lever's free-text Commitment to core employment type constants.
func normalizeLeverCommitment(c string) string {
	switch strings.ToLower(c) {
	case "full-time", "fulltime", "full time":
		return core.EmployFullTime
	case "part-time", "parttime", "part time":
		return core.EmployPartTime
	case "contract", "contractor":
		return core.EmployContract
	case "internship", "intern":
		return core.EmployInternship
	default:
		return ""
	}
}
