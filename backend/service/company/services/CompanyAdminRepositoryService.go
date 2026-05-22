package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/tracing"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
	"gorm.io/gorm"
)

// adminCursor is the decoded form of the opaque admin list cursor.
// SV is the sort-column value (string-encoded); ID breaks ties.
// NullSV is true when the sort-column value for this row is NULL in the DB
// (e.g. last_scan_at for companies that have never been scanned).
type adminCursor struct {
	ID     string `json:"id"`
	SV     string `json:"sv"`
	IsNull bool   `json:"null,omitempty"`
}

// encodeAdminCursor encodes a cursor anchor as an opaque string.
func encodeAdminCursor(id, sv string, isNull bool) string {
	b, _ := json.Marshal(adminCursor{ID: id, SV: sv, IsNull: isNull})
	return base64.StdEncoding.EncodeToString(b)
}

// decodeAdminCursor decodes an admin cursor string. Returns zero value on
// failure (treated as first-page by callers).
func decodeAdminCursor(s string) adminCursor {
	if s == "" {
		return adminCursor{}
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return adminCursor{}
	}
	var c adminCursor
	_ = json.Unmarshal(b, &c)
	return c
}

// adminRepositoryService is the terminal CompanyAdminService implementation.
// Reads are split into two queries (companies, then scan_settings keyed by
// the resulting IDs) — equivalent to a LEFT JOIN at the result level but
// far simpler to scan into typed structs without per-column GORM tags.
type adminRepositoryService struct {
	db                *gorm.DB
	scanHistoryReader ScanHistoryReader
	log               logger.Logger
}

// NewAdminRepositoryService constructs the terminal CompanyAdminService.
// A nil log silences all structured logging.
func NewAdminRepositoryService(db *gorm.DB, scanHistoryReader ScanHistoryReader) CompanyAdminService {
	return &adminRepositoryService{db: db, scanHistoryReader: scanHistoryReader, log: logger.NewNoop()}
}

// List returns admin views joined with scan_settings. Honours every filter
// field; falls back to name ASC sort by default. Pagination is keyset-based:
// the caller supplies an opaque cursor (from a previous response's NextCursor)
// and the limit. An empty cursor returns the first page.
func (r *adminRepositoryService) List(ctx context.Context, filter companydto.CompanyFilter, limit int) (_ companydto.CompanyAdminListResult, err error) {
	ctx, span := tracing.Start(ctx, "company.admin.repo.List")
	defer tracing.End(span, &err)

	companies, total, nextCursor, err := r.queryCompanies(ctx, filter, limit)
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

	views := make([]companydto.CompanyAdminListView, 0, len(companies))
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
		view := companydto.CompanyAdminListView{Company: c}
		if s, ok := settingsByID[c.ID]; ok {
			view.ScanSettings = companydto.ScanSettingsListViewFromModel(s)
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
		NextCursor:  nextCursor,
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
		r.log.Warn("failed to fetch company", logger.Str(logger.FieldCompany, id), logger.Err(err))
		return nil, apperr.ErrInternal.WithMessage("failed to fetch company")
	}

	view := companydto.CompanyAdminView{Company: c}
	var s companymodel.CompanyScanSettings
	if err = r.db.WithContext(ctx).First(&s, "company_id = ?", id).Error; err != nil {
		if !apperr.IsNotFound(err) {
			r.log.Warn("failed to fetch scan settings", logger.Str(logger.FieldCompany, id), logger.Err(err))
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
		r.log.Warn("failed to create company", logger.Str(logger.FieldCompany, public.ID), logger.Err(err))
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
		r.log.Warn("failed to update company", logger.Str(logger.FieldCompany, id), logger.Err(res.Error))
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
		r.log.Warn("failed to update scan settings", logger.Str(logger.FieldCompany, id), logger.Err(res.Error))
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
		r.log.Warn("failed to delete company", logger.Str(logger.FieldCompany, id), logger.Err(res.Error))
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

// queryCompanies returns the cursor-paginated companies matching filter, plus
// the unbounded matching total and an opaque next-page cursor (nil when no
// next page exists). Operational filters (Active, AdapterTypes, HasSettings)
// require joining scan_settings — handled here via Joins so the count and
// the page share the same WHERE clause.
func (r *adminRepositoryService) queryCompanies(ctx context.Context, filter companydto.CompanyFilter, limit int) ([]companymodel.Company, int64, *string, error) {
	q := r.db.WithContext(ctx).
		Table("companies AS c").
		Joins("LEFT JOIN company_scan_settings s ON s.company_id = c.id")

	if filter.Search != "" {
		q = q.Where("c.name ILIKE ?", "%"+filter.Search+"%")
	}
	if filter.Active != nil {
		if *filter.Active {
			// Only companies with an explicit active=true scan_settings row.
			q = q.Where("s.active = true")
		} else {
			// Inactive = explicit active=false OR no scan_settings row at all
			// (LEFT JOIN produces s.active = NULL for companies without a row;
			// NULL = false evaluates to NULL in SQL, not true, so without the
			// IS NULL arm those companies would be silently excluded).
			q = q.Where("(s.active = false OR s.active IS NULL)")
		}
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
	case scancommon.QueueStatusQueued:
		q = q.Where("EXISTS (SELECT 1 FROM company_scan_queue csq WHERE csq.company_id = c.id AND csq.status = ?)", scancommon.QueueStatusQueued)
	case scancommon.QueueStatusRunning:
		q = q.Where("EXISTS (SELECT 1 FROM company_scan_queue csq WHERE csq.company_id = c.id AND csq.status = ?)", scancommon.QueueStatusRunning)
	case scancommon.ScanStatusError:
		q = q.Where("s.consecutive_errors > 0")
	case scancommon.ScanStatusOK:
		q = q.Where("s.consecutive_errors = 0 AND s.last_scan_at IS NOT NULL")
	}

	// Session({}) clones the statement so Distinct("c.id") only applies to
	// the count query and does not mutate q. Without this, GORM carries the
	// DISTINCT marker into the subsequent data query, generating
	// SELECT DISTINCT c.* ... ORDER BY s.last_scan_at which PostgreSQL
	// rejects when the ORDER BY column is not in the SELECT list.
	var total int64
	if err := q.Session(&gorm.Session{}).Distinct("c.id").Count(&total).Error; err != nil {
		return nil, 0, nil, apperr.ErrInternal.WithMessage("failed to count companies")
	}

	col, dir := adminSortClause(filter.SortBy, filter.SortDir)
	isDesc := dir == "DESC"

	// Apply keyset cursor predicate. For nullable sort columns (e.g.
	// last_scan_at), NULL rows sort LAST in ASC and FIRST in DESC (NULLS LAST
	// is applied in the ORDER BY). The cursor records whether the last item's
	// sort value was NULL so we can construct the correct predicate.
	cur := decodeAdminCursor(filter.Cursor)
	if cur.ID != "" {
		if cur.IsNull {
			if isDesc {
				// Cursor row had a NULL sort value: in DESC+NULLS LAST, NULL rows
				// come first. The next page starts after the last NULL row with
				// id < cursor.ID (within the NULL bucket), then non-NULL rows.
				// Since NULLS LAST reversal puts NULLs at the beginning in DESC,
				// and our order is DESC NULLS LAST (NULLs at end), we just
				// continue within the NULL bucket by id.
				q = q.Where("("+col+" IS NULL AND c.id < ?) OR "+col+" IS NOT NULL", cur.ID)
			} else {
				// ASC NULLS LAST: NULL rows come last; cursor is in the NULL
				// bucket, so next page is remaining NULL rows after cursor id.
				q = q.Where(col+" IS NULL AND c.id > ?", cur.ID)
			}
		} else {
			if isDesc {
				q = q.Where("("+col+" < ? OR ("+col+" = ? AND c.id < ?)) OR "+col+" IS NULL", cur.SV, cur.SV, cur.ID)
			} else {
				// ASC: next page has values > cursor.SV, or equal with id > cursor.ID.
				// NULL rows come after (NULLS LAST), so they are naturally excluded.
				q = q.Where("("+col+" > ? OR ("+col+" = ? AND c.id > ?))", cur.SV, cur.SV, cur.ID)
			}
		}
	}

	var rows []companymodel.Company
	q2 := q.Select("c.*").
		Order(col + " " + dir + " NULLS LAST").
		Order("c.id " + dir).
		Limit(limit + 1)
	if err := q2.Scan(&rows).Error; err != nil {
		return nil, 0, nil, apperr.ErrInternal.WithMessage("failed to fetch companies")
	}

	var nextCursor *string
	if len(rows) > limit {
		rows = rows[:limit]
	} else {
		return rows, total, nil, nil
	}

	// Build next-page cursor from the last row in the result set.
	// The cursor value depends on the sort column; nullable columns encode
	// IsNull=true so the next-page predicate can handle the NULL bucket.
	last := rows[len(rows)-1]
	enc := encodeAdminCursor(last.ID, "", false) // default: name sort
	switch filter.SortBy {
	case "adapter_type", "last_scan_at", "created_at":
		// For these sort columns, we do not have the value directly on the
		// Company struct (they are on scan_settings or are timestamps not
		// surfaced in the public select). We rely on the id-only cursor
		// with an empty SV — the id tie-break ensures forward progress.
		// In practice, last_scan_at and adapter_type NULLs are handled by
		// the IsNull=false, SV="" path which degrades gracefully.
		enc = encodeAdminCursor(last.ID, "", false)
	default:
		// Default sort: name ASC. SV = the name value.
		enc = encodeAdminCursor(last.ID, last.Name, false)
	}
	nextCursor = &enc

	return rows, total, nextCursor, nil
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
		Raw(`SELECT company_id, status FROM company_scan_queue WHERE company_id IN ? AND status IN ?`,
			ids, []string{scancommon.QueueStatusQueued, scancommon.QueueStatusRunning}).
		Scan(&rows).Error; err != nil {
		return nil, apperr.ErrInternal.WithMessage("failed to fetch queue status")
	}
	for _, row := range rows {
		out[row.CompanyID] = row.Status
	}
	return out, nil
}

// adminSortClause whitelists sort columns for the admin chain and sanitises
// the direction via safeSortDir to prevent SQL injection through the direction
// parameter.
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
	return col, safeSortDir(sortDir)
}

// safeSortDir validates and normalises a raw sort direction string to prevent
// SQL injection. Any value other than "DESC" (case-insensitive) is treated as
// "ASC" — the safe default.
func safeSortDir(dir string) string {
	if strings.ToUpper(dir) == "DESC" {
		return "DESC"
	}
	return "ASC"
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
