package api

// JobPost mirrors the full Gem Job Board API response for a single posting.
// Every documented field is captured so adding a downstream consumer never
// requires touching the HTTP/decode path.
type JobPost struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	AbsoluteURL      string     `json:"absolute_url"`
	Content          string     `json:"content"`       // full HTML description
	ContentPlain     string     `json:"content_plain"` // pre-stripped plain text (preferred)
	Location         Location   `json:"location"`
	LocationType     string     `json:"location_type"`   // in_office | remote | hybrid
	EmploymentType   string     `json:"employment_type"` // full_time | part_time | contract | intern
	Departments      []Taxonomy `json:"departments"`
	Offices          []Office   `json:"offices"`
	FirstPublishedAt string     `json:"first_published_at"` // ISO 8601
	UpdatedAt        string     `json:"updated_at"`         // ISO 8601
	CreatedAt        string     `json:"created_at"`         // ISO 8601
}

// Location holds the name of a geographic location.
type Location struct {
	Name string `json:"name"`
}

// Taxonomy represents a department or team node.
type Taxonomy struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Office represents a physical office with its own location.
type Office struct {
	Location Location `json:"location"`
}
