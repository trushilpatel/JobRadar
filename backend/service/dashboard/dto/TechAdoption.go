package dto

// TechAdoptionRow is the raw query result for tech tag quarterly job counts, before Go classification.
type TechAdoptionRow struct {
	Tech      string
	Q1        int
	Q2        int
	Q3        int
	Q4        int
	TotalJobs int
}
