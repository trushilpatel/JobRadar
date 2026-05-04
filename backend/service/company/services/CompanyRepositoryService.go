package services

import (
	"context"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	"gorm.io/gorm"
)

// publicRepositoryService is the terminal CompanyService implementation.
// It hits the `companies` table only — never `company_scan_settings` — so
// the public chain cannot accidentally leak operational fields. Each method
// converts companymodel.Company straight into a CompanyPublicView.
type publicRepositoryService struct {
	db *gorm.DB
}

// NewRepositoryService constructs the terminal CompanyService backed by GORM.
func NewRepositoryService(db *gorm.DB) CompanyService {
	return &publicRepositoryService{db: db}
}

// ListPublic returns a filtered, paginated page of public company views.
// The Active / AdapterTypes / HasSettings filter fields are intentionally
// IGNORED in the public chain — those are operational concerns the public
// API must not expose. Only Search and the Sort fields are honoured.
func (r *publicRepositoryService) ListPublic(ctx context.Context, filter companydto.CompanyFilter, page, limit int) (_ companydto.CompanyPublicListResult, err error) {
	ctx, span := tracing.Start(ctx, "company.public.repo.ListPublic")
	defer tracing.End(span, &err)

	q := r.db.WithContext(ctx).Model(&companymodel.Company{})
	if filter.Search != "" {
		q = q.Where("name ILIKE ?", "%"+filter.Search+"%")
	}

	var total int64
	if err = q.Count(&total).Error; err != nil {
		return companydto.CompanyPublicListResult{}, apperr.ErrInternal.WithMessage("failed to count companies")
	}

	col := publicSortColumn(filter.SortBy)
	dir := "ASC"
	if filter.SortDir == "desc" {
		dir = "DESC"
	}

	var rows []companymodel.Company
	offset := (page - 1) * limit
	if err = q.Order(col + " " + dir + " NULLS LAST").
		Offset(offset).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return companydto.CompanyPublicListResult{}, apperr.ErrInternal.WithMessage("failed to fetch companies")
	}

	views := make([]companydto.CompanyPublicView, 0, len(rows))
	for _, c := range rows {
		views = append(views, companydto.CompanyPublicView{Company: c})
	}
	return companydto.CompanyPublicListResult{Items: views, Total: total}, nil
}

// GetPublic returns the public view of one company by ID.
func (r *publicRepositoryService) GetPublic(ctx context.Context, id string) (_ *companydto.CompanyPublicView, err error) {
	ctx, span := tracing.Start(ctx, "company.public.repo.GetPublic")
	defer tracing.End(span, &err)

	var c companymodel.Company
	if err = r.db.WithContext(ctx).First(&c, "id = ?", id).Error; err != nil {
		if apperr.IsNotFound(err) {
			return nil, apperr.ErrNotFound.WithMessage("company not found")
		}
		return nil, apperr.ErrInternal.WithMessage("failed to fetch company")
	}
	return &companydto.CompanyPublicView{Company: c}, nil
}

// publicSortColumn whitelists sort columns to prevent SQL injection.
// Only public-fields are addressable here — operational sorts (last_scan_at)
// are handled exclusively by the admin chain.
func publicSortColumn(sortBy string) string {
	switch sortBy {
	case "created_at":
		return "created_at"
	case "updated_at":
		return "updated_at"
	default:
		return "name"
	}
}
