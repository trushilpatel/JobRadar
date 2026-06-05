// Package api contains the raw types decoded from the Deel RSC stream.
package api

import "encoding/json"

// RawJobPosting is one job posting object extracted from the RSC stream.
// The outer `id` is the job posting ID used in public URLs; `jobId` is the
// internal Deel job ID. Title and richtextDescription live at the outer level;
// employment, location, and department data are on the nested `job` object.
type RawJobPosting struct {
	ID                  string             `json:"id"`
	JobID               string             `json:"jobId"`
	Title               string             `json:"title"`
	RichtextDescription json.RawMessage    `json:"richtextDescription"`
	Job                 RawJob             `json:"job"`
	JobPostingPubs      []RawJobPostingPub `json:"jobPostingPublications"`
}

// RawJob holds nested per-job data nested under `job` in the posting record.
type RawJob struct {
	JobEmploymentTypes  []RawEmploymentType `json:"jobEmploymentTypes"`
	JobLocations        []RawJobLocation    `json:"jobLocations"`
	JobDepartments      []RawJobDepartment  `json:"jobDepartments"`
	JobTeams            []RawJobTeam        `json:"jobTeams"`
	CurrentCompensation *RawCompensation    `json:"currentCompensation"`
}

// RawEmploymentType wraps a single named employment type entry.
type RawEmploymentType struct {
	EmploymentType RawNamed `json:"employmentType"`
}

// RawJobLocation wraps a single named location entry.
type RawJobLocation struct {
	Location RawNamed `json:"location"`
}

// RawJobDepartment wraps a single named department entry.
type RawJobDepartment struct {
	Department RawNamed `json:"department"`
}

// RawJobTeam wraps a single named team entry.
type RawJobTeam struct {
	Team RawNamed `json:"team"`
}

// RawNamed is a generic name-bearing object used in employment type,
// location, department, and team sub-records.
type RawNamed struct {
	Name string `json:"name"`
}

// RawCompensation is the structured pay range attached to a job.
type RawCompensation struct {
	CurrencyIsoCode string  `json:"currencyIsoCode"`
	MinAmount       float64 `json:"minAmount"`
	MaxAmount       float64 `json:"maxAmount"`
}

// RawJobPostingPub carries publication-state timestamps.
type RawJobPostingPub struct {
	CurrentState RawPubState `json:"currentState"`
}

// RawPubState holds the publication event timestamp.
type RawPubState struct {
	CreatedAt string `json:"createdAt"`
}
