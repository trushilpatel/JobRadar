package services

import (
	"context"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	"gorm.io/gorm"
)

// adminRepositoryService is the terminal CompanyAdminService implementation.
// Reads are split into two queries (companies, then scan_settings keyed by
// the resulting IDs) — equivalent to a LEFT JOIN at the result level but
// far simpler to scan into typed structs without per-column GORM tags.
type adminRepositoryService struct {
	db                *gorm.DB
	scanHistoryReader companydto.ScanHistoryReader
}

// NewAdminRepositoryService constructs the terminal CompanyAdminService.
func NewAdminRepositoryService(db *gorm.DB, scanHistoryReader companydto.ScanHistoryReader) CompanyAdminService {
	return &adminRepositoryService{db: db, scanHistoryReader: scanHistoryReader}
}

// List returns admin views joined with scan_settings. Honours every filter
// field; falls back to name ASC sort by default.
func (r *adminRepositoryService) List(ctx context.Context, filter companydto.CompanyFilter, page, limit int) (_ companydto.CompanyAdminListResult, err error) {
	ctx, span := tracing.Start(ctx, "company.admin.repo.List")
	defer tracing.End(span, &err)

	companies, total, err := r.queryCompanies(ctx, filter, page, limit)
	if err != nil {
		return companydto.CompanyAdminListResult{}, err
	}
	settingsByID, err := r.fetchSettingsForCompanies(ctx, companies)
	if err != nil {
		return companydto.CompanyAdminListResult{}, err
	}

	var totalActive int64
	if err = r.db.WithContext(ctx).
		Raw(`SELECT COUNT(*) FROM company_scan_settings WHERE active = true`).
		Scan(&totalActive).Error; err != nil {
		return companydto.CompanyAdminListResult{}, apperr.ErrInternal.WithMessage("failed to count active scan settings")
	}

	views := make([]companydto.CompanyAdminView, 0, len(companies))
	ids := make([]string, 0, len(companies))
	for _, c := range companies {
		ids = append(ids, c.ID)
	}

	queueStatusByID, err := r.fetchQueueStatusForCompanies(ctx, companies)
	if err != nil {
		return companydto.CompanyAdminListResult{}, err
	}

	historiesByID, err := r.scanHistoryReader.GetScanHistories(ctx, ids)
	if err != nil {
		return companydto.CompanyAdminListResult{}, apperr.ErrInternal.WithMessage("failed to fetch scan histories")
	}

	for _, c := range companies {
		view := companydto.CompanyAdminView{Company: c}
		if s, ok := settingsByID[c.ID]; ok {
			view.ScanSettings = companydto.ScanSettingsViewFromModel(s)
		}
		if qs, ok := queueStatusByID[c.ID]; ok {
			view.QueueStatus = &qs
		}
		if h, ok := historiesByID[c.ID]; ok {
			view.ScanHistory = h
		}
		views = append(views, view)
	}
	return companydto.CompanyAdminListResult{
		Items:       views,
		Total:       total,
		TotalActive: totalActive,
	}, nil
}

// Get returns a single admin view, NOT_FOUND when the company is missing.
// Issues two reads: one for the company row, one for the (optional)
// scan_settings row.
func (r *adminRepositoryService) Get(ctx context.Context, id string) (_ *companydto.CompanyAdminView, err error) {
	ctx, span := tracing.Start(ctx, "company.admin.repo.Get")
	defer tracing.End(span, &err)

	var c companymodel.Company
	if err = r.db.WithContext(ctx).First(&c, "id = ?", id).Error; err != nil {
		if apperr.IsNotFound(err) {
			return nil, apperr.ErrNotFound.WithMessage("company not found")
		}
		return nil, apperr.ErrInternal.WithMessage("failed to fetch company")
	}

	view := companydto.CompanyAdminView{Company: c}
	var s companymodel.CompanyScanSettings
	if err = r.db.WithContext(ctx).First(&s, "company_id = ?", id).Error; err != nil {
		if !apperr.IsNotFound(err) {
			return nil, apperr.ErrInternal.WithMessage("failed to fetch scan settings")
		}
	} else {
		view.ScanSettings = companydto.ScanSettingsViewFromModel(&s)
	}
	return &view, nil
}

// Create inserts a `companies` row plus an optional `company_scan_settings`
// row in a single transaction. Returns the resulting admin view (settings
// is nil when the input was empty).
func (r *adminRepositoryService) Create(ctx context.Context, public companydto.CompanyPublicInput, settings companydto.CompanyScanSettingsInput) (_ *companydto.CompanyAdminView, err error) {
	ctx, span := tracing.Start(ctx, "company.admin.repo.Create")
	defer tracing.End(span, &err)

	company := companymodel.Company{
		ID:   public.ID,
		Name: public.Name,
		Slug: public.Slug,
	}
	if public.Website != nil {
		company.Website = *public.Website
	}
	if public.CareersURL != nil {
		company.CareersURL = public.CareersURL
	}
	if public.LogoURL != nil {
		company.LogoURL = public.LogoURL
	}

	var settingsRow *companymodel.CompanyScanSettings
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if cerr := tx.Create(&company).Error; cerr != nil {
			return cerr
		}
		if !settings.IsEmpty() {
			row := companymodel.CompanyScanSettings{CompanyID: company.ID, Active: true}
			if settings.AdapterType != nil {
				row.AdapterType = *settings.AdapterType
			}
			if settings.AdapterConfig != nil {
				row.AdapterConfig = *settings.AdapterConfig
			}
			if settings.ScanIntervalOverride != nil {
				row.ScanIntervalOverride = settings.ScanIntervalOverride
			}
			if settings.Active != nil {
				row.Active = *settings.Active
			}
			if cerr := tx.Create(&row).Error; cerr != nil {
				return cerr
			}
			settingsRow = &row
		}
		return nil
	})
	if err != nil {
		return nil, apperr.ErrInternal.WithMessage("failed to create company")
	}

	return &companydto.CompanyAdminView{
		Company:      company,
		ScanSettings: companydto.ScanSettingsViewFromModel(settingsRow),
	}, nil
}

// UpdatePublic patches the `companies` row only.
func (r *adminRepositoryService) UpdatePublic(ctx context.Context, id string, p companydto.CompanyPublicInput) (err error) {
	ctx, span := tracing.Start(ctx, "company.admin.repo.UpdatePublic")
	defer tracing.End(span, &err)

	fields := publicInputToFields(p)
	if len(fields) == 0 {
		return apperr.ErrBadRequest.WithMessage("at least one field must be provided")
	}
	res := r.db.WithContext(ctx).
		Model(&companymodel.Company{}).
		Where("id = ?", id).
		Updates(fields)
	if res.Error != nil {
		return apperr.ErrInternal.WithMessage("failed to update company")
	}
	if res.RowsAffected == 0 {
		return apperr.ErrNotFound.WithMessage("company not found")
	}
	return nil
}

// UpdateScanSettings patches the `company_scan_settings` row.
func (r *adminRepositoryService) UpdateScanSettings(ctx context.Context, id string, p companydto.CompanyScanSettingsInput) (err error) {
	if p.IsEmpty() {
		return apperr.ErrBadRequest.WithMessage("at least one field must be provided")
	}
	ctx, span := tracing.Start(ctx, "company.admin.repo.UpdateScanSettings")
	defer tracing.End(span, &err)

	fields := scanSettingsInputToFields(p)
	res := r.db.WithContext(ctx).
		Model(&companymodel.CompanyScanSettings{}).
		Where("company_id = ?", id).
		Updates(fields)
	if res.Error != nil {
		return apperr.ErrInternal.WithMessage("failed to update scan settings")
	}
	if res.RowsAffected == 0 {
		return apperr.ErrNotFound.WithMessage("scan settings not found")
	}
	return nil
}

// Delete removes the company row. ON DELETE CASCADE removes the matching
// scan_settings row.
func (r *adminRepositoryService) Delete(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "company.admin.repo.Delete")
	defer tracing.End(span, &err)

	res := r.db.WithContext(ctx).Delete(&companymodel.Company{}, "id = ?", id)
	if res.Error != nil {
		return apperr.ErrInternal.WithMessage("failed to delete company")
	}
	if res.RowsAffected == 0 {
		return apperr.ErrNotFound.WithMessage("company not found")
	}
	return nil
}

// SetAllActive toggles `active` across every scan_settings row.
func (r *adminRepositoryService) SetAllActive(ctx context.Context, active bool) (err error) {
	ctx, span := tracing.Start(ctx, "company.admin.repo.SetAllActive")
	defer tracing.End(span, &err)

	if err = r.db.WithContext(ctx).
		Model(&companymodel.CompanyScanSettings{}).
		Where("1 = 1").
		Update("active", active).Error; err != nil {
		return apperr.ErrInternal.WithMessage("failed to update scan settings")
	}
	return nil
}

// GetByIDs returns admin views for the given IDs. Returns an empty slice
// (not nil) when ids is empty.
func (r *adminRepositoryService) GetByIDs(ctx context.Context, ids []string) (_ []companydto.CompanyAdminView, err error) {
	ctx, span := tracing.Start(ctx, "company.admin.repo.GetByIDs")
	defer tracing.End(span, &err)

	if len(ids) == 0 {
		return []companydto.CompanyAdminView{}, nil
	}

	var companies []companymodel.Company
	if err = r.db.WithContext(ctx).Where("id IN ?", ids).Find(&companies).Error; err != nil {
		return nil, apperr.ErrInternal.WithMessage("failed to fetch companies")
	}
	settingsByID, err := r.fetchSettingsForCompanies(ctx, companies)
	if err != nil {
		return nil, err
	}
	views := make([]companydto.CompanyAdminView, 0, len(companies))
	for _, c := range companies {
		view := companydto.CompanyAdminView{Company: c}
		if s, ok := settingsByID[c.ID]; ok {
			view.ScanSettings = companydto.ScanSettingsViewFromModel(s)
		}
		views = append(views, view)
	}
	return views, nil
}

// GetAll returns every company as an admin view (workers consume this).
func (r *adminRepositoryService) GetAll(ctx context.Context) (_ []companydto.CompanyAdminView, err error) {
	ctx, span := tracing.Start(ctx, "company.admin.repo.GetAll")
	defer tracing.End(span, &err)

	var companies []companymodel.Company
	if err = r.db.WithContext(ctx).Order("name ASC").Find(&companies).Error; err != nil {
		return nil, apperr.ErrInternal.WithMessage("failed to fetch companies")
	}
	settingsByID, err := r.fetchSettingsForCompanies(ctx, companies)
	if err != nil {
		return nil, err
	}
	views := make([]companydto.CompanyAdminView, 0, len(companies))
	for _, c := range companies {
		view := companydto.CompanyAdminView{Company: c}
		if s, ok := settingsByID[c.ID]; ok {
			view.ScanSettings = companydto.ScanSettingsViewFromModel(s)
		}
		views = append(views, view)
	}
	return views, nil
}

// queryCompanies returns the paginated companies matching filter, plus the
// unbounded matching total. Operational filters (Active, AdapterTypes,
// HasSettings) require joining scan_settings — handled here via Joins so
// the count and the page share the same WHERE clause.
func (r *adminRepositoryService) queryCompanies(ctx context.Context, filter companydto.CompanyFilter, page, limit int) ([]companymodel.Company, int64, error) {
	q := r.db.WithContext(ctx).
		Table("companies AS c").
		Joins("LEFT JOIN company_scan_settings s ON s.company_id = c.id")

	if filter.Search != "" {
		q = q.Where("c.name ILIKE ?", "%"+filter.Search+"%")
	}
	if filter.Active != nil {
		q = q.Where("s.active = ?", *filter.Active)
	}
	if len(filter.AdapterTypes) > 0 {
		q = q.Where("s.adapter_type IN ?", filter.AdapterTypes)
	}
	if filter.HasSettings != nil {
		if *filter.HasSettings {
			q = q.Where("s.company_id IS NOT NULL")
		} else {
			q = q.Where("s.company_id IS NULL")
		}
	}
	switch filter.ScanStatus {
	case "queued":
		q = q.Where("EXISTS (SELECT 1 FROM company_scan_queue csq WHERE csq.company_id = c.id AND csq.status = 'queued')")
	case "running":
		q = q.Where("EXISTS (SELECT 1 FROM company_scan_queue csq WHERE csq.company_id = c.id AND csq.status = 'running')")
	case "error":
		q = q.Where("s.consecutive_errors > 0")
	case "ok":
		q = q.Where("s.consecutive_errors = 0 AND s.last_scan_at IS NOT NULL")
	}

	var total int64
	if err := q.Distinct("c.id").Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternal.WithMessage("failed to count companies")
	}

	col, dir := adminSortClause(filter.SortBy, filter.SortDir)
	offset := max((page-1)*limit, 0)

	var rows []companymodel.Company
	q2 := q.Select("c.*").Order(col + " " + dir + " NULLS LAST")
	if limit > 0 {
		q2 = q2.Limit(limit).Offset(offset)
	}
	if err := q2.Scan(&rows).Error; err != nil {
		return nil, 0, apperr.ErrInternal.WithMessage("failed to fetch companies")
	}
	return rows, total, nil
}

// fetchSettingsForCompanies returns a {company_id → *CompanyScanSettings}
// map for the provided companies. Empty input returns an empty map; missing
// settings rows are omitted (callers treat absence as "no scan_settings").
func (r *adminRepositoryService) fetchSettingsForCompanies(ctx context.Context, companies []companymodel.Company) (map[string]*companymodel.CompanyScanSettings, error) {
	out := map[string]*companymodel.CompanyScanSettings{}
	if len(companies) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(companies))
	for _, c := range companies {
		ids = append(ids, c.ID)
	}
	var settings []companymodel.CompanyScanSettings
	if err := r.db.WithContext(ctx).
		Where("company_id IN ?", ids).
		Find(&settings).Error; err != nil {
		return nil, apperr.ErrInternal.WithMessage("failed to fetch scan settings")
	}
	for i := range settings {
		s := settings[i]
		out[s.CompanyID] = &s
	}
	return out, nil
}

// fetchQueueStatusForCompanies returns a {company_id → status} map for
// companies that currently have a queued or running row in company_scan_queue.
// Companies not in the queue are omitted from the map.
func (r *adminRepositoryService) fetchQueueStatusForCompanies(ctx context.Context, companies []companymodel.Company) (map[string]string, error) {
	out := map[string]string{}
	if len(companies) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(companies))
	for _, c := range companies {
		ids = append(ids, c.ID)
	}
	var rows []struct {
		CompanyID string `gorm:"column:company_id"`
		Status    string `gorm:"column:status"`
	}
	if err := r.db.WithContext(ctx).
		Raw(`SELECT company_id, status FROM company_scan_queue WHERE company_id IN ? AND status IN ('queued','running')`, ids).
		Scan(&rows).Error; err != nil {
		return nil, apperr.ErrInternal.WithMessage("failed to fetch queue status")
	}
	for _, row := range rows {
		out[row.CompanyID] = row.Status
	}
	return out, nil
}

// adminSortClause whitelists sort columns for the admin chain.
func adminSortClause(sortBy, sortDir string) (col, dir string) {
	switch sortBy {
	case "adapter_type":
		col = "s.adapter_type"
	case "last_scan_at":
		col = "s.last_scan_at"
	case "created_at":
		col = "c.created_at"
	default:
		col = "c.name"
	}
	dir = "ASC"
	if sortDir == "desc" {
		dir = "DESC"
	}
	return col, dir
}

// publicInputToFields projects a public-input partial-patch into a GORM
// field map. Required fields (Name/Slug) overwrite when non-empty so admin
// rename + slug-fix workflows stay one PATCH each.
func publicInputToFields(p companydto.CompanyPublicInput) map[string]any {
	fields := map[string]any{}
	if p.Name != "" {
		fields["name"] = p.Name
	}
	if p.Slug != "" {
		fields["slug"] = p.Slug
	}
	if p.Website != nil {
		fields["website"] = *p.Website
	}
	if p.CareersURL != nil {
		fields["careers_url"] = *p.CareersURL
	}
	if p.LogoURL != nil {
		fields["logo_url"] = *p.LogoURL
	}
	return fields
}
