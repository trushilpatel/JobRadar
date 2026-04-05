package store

import "gorm.io/gorm"

// TechCount holds the number of jobs mentioning a specific technology.
type TechCount struct {
	Tech  string `json:"tech"`
	Count int    `json:"count"`
}

// CompanyActivityItem holds the job count for a company in a given ISO week.
type CompanyActivityItem struct {
	CompanyName string `json:"company_name"`
	Week        string `json:"week"`
	Count       int    `json:"count"`
}

// InsightsReader provides aggregated analytics over the job pipeline.
type InsightsReader interface {
	GetTechStackTrends(limit int) ([]TechCount, error)
	GetCompanyActivity() ([]CompanyActivityItem, error)
}

type insightsStore struct {
	db *gorm.DB
}

func NewInsightsStore(db *gorm.DB) InsightsReader {
	return &insightsStore{db: db}
}

// GetTechStackTrends unnests the tech_stack JSONB array and counts occurrences.
func (s *insightsStore) GetTechStackTrends(limit int) ([]TechCount, error) {
	var results []TechCount
	err := s.db.Raw(`
		SELECT tech, COUNT(*) AS count
		FROM job, jsonb_array_elements_text(tech_stack) AS tech
		WHERE tech_stack IS NOT NULL
		GROUP BY tech
		ORDER BY count DESC
		LIMIT ?
	`, limit).Scan(&results).Error
	return results, err
}

// GetCompanyActivity returns job counts per company per ISO week.
func (s *insightsStore) GetCompanyActivity() ([]CompanyActivityItem, error) {
	var results []CompanyActivityItem
	err := s.db.Raw(`
		SELECT
			c.name AS company_name,
			TO_CHAR(DATE_TRUNC('week', j.added_at), 'YYYY-MM-DD') AS week,
			COUNT(*) AS count
		FROM job j
		JOIN companies c ON j.company_id = c.id
		GROUP BY c.name, DATE_TRUNC('week', j.added_at)
		ORDER BY week DESC, count DESC
	`).Scan(&results).Error
	return results, err
}
