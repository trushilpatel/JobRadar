// Package companymodel defines the GORM structs for the company domain.
//
// Phase 2 split: this `Company` struct holds only the public-facing fields
// (id, name, slug, website, careers_url, logo_url, timestamps). All
// operational scan state — adapter type, adapter config, scan timestamps,
// error counters — moved to `CompanyScanSettings` (a separate row, separate
// table, separate decorator chain).
package companymodel

import "time"

// Company is the public-only company row backed by the `companies` table.
// Workers and the public API see only this struct; admin endpoints join
// CompanyScanSettings on top via the admin chain.
type Company struct {
	ID         string    `gorm:"primaryKey"              json:"id"`
	Name       string    `gorm:"not null"                json:"name"`
	Slug       string    `gorm:"not null;uniqueIndex"    json:"slug"`
	Website    *string   `gorm:"column:website"          json:"website,omitempty"`
	CareersURL *string   `gorm:"column:careers_url"      json:"careers_url,omitempty"`
	LogoURL    *string   `gorm:"column:logo_url"         json:"logo_url,omitempty"`
	CreatedAt  time.Time `gorm:"not null;autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

// TableName pins the GORM table name. Kept explicit so a future rename
// (e.g. to `public_companies`) is one edit, not a hunt.
func (Company) TableName() string { return "companies" }
