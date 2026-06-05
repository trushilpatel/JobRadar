package meta

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/trushilpatel/jobradar/jobingest/adapters/ats"
	metaapi "github.com/trushilpatel/jobradar/jobingest/adapters/ats/meta/api"
	"github.com/trushilpatel/jobradar/jobingest/core"
)

const adapterPrefix = "meta"

// mapJobSummary builds a thin core.JobSummary from a numeric job ID.
// Title and Location are intentionally empty — not available at listing time.
func mapJobSummary(numericID string) core.JobSummary {
	return core.JobSummary{
		ID:       adapterPrefix + "-" + numericID,
		URL:      detailURLPrefix + numericID,
		Title:    "",
		Location: "",
	}
}

// mapJobPost converts a parsed JobPosting and the originating summary into a
// core.JobPost. The summary.ID is passed through unchanged.
func mapJobPost(jp *metaapi.JobPosting, summaryID string) core.JobPost {
	return core.JobPost{
		ID:             summaryID,
		Title:          strings.TrimSpace(jp.Title),
		Location:       extractLocation(jp),
		Description:    buildDescription(jp),
		EmploymentType: core.EmployFullTime,
		Department:     jp.HiringOrg.Name,
		PublishedDate:  jp.DatePosted,
		CountryCode:    extractCountryCode(jp),
		Compensation:   buildCompensation(jp),
	}
}

// buildDescription assembles the canonical multi-section description string.
// Strips HTML from each section then joins them with headers.
func buildDescription(jp *metaapi.JobPosting) string {
	var b strings.Builder

	if t := strings.TrimSpace(jp.Title); t != "" {
		b.WriteString(t)
		b.WriteString("\n\n")
	}

	locs := locations(jp)
	if len(locs) > 0 {
		const cap = 5
		shown := locs
		if len(locs) > cap {
			shown = locs[:cap]
		}
		b.WriteString(strings.Join(shown, "; "))
		if len(locs) > cap {
			fmt.Fprintf(&b, " (+%d more)", len(locs)-cap)
		}
		b.WriteString("\n\n")
	}

	writeSection := func(label, htmlText string) {
		clean := ats.StripHTML(htmlText)
		if clean == "" {
			return
		}
		fmt.Fprintf(&b, "%s\n%s\n\n", label, clean)
	}
	writeSection("Description", jp.Description)
	writeSection("Responsibilities", jp.Responsibilities)
	writeSection("Qualifications", jp.Qualifications)

	return strings.TrimSpace(b.String())
}

// buildCompensation formats the baseSalary field when present.
// Returns empty string if baseSalary is absent or malformed.
func buildCompensation(jp *metaapi.JobPosting) string {
	if len(jp.BaseSalary) == 0 {
		return ""
	}
	var salary struct {
		Currency string `json:"currency"`
		Value    struct {
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
			UnitText string  `json:"unitText"`
		} `json:"value"`
	}
	if err := json.Unmarshal(jp.BaseSalary, &salary); err != nil {
		return ""
	}
	if salary.Value.MinValue == 0 && salary.Value.MaxValue == 0 {
		return ""
	}
	currency := salary.Currency
	if currency == "" {
		currency = "USD"
	}
	unit := salary.Value.UnitText
	if unit == "" {
		unit = "YEAR"
	}
	if salary.Value.MinValue > 0 && salary.Value.MaxValue > 0 {
		return fmt.Sprintf("%s %.0f – %.0f / %s", currency, salary.Value.MinValue, salary.Value.MaxValue, unit)
	}
	if salary.Value.MaxValue > 0 {
		return fmt.Sprintf("%s %.0f / %s", currency, salary.Value.MaxValue, unit)
	}
	return fmt.Sprintf("%s %.0f / %s", currency, salary.Value.MinValue, unit)
}
