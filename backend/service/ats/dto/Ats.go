// Package dto holds the data transfer objects for the ats domain.
package dto

import "time"

// ATSDTO is the JSON-serialisable representation of an ATS row returned by
// the service layer to callers. It carries all 8 fields from the DB without
// any GORM coupling.
type ATSDTO struct {
	Name               string    `json:"name"`
	DisplayName        string    `json:"display_name"`
	BaseURL            string    `json:"base_url"`
	IsActive           bool      `json:"is_active"`
	IsAdminCreatable   bool      `json:"is_admin_creatable"`
	ScanIntervalMin    int       `json:"scan_interval_min"`
	InterCompanyDelayS int       `json:"inter_company_delay_s"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// UpdateATSInput carries the optional fields an admin may change on an
// existing ATS row. All fields are pointers so the handler can distinguish
// between "not supplied" (nil) and "set to zero value".
type UpdateATSInput struct {
	ScanIntervalMin    *int  `json:"scan_interval_min"`
	InterCompanyDelayS *int  `json:"inter_company_delay_s"`
	IsActive           *bool `json:"is_active"`
	IsAdminCreatable   *bool `json:"is_admin_creatable"`
}
