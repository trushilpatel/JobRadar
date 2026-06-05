package api

type JobSummary struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
	AbsoluteURL string `json:"absolute_url"`
}

type JobListing struct {
	Jobs []JobSummary `json:"jobs"`
}

type JobPost struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
	AbsoluteURL string `json:"absolute_url"`
	Content     string `json:"content"` // full HTML job description
	Departments []struct {
		Name string `json:"name"`
	} `json:"departments"`
	UpdatedAt      string `json:"updated_at"`
	PayInputRanges []struct {
		MinCents     int    `json:"min_cents"`
		MaxCents     int    `json:"max_cents"`
		CurrencyType string `json:"currency_type"`
		Title        string `json:"title"`
		Blurb        string `json:"blurb"`
	} `json:"pay_input_ranges"`
}
