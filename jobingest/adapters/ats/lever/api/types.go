package api

// JobSummary represents one posting in the Lever listing array.
// GET /v0/postings/{company}?mode=json returns a JSON array of these.
type JobSummary struct {
	ID         string `json:"id"`        // UUID string
	Text       string `json:"text"`      // job title
	HostedURL  string `json:"hostedUrl"` // hosted job posting page URL
	ApplyURL   string `json:"applyUrl"`  // direct application form URL
	Country    string `json:"country"`   // ISO 3166-1 alpha-2, may be empty
	Categories struct {
		Location     string   `json:"location"`
		Commitment   string   `json:"commitment"` // e.g. "Full-time", "Contract"
		Team         string   `json:"team"`
		Department   string   `json:"department"`
		AllLocations []string `json:"allLocations"`
	} `json:"categories"`
	WorkplaceType string `json:"workplaceType"` // unspecified | on-site | remote | hybrid
}

// SalaryRange holds the structured salary range from a Lever job post.
type SalaryRange struct {
	Currency string  `json:"currency"` // e.g. "USD"
	Interval string  `json:"interval"` // e.g. "per-year-salary"
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
}

// JobPost mirrors the complete Lever Postings API detail response.
// GET /v0/postings/{company}/{id}?mode=json returns a JSON object of this type.
type JobPost struct {
	ID      string `json:"id"`      // UUID string
	Text    string `json:"text"`    // job title
	Country string `json:"country"` // ISO 3166-1 alpha-2, may be empty

	// URLs
	HostedURL string `json:"hostedUrl"` // hosted job posting page URL
	ApplyURL  string `json:"applyUrl"`  // direct application form URL

	// Description — multiple variants; descriptionPlain is primary
	Opening              string `json:"opening"`              // opening section (HTML)
	OpeningPlain         string `json:"openingPlain"`         // opening section (plain text)
	Description          string `json:"description"`          // opening + body (HTML)
	DescriptionPlain     string `json:"descriptionPlain"`     // opening + body (plain text) — primary
	DescriptionBody      string `json:"descriptionBody"`      // body without opening (HTML)
	DescriptionBodyPlain string `json:"descriptionBodyPlain"` // body without opening (plain text)
	Additional           string `json:"additional"`           // closing content (HTML)
	AdditionalPlain      string `json:"additionalPlain"`      // closing content (plain text)

	// Structured requirement/benefit sections
	Lists []struct {
		Text    string `json:"text"`    // section heading e.g. "Requirements"
		Content string `json:"content"` // unstyled HTML <li> items
	} `json:"lists"`

	// Compensation
	SalaryRange            *SalaryRange `json:"salaryRange"`            // structured range, optional
	SalaryDescription      string       `json:"salaryDescription"`      // salary narrative (HTML)
	SalaryDescriptionPlain string       `json:"salaryDescriptionPlain"` // salary narrative (plain text) — primary

	WorkplaceType string `json:"workplaceType"` // unspecified | on-site | remote | hybrid

	Categories struct {
		Location     string   `json:"location"`
		Commitment   string   `json:"commitment"` // e.g. "Full-time"
		Team         string   `json:"team"`
		Department   string   `json:"department"` // broader grouping above team
		AllLocations []string `json:"allLocations"`
	} `json:"categories"`

	CreatedAt int64 `json:"createdAt"` // Unix ms timestamp
}
