package dto

// CompanyPublicListResult is returned by CompanyService.ListPublic.
type CompanyPublicListResult struct {
	Items      []CompanyPublicView
	Total      int64
	NextCursor *string // nil when no next page exists
}
