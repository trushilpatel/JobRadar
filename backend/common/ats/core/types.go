package core

// ================================================
// Crawler: Types
// ================================================

// JobContent — full details of a job.
// Sometimes career page provides all details at listing time.
// JobContent is the full details of a job.
type JobContent struct {
	Description   string `json:"description"`
	Requirements  string `json:"requirements"`
	Compensation  string `json:"compensation"`
	WorkplaceType string `json:"workplace_type"`
	Department    string `json:"department"`
	PublishedDate string `json:"published_date"`
}

// JobSummary — one job on the company careers listing page.
type JobSummary struct {
	ID            string      `json:"id"`         // e.g. "greenhouse-12345"
	CompanyID     string      `json:"company_id"` // UUID from companies table
	Adapter       string      `json:"adapter"`    // "greenhouse", "lever", "ashby", etc.
	Title         string      `json:"title"`
	Location      string      `json:"location"`
	URL           string      `json:"url"`
	EmployType    string      `json:"employ_type"` // full_time, contract, part_time, internship
	JobContent    *JobContent `json:"job_content,omitempty"`
	LocationKnown bool        `json:"location_known,omitempty"` // true when AllLocations is reliable
	AllLocations  []string    `json:"all_locations,omitempty"`  // every distinct location from the adapter
}

// JobListing — full listing payload for a company's careers page (crawler output).
type JobListing struct {
	Jobs []JobSummary `json:"jobs"`
}

// ================================================
// Enricher: Types
// ================================================

// JobPost is the full details of a job.
// Union of all fields available across Greenhouse, Lever, and Ashby.
// Fields not provided by a platform are left as empty strings.
type JobPost struct {
	ID             string `json:"id"`              // all platforms
	Title          string `json:"title"`           // all platforms
	Location       string `json:"location"`        // all platforms
	URL            string `json:"url"`             // Greenhouse, Lever
	Description    string `json:"description"`     // plain text — HTML stripped from all platforms
	EmploymentType string `json:"employment_type"` // Lever (commitment), Ashby (employmentType)
	Department     string `json:"department"`      // Greenhouse (departments[0]), Lever (department > team fallback)
	Compensation   string `json:"compensation"`    // Greenhouse (pay_input_ranges), Lever (salaryDescriptionPlain > salaryRange), Ashby (compensationTierSummary)
	WorkplaceType  string `json:"workplace_type"`  // Lever (workplaceType), Ashby (workplaceType)
	Requirements   string `json:"requirements"`    // plain text — Lever (lists[])
	PublishedDate  string `json:"published_date"`  // Lever (createdAt ms), Ashby (publishedDate), Greenhouse (updated_at)
	CountryCode    string `json:"country_code"`    // ISO alpha-2 country code; populated by Workday and Meta FetchJobContent converters
}
