package services

import (
	"context"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

// publicValidationService validates inputs to the public chain. The public
// surface is read-only, so only ListPublic carries validation rules — Get
// is a single id lookup the REST layer already constrains via path-param
// length / shape.
type publicValidationService struct {
	BasePublicService
}

// NewValidationService wraps next with the public chain validator.
func NewValidationService(next CompanyService) CompanyService {
	return &publicValidationService{BasePublicService: NewBasePublicService(next)}
}

// ListPublic enforces limit bounds and search-length so a malicious caller
// cannot cause a DB scan with an unbounded limit or LIKE pattern.
func (v *publicValidationService) ListPublic(ctx context.Context, filter companydto.CompanyFilter, limit int) (companydto.CompanyPublicListResult, error) {
	var errs apperr.ValidationErrors
	if len(filter.Search) > 100 {
		errs = append(errs, apperr.ValidationError{
			Field:   "search",
			Code:    "INVALID_VALUE",
			Message: "search must be 100 characters or fewer",
		})
	}
	if limit < 1 || limit > 100 {
		errs = append(errs, apperr.ValidationError{
			Field:   "limit",
			Code:    "INVALID_VALUE",
			Message: "limit must be between 1 and 100",
		})
	}
	if len(errs) > 0 {
		return companydto.CompanyPublicListResult{}, errs
	}
	return v.next.ListPublic(ctx, filter, limit)
}

var _ CompanyService = (*publicValidationService)(nil)
