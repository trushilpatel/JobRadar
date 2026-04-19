package company

import (
	"github.com/trushilpatel/jobradar/service/company/common"
	"github.com/trushilpatel/jobradar/service/company/dto"
)

// Backward compatibility: re-export types from new locations
type ScanHistoryReader = dto.ScanHistoryReader
type Config = common.Config
type CompanyFilter = dto.CompanyFilter
type CompanyListResult = dto.CompanyListResult
type CompanyView = dto.CompanyView
type CreateCompanyInput = dto.CreateCompanyInput
type UpdateCompanyInput = dto.UpdateCompanyInput
