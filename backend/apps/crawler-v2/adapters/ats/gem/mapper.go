package gem

import (
	"strings"

	"github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats"
	gemapi "github.com/trushilpatel/jobradar/apps/crawler-v2/adapters/ats/gem/api"
	"github.com/trushilpatel/jobradar/apps/crawler-v2/core"
)

func mapJobListing(posts []gemapi.JobPost) core.JobListing {
	items := make([]core.JobSummary, 0, len(posts))
	for _, p := range posts {
		items = append(items, mapJobSummary(p))
	}
	return core.JobListing{Jobs: items}
}

func mapJobSummary(p gemapi.JobPost) core.JobSummary {
	loc := resolveLocation(p)
	return core.JobSummary{
		ID:            "gem-" + p.ID,
		Title:         p.Title,
		URL:           p.AbsoluteURL,
		Location:      loc,
		EmployType:    mapEmploymentType(p.EmploymentType),
		LocationKnown: true,
		AllLocations:  []string{loc},
		JobContent: &core.JobContent{
			Description:   resolveDescription(p),
			Department:    firstDepartment(p.Departments),
			WorkplaceType: mapWorkplaceType(p.LocationType),
			PublishedDate: resolvePublishedDate(p),
		},
	}
}

// resolveLocation prefers location.name, falling back to joining all office
// location names when the primary field is empty.
func resolveLocation(p gemapi.JobPost) string {
	if s := strings.TrimSpace(p.Location.Name); s != "" {
		return s
	}
	parts := make([]string, 0, len(p.Offices))
	for _, o := range p.Offices {
		if s := strings.TrimSpace(o.Location.Name); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "; ")
}

// resolveDescription prefers the pre-stripped content_plain field when present,
// falling back to StripHTML over the HTML content.
func resolveDescription(p gemapi.JobPost) string {
	if s := strings.TrimSpace(p.ContentPlain); s != "" {
		return s
	}
	return ats.StripHTML(p.Content)
}

// resolvePublishedDate prefers first_published_at, falls back to updated_at,
// then created_at — whichever is first non-empty.
func resolvePublishedDate(p gemapi.JobPost) string {
	for _, s := range []string{p.FirstPublishedAt, p.UpdatedAt, p.CreatedAt} {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

func firstDepartment(deps []gemapi.Taxonomy) string {
	if len(deps) == 0 {
		return ""
	}
	return deps[0].Name
}

// mapEmploymentType maps Gem's snake_case employment_type values onto the
// core constants. Unknown values default to full_time, consistent with all
// other adapters.
func mapEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "full_time":
		return core.EmployFullTime
	case "part_time":
		return core.EmployPartTime
	case "contract":
		return core.EmployContract
	case "intern", "internship":
		return core.EmployInternship
	default:
		return core.EmployFullTime
	}
}

// mapWorkplaceType normalizes Gem's location_type values to the strings the
// rest of the pipeline already expects (remote / hybrid / onsite).
func mapWorkplaceType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "in_office", "onsite", "on_site":
		return "onsite"
	default:
		return ""
	}
}
