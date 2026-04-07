package store

import "gorm.io/gorm"

// ── Output types ─────────────────────────────────────────────────────────────

type TechCount struct {
	Tech  string `json:"tech"`
	Count int    `json:"count"`
}

type CompanyActivityItem struct {
	CompanyName string `json:"company_name"`
	Week        string `json:"week"`
	Count       int    `json:"count"`
}

// FunnelStep holds a single stage in the enrichment → action funnel.
type FunnelStep struct {
	Stage string `json:"stage"`
	Count int    `json:"count"`
}

// LocationCount holds job counts by country.
type LocationCount struct {
	Country string `json:"country"`
	Count   int    `json:"count"`
}

// StateCount holds job counts by US state.
type StateCount struct {
	State string `json:"state"`
	Count int    `json:"count"`
}

// SponsorshipDay holds daily sponsorship vs total enriched counts.
type SponsorshipDay struct {
	Date      string `json:"date"`
	Total     int    `json:"total"`
	Sponsored int    `json:"sponsored"`
}

// CompanyVelocity holds per-company job counts for the current and prior week.
type CompanyVelocity struct {
	CompanyID   string `json:"company_id"`
	CompanyName string `json:"company_name"`
	ThisWeek    int    `json:"this_week"`
	LastWeek    int    `json:"last_week"`
	Delta       int    `json:"delta"` // this_week - last_week
}

// RoleCategoryCount holds job counts per role category.
type RoleCategoryCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// PipelineCounts holds the current depth of each pipeline stage.
type PipelineCounts struct {
	// Pending = fetched by scanner but not yet enriched (in scorer or enricher queue)
	Pending int64 `json:"pending"`
	// Errors = jobs marked 'error' in the seen ledger
	Errors int64 `json:"errors"`
}

// ── Interface ────────────────────────────────────────────────────────────────

// InsightsReader provides aggregated analytics over the job pipeline.
type InsightsReader interface {
	// Existing — queries job table (all historical data including dismissed)
	GetTechStackTrends(limit int) ([]TechCount, error)
	GetCompanyActivity() ([]CompanyActivityItem, error)

	// New — query job_snapshot
	GetPipelineFunnel() ([]FunnelStep, error)
	GetLocationBreakdown() ([]LocationCount, error)
	GetUSAStateBreakdown() ([]StateCount, error)
	GetSponsorshipTrend(days int) ([]SponsorshipDay, error)
	GetCompanyVelocity(limit int) ([]CompanyVelocity, error)
	GetRoleCategoryBreakdown() ([]RoleCategoryCount, error)

	// Queue depth — jobs in-flight vs errored
	GetPipelineCounts() (PipelineCounts, error)
}

type insightsStore struct {
	db *gorm.DB
}

func NewInsightsStore(db *gorm.DB) InsightsReader {
	return &insightsStore{db: db}
}

// ── Existing queries (job table) ─────────────────────────────────────────────

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

// ── New queries (job_snapshot) ────────────────────────────────────────────────

// GetPipelineFunnel returns enriched → saved → applied counts from snapshot.
func (s *insightsStore) GetPipelineFunnel() ([]FunnelStep, error) {
	var rows []struct {
		Action *string
		Count  int
	}
	err := s.db.Raw(`
		SELECT user_action AS action, COUNT(*) AS count
		FROM job_snapshot
		GROUP BY user_action
	`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	counts := map[string]int{}
	total := 0
	for _, r := range rows {
		total += r.Count
		if r.Action != nil {
			counts[*r.Action] = r.Count
		}
	}

	return []FunnelStep{
		{Stage: "enriched", Count: total},
		{Stage: "saved", Count: counts["saved"]},
		{Stage: "applied", Count: counts["applied"]},
		{Stage: "dismissed", Count: counts["dismissed"]},
	}, nil
}

// GetLocationBreakdown returns job counts by country.
func (s *insightsStore) GetLocationBreakdown() ([]LocationCount, error) {
	var results []LocationCount
	err := s.db.Raw(`
		SELECT COALESCE(country, 'Unknown') AS country, COUNT(*) AS count
		FROM job_snapshot
		GROUP BY country
		ORDER BY count DESC
	`).Scan(&results).Error
	return results, err
}

// GetUSAStateBreakdown returns job counts by US state.
func (s *insightsStore) GetUSAStateBreakdown() ([]StateCount, error) {
	var results []StateCount
	err := s.db.Raw(`
		SELECT COALESCE(state, 'Unknown') AS state, COUNT(*) AS count
		FROM job_snapshot
		WHERE country = 'USA' AND state IS NOT NULL
		GROUP BY state
		ORDER BY count DESC
		LIMIT 20
	`).Scan(&results).Error
	return results, err
}

// GetSponsorshipTrend returns daily total and sponsored counts for the last N days.
func (s *insightsStore) GetSponsorshipTrend(days int) ([]SponsorshipDay, error) {
	var results []SponsorshipDay
	err := s.db.Raw(`
		SELECT
			TO_CHAR(snapshotted_at, 'YYYY-MM-DD') AS date,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE sponsorship = true) AS sponsored
		FROM job_snapshot
		WHERE snapshotted_at >= CURRENT_DATE - ? * INTERVAL '1 day'
		GROUP BY snapshotted_at
		ORDER BY snapshotted_at ASC
	`, days).Scan(&results).Error
	return results, err
}

// GetCompanyVelocity compares this week vs last week per company, sorted by delta desc.
func (s *insightsStore) GetCompanyVelocity(limit int) ([]CompanyVelocity, error) {
	var results []CompanyVelocity
	err := s.db.Raw(`
		WITH weekly AS (
			SELECT
				s.company_id,
				c.name AS company_name,
				COUNT(*) FILTER (
					WHERE s.snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE)
				) AS this_week,
				COUNT(*) FILTER (
					WHERE s.snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE) - INTERVAL '7 days'
					  AND s.snapshotted_at <  DATE_TRUNC('week', CURRENT_DATE)
				) AS last_week
			FROM job_snapshot s
			JOIN companies c ON s.company_id = c.id
			WHERE s.snapshotted_at >= DATE_TRUNC('week', CURRENT_DATE) - INTERVAL '7 days'
			GROUP BY s.company_id, c.name
		)
		SELECT *, (this_week - last_week) AS delta
		FROM weekly
		WHERE this_week > 0 OR last_week > 0
		ORDER BY delta DESC, this_week DESC
		LIMIT ?
	`, limit).Scan(&results).Error
	return results, err
}

// GetPipelineCounts returns the number of jobs pending enrichment and errored.
// Pending = in seen_ledger with reason='fetch' but no corresponding row in job table.
// Errors = in seen_ledger with reason='error'.
func (s *insightsStore) GetPipelineCounts() (PipelineCounts, error) {
	var result struct {
		Pending int64
		Errors  int64
	}
	err := s.db.Raw(`
		SELECT
			COUNT(*) FILTER (
				WHERE l.reason = 'fetch'
				AND NOT EXISTS (SELECT 1 FROM job j WHERE j.job_post_id = l.job_id)
			) AS pending,
			COUNT(*) FILTER (WHERE l.reason = 'error') AS errors
		FROM job_seen_ledger l
	`).Scan(&result).Error
	return PipelineCounts{Pending: result.Pending, Errors: result.Errors}, err
}

// GetRoleCategoryBreakdown returns job counts per role category.
func (s *insightsStore) GetRoleCategoryBreakdown() ([]RoleCategoryCount, error) {
	var results []RoleCategoryCount
	err := s.db.Raw(`
		SELECT COALESCE(role_category, 'other') AS category, COUNT(*) AS count
		FROM job_snapshot
		GROUP BY role_category
		ORDER BY count DESC
	`).Scan(&results).Error
	return results, err
}
