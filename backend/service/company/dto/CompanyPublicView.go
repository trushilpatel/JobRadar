package dto

import (
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
)

// CompanyPublicView is the lean public-facing view returned by /api/companies.
// It deliberately exposes ONLY the public Company fields — no scan timestamps,
// no adapter type, no error counters. Operational state lives on
// CompanyAdminView and is reachable only through the admin chain.
type CompanyPublicView struct {
	companymodel.Company
}
