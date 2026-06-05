package api

import "encoding/json"

// SearchResponse is the top-level JSON from /api/pcsx/search.
// Error is RawMessage because the API returns either a string or an object
// depending on the error condition; RawMessage accepts any JSON value.
type SearchResponse struct {
	Status any                `json:"status"`
	Error  json.RawMessage    `json:"error"`
	Data   SearchResponseData `json:"data"`
}

// SearchResponseData holds the paginated positions list and total count.
type SearchResponseData struct {
	Count          int         `json:"count"`
	Positions      []Position  `json:"positions"`
	FilterDef      interface{} `json:"filterDef"`
	SortBy         interface{} `json:"sortBy"`
	AppliedFilters interface{} `json:"appliedFilters"`
}

// Position mirrors a single entry in the Microsoft positions array.
// StandardizedLocations is RawMessage because the API has returned both
// map[string][]string and []interface{} depending on the response context.
type Position struct {
	ID                    int64           `json:"id"`
	DisplayJobID          string          `json:"displayJobId"`
	Name                  string          `json:"name"`
	Locations             []string        `json:"locations"`
	StandardizedLocations json.RawMessage `json:"standardizedLocations"`
	PostedTs              int64           `json:"postedTs"`
	Department            string          `json:"department"`
	WorkLocationOption    string          `json:"workLocationOption"`
	AtsJobID              string          `json:"atsJobId"`
	PositionURL           string          `json:"positionUrl"`
}

// PositionDetail is the payload inside data.* from /api/pcsx/position_details.
// StandardizedLocations is RawMessage — the API returns []string, not map[string][]string.
type PositionDetail struct {
	ID                         int64           `json:"id"`
	DisplayJobID               string          `json:"displayJobId"`
	Name                       string          `json:"name"`
	Locations                  []string        `json:"locations"`
	StandardizedLocations      json.RawMessage `json:"standardizedLocations"`
	PostedTs                   int64           `json:"postedTs"`
	Department                 string          `json:"department"`
	WorkLocationOption         string          `json:"workLocationOption"`
	JobDescription             string          `json:"jobDescription"`
	Location                   string          `json:"location"`
	EfcustomTextWorkSite       []string        `json:"efcustomTextWorkSite"`
	EfcustomTextEmploymentType []string        `json:"efcustomTextEmploymentType"`
	EfcustomTextRoletype       []string        `json:"efcustomTextRoletype"`
	EfcustomTextRequiredTravel []string        `json:"efcustomTextRequiredTravel"`
	PublicURL                  string          `json:"publicUrl"`
	AtsJobID                   string          `json:"atsJobId"`
	PositionURL                string          `json:"positionUrl"`
}

// PositionDetailResponse is the top-level wrapper from /api/pcsx/position_details.
// The actual detail lives in Data, not at the top level.
type PositionDetailResponse struct {
	Status any             `json:"status"`
	Error  json.RawMessage `json:"error"`
	Data   PositionDetail  `json:"data"`
}
