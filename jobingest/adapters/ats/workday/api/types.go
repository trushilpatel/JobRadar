// Package api holds raw CXS API types for the Workday adapter.
// Mirrors the full documented response so adding a downstream field never
// requires touching the HTTP/decode path.
package api

import "encoding/json"

// ListJobsRequest is the POST body sent to the CXS listings endpoint.
type ListJobsRequest struct {
	AppliedFacets map[string][]string `json:"appliedFacets"`
	Limit         int                 `json:"limit"`
	Offset        int                 `json:"offset"`
	SearchText    string              `json:"searchText"`
}

// ListJobsResponse is the JSON response from POST .../jobs.
type ListJobsResponse struct {
	Total       int          `json:"total"`
	JobPostings []JobPosting `json:"jobPostings"`
}

// JobPosting is one entry in the listing response.
type JobPosting struct {
	Title         string   `json:"title"`
	ExternalPath  string   `json:"externalPath"`
	LocationsText string   `json:"locationsText"`
	PostedOn      string   `json:"postedOn"`
	BulletFields  []string `json:"bulletFields"`
}

// DetailResponse is the JSON response from GET .../job/{externalPath}.
type DetailResponse struct {
	JobPostingInfo JobPostingInfo `json:"jobPostingInfo"`
}

// JobPostingInfo mirrors the CXS detail response. Field types and names were
// validated against live Workday tenants; notably `country` is an object
// ({descriptor, id}) not a string, and the only ISO-shaped date Workday returns
// is `startDate` (`postedOn` is a display string like "Posted Today").
type JobPostingInfo struct {
	ID                     string          `json:"id"`
	Title                  string          `json:"title"`
	JobDescription         string          `json:"jobDescription"`
	Location               string          `json:"location"`
	AdditionalLocations    json.RawMessage `json:"additionalLocations"`
	PostedOn               string          `json:"postedOn"`
	StartDate              string          `json:"startDate"`
	TimeType               string          `json:"timeType"`
	JobReqID               string          `json:"jobReqId"`
	JobPostingID           string          `json:"jobPostingId"`
	JobPostingSiteID       string          `json:"jobPostingSiteId"`
	Country                json.RawMessage `json:"country"`
	CanApply               bool            `json:"canApply"`
	Posted                 bool            `json:"posted"`
	IncludeResumeParsing   bool            `json:"includeResumeParsing"`
	JobRequisitionLocation json.RawMessage `json:"jobRequisitionLocation"`
	ExternalURL            string          `json:"externalUrl"`
	QuestionnaireID        string          `json:"questionnaireId"`
}
