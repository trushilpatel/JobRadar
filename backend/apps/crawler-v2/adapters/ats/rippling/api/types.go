package api

// ListingNextData is the top-level shape of __NEXT_DATA__ on the careers listing page.
// Path: props.pageProps.jobs.items[]
type ListingNextData struct {
	Props struct {
		PageProps struct {
			Jobs struct {
				Items []ListingJob `json:"items"`
			} `json:"jobs"`
		} `json:"pageProps"`
	} `json:"props"`
}

// ListingJob is one item from the listing page's jobs.items array.
type ListingJob struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	URL        string     `json:"url"`
	Department Department `json:"department"`
	Locations  []Location `json:"locations"`
	Language   string     `json:"language"`
}

// DetailNextData is the top-level shape of __NEXT_DATA__ on a job detail page.
// Path: props.pageProps.apiData
type DetailNextData struct {
	Props struct {
		PageProps struct {
			APIData DetailJob `json:"apiData"`
		} `json:"pageProps"`
	} `json:"props"`
}

// DetailJob is the apiData object from the detail page.
// Field names are flexible to handle API variations.
type DetailJob struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	DescriptionHTML string     `json:"descriptionHtml"`
	Description     string     `json:"description"`
	JobDescription  string     `json:"jobDescription"`
	Body            string     `json:"body"`
	Compensation    string     `json:"compensation"`
	CompensationRng string     `json:"compensationRange"`
	Salary          string     `json:"salary"`
	WorkplaceType   string     `json:"workplaceType"`
	WorkplaceTypAlt string     `json:"workplace_type"`
	Department      Department `json:"department"`
	DepartmentName  string     `json:"departmentName"`
	Locations       []Location `json:"locations"`
	CreatedAt       string     `json:"createdAt"`
}

// Department represents a job's department.
type Department struct {
	Name string `json:"name"`
}

// Location represents a geographic location for a job.
type Location struct {
	Name        string `json:"name"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	State       string `json:"state"`
	StateCode   string `json:"stateCode"`
	City        string `json:"city"`
}
