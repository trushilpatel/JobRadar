package greenhouse

import (
	"fmt"

	ghapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/greenhouse/api"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

func mapJobListing(g ghapi.JobListing) core.JobListing {
	items := make([]core.JobSummary, 0, len(g.Jobs))
	for _, j := range g.Jobs {
		items = append(items, mapJobSummary(j))
	}
	return core.JobListing{Jobs: items}
}

func mapJobSummary(g ghapi.JobSummary) core.JobSummary {
	return core.JobSummary{
		ID:       fmt.Sprintf("greenhouse-%d", g.ID),
		Title:    g.Title,
		Location: g.Location.Name,
		URL:      g.AbsoluteURL,
	}
}

func mapJobPost(g ghapi.JobPost) core.JobPost {
	dept := ""
	if len(g.Departments) > 0 {
		dept = g.Departments[0].Name
	}
	comp := ""
	if len(g.PayInputRanges) > 0 {
		p := g.PayInputRanges[0]
		comp = fmt.Sprintf("$%d – $%d %s", p.MinCents/100, p.MaxCents/100, p.CurrencyType)
	}
	return core.JobPost{
		ID:            fmt.Sprintf("greenhouse-%d", g.ID),
		Title:         g.Title,
		Location:      g.Location.Name,
		URL:           g.AbsoluteURL,
		Description:   g.Content,
		Department:    dept,
		Compensation:  comp,
		PublishedDate: g.UpdatedAt,
	}
}
