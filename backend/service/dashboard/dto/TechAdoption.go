package dto

// TechAdoptionRow is the raw query result for tech tag monthly job counts, before Go classification.
type TechAdoptionRow struct {
	Tech      string
	M1        int
	M2        int
	M3        int
	M4        int
	TotalJobs int
}
