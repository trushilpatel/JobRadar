package services

import (
	"context"
	"encoding/base64"
	"encoding/json"

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

// publicCursor is the decoded form of the opaque cursor string passed by
// callers for keyset pagination. The sort column value (SV) is the anchor
// for the WHERE clause; ID breaks ties when SV values are equal.
type publicCursor struct {
	ID string `json:"id"`
	SV string `json:"sv"`
}

// encodePublicCursor base64-encodes a JSON cursor anchor.
func encodePublicCursor(id, sortValue string) string {
	b, _ := json.Marshal(publicCursor{ID: id, SV: sortValue})
	return base64.StdEncoding.EncodeToString(b)
}

// decodePublicCursor decodes a cursor string. Returns zero value on failure
// (treated as first-page by callers).
func decodePublicCursor(s string) publicCursor {
	if s == "" {
		return publicCursor{}
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return publicCursor{}
	}
	var c publicCursor
	_ = json.Unmarshal(b, &c)
	return c
}

// ListPublic returns a filtered, cursor-paginated page of public company views.
// The Active / AdapterTypes / HasSettings filter fields are intentionally
// IGNORED in the public chain — those are operational concerns the public
// API must not expose. Only Search and the Sort fields are honoured.
//
// Cursor encoding: base64(json({id, sv})) where sv is the sort-column value
// of the last item on the previous page. The first page is requested with an
// empty cursor.
func (r *publicRepositoryService) ListPublic(ctx context.Context, filter companydto.CompanyFilter, limit int) (_ companydto.CompanyPublicListResult, err error) {
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
	dir := safeSortDir(filter.SortDir)
	isDesc := dir == "DESC"

	// Apply keyset cursor predicate.
	cur := decodePublicCursor(filter.Cursor)
	if cur.ID != "" {
		if isDesc {
			// DESC: next page has values strictly less than the cursor, or equal
			// with id strictly less than the cursor id.
			q = q.Where("("+col+" < ? OR ("+col+" = ? AND id < ?))", cur.SV, cur.SV, cur.ID)
		} else {
			// ASC: next page has values strictly greater than the cursor, or equal
			// with id strictly greater than the cursor id.
			q = q.Where("("+col+" > ? OR ("+col+" = ? AND id > ?))", cur.SV, cur.SV, cur.ID)
		}
	}

	// Fetch one extra to detect whether another page exists.
	var rows []companymodel.Company
	if err = q.Order(col + " " + dir + " NULLS LAST").
		Order("id " + dir).
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return companydto.CompanyPublicListResult{}, apperr.ErrInternal.WithMessage("failed to fetch companies")
	}

	var nextCursor *string
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		enc := encodePublicCursor(last.ID, publicSortValue(last, filter.SortBy))
		nextCursor = &enc
	}

	views := make([]companydto.CompanyPublicView, 0, len(rows))
	for _, c := range rows {
		views = append(views, companydto.CompanyPublicView{Company: c})
	}
	return companydto.CompanyPublicListResult{Items: views, Total: total, NextCursor: nextCursor}, nil
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

// publicSortValue extracts the string representation of the sort-column value
// from a Company row, used to build the cursor anchor for the next page.
func publicSortValue(c companymodel.Company, sortBy string) string {
	switch sortBy {
	case "created_at":
		return c.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z")
	case "updated_at":
		return c.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z")
	default:
		return c.Name
	}
}
