package api

// JobPosting is one entry in the Ashby public job board listing response.
type JobPosting struct {
	ID                      string `json:"id"`
	Title                   string `json:"title"`
	LocationName            string `json:"locationName"`
	WorkplaceType           string `json:"workplaceType"`           // Remote, Hybrid, OnSite
	EmploymentType          string `json:"employmentType"`          // FullTime, PartTime, Contract, Intern
	CompensationTierSummary string `json:"compensationTierSummary"` // e.g. "$288K – $425K"
}

// JobBoardWithTeams wraps the job postings list in the GraphQL response.
type JobBoardWithTeams struct {
	JobPostings []JobPosting `json:"jobPostings"`
}

// ListingData is the top-level data object in the listing GraphQL response.
type ListingData struct {
	JobBoardWithTeams JobBoardWithTeams `json:"jobBoardWithTeams"`
}

// ListingResponse is the full GraphQL response for ApiJobBoardWithTeams.
type ListingResponse struct {
	Data ListingData `json:"data"`
}

// JobPostingDetail is the full job posting returned by ApiJobPosting.
type JobPostingDetail struct {
	ID                      string `json:"id"`
	Title                   string `json:"title"`
	LocationName            string `json:"locationName"`
	WorkplaceType           string `json:"workplaceType"`
	EmploymentType          string `json:"employmentType"`
	DescriptionHtml         string `json:"descriptionHtml"`
	CompensationTierSummary string `json:"compensationTierSummary"`
	PublishedDate           string `json:"publishedDate"`
}

// PostingData is the top-level data object in the detail GraphQL response.
type PostingData struct {
	JobPosting JobPostingDetail `json:"jobPosting"`
}

// PostingResponse is the full GraphQL response for ApiJobPosting.
type PostingResponse struct {
	Data PostingData `json:"data"`
}
