package dto

// CompanyPublicListResult is returned by CompanyService.ListPublic.
type CompanyPublicListResult struct {
	Items []CompanyPublicView
	Total int64
}
