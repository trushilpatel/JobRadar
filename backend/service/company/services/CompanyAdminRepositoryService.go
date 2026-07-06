package services

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/tracing"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
func NewAdminRepositoryService(db *gorm.DB, scanHistoryReader ScanHistoryReader, log logger.Logger) CompanyAdminService {
	if log == nil {
		log = logger.NewNoop()
	}
	return &adminRepositoryService{db: db, scanHistoryReader: scanHistoryReader, log: log}
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
		r.log.Warn("failed to count active scan settings", logger.Err(err))
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
		r.log.Warn("failed to fetch scan histories", logger.Err(err))
		return companydto.CompanyAdminListResult{}, apperr.ErrInternal.WithMessage("failed to fetch scan histories")
	}

	for _, c := range companies {
		view := companydto.CompanyAdminListView{CompanyAdminCommon: companydto.CompanyAdminCommon{Company: c}}
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

	view := companydto.CompanyAdminView{CompanyAdminCommon: companydto.CompanyAdminCommon{Company: c}}
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
		company.Website = public.Website
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
		// Classify the failure so callers get an actionable status code:
		//   - unique violation (duplicate id/slug)      → 409 Conflict
		//   - FK violation (adapter_type not registered) → 400 Bad Request
		//   - anything else                              → 500 Internal
		switch {
		case isUniqueViolation(err):
			r.log.Warn("company create conflict", logger.Str(logger.FieldCompany, public.ID), logger.Err(err))
			return nil, apperr.ErrConflict.WithMessage("company id or slug already exists")
		case isForeignKeyViolation(err):
			r.log.Warn("company create FK violation",
				logger.Str(logger.FieldCompany, public.ID),
				logger.Str(logger.FieldAdapterType, adapterTypeOf(settings)),
				logger.Err(err))
			return nil, apperr.ErrBadRequest.WithMessage("adapter_type not registered")
		default:
			r.log.Warn("failed to create company", logger.Str(logger.FieldCompany, public.ID), logger.Err(err))
			return nil, apperr.ErrInternal.WithMessage("failed to create company")
		}
	}

	return &companydto.CompanyAdminView{
		CompanyAdminCommon: companydto.CompanyAdminCommon{Company: company},
		ScanSettings:       companydto.ScanSettingsViewFromModel(settingsRow),
	}, nil
}

// adapterTypeOf returns the adapter_type string from a scan-settings input,
// or empty when none was supplied. Used only to enrich error logs.
func adapterTypeOf(s companydto.CompanyScanSettingsInput) string {
	if s.AdapterType != nil {
		return *s.AdapterType
	}
	return ""
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
//
// The whole read-compare-write sequence runs inside one transaction that
// locks the row up front (`SELECT ... FOR UPDATE` on Postgres, via
// clause.Locking — a documented no-op on sqlite, which is what the test
// suite runs against). This closes a TOCTOU race: without the lock, two
// concurrent admin patches (or an admin patch racing a worker's write to the
// same row) could each decide whether adapter_type "changed" based on a
// stale read, corrupting the failure-history reset below. Holding the lock
// for the transaction's full duration forces any other writer targeting this
// row to wait until this transaction commits.
func (r *adminRepositoryService) UpdateScanSettings(ctx context.Context, id string, p companydto.CompanyScanSettingsInput) (err error) {
	if p.IsEmpty() {
		return apperr.ErrBadRequest.WithMessage("at least one field must be provided")
	}
	ctx, span := tracing.Start(ctx, "company.admin.repo.UpdateScanSettings")
	defer tracing.End(span, &err)

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current companymodel.CompanyScanSettings
		if lockErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("adapter_type").
			Where("company_id = ?", id).
			Take(&current).Error; lockErr != nil {
			if apperr.IsNotFound(lockErr) {
				return apperr.ErrNotFound.WithMessage("scan settings not found")
			}
			return lockErr
		}

		fields := scanSettingsInputToFields(p)
		// Only a genuine adapter_type change (present in this patch AND
		// different from what's locked-in-place above) resets the failure
		// history. The new adapter has no relationship to the old one's
		// error streak — carrying it over would misrepresent the new
		// adapter's health. An explicit AutoPaused value in the SAME patch
		// (already captured in `fields` by scanSettingsInputToFields above)
		// wins over this derived value, so re-apply it last if present.
		if p.AdapterType != nil && *p.AdapterType != current.AdapterType {
			fields["consecutive_errors"] = 0
			fields["last_error_at"] = nil
			fields["last_error_message"] = nil
			fields["auto_paused"] = false
			if p.AutoPaused != nil {
				fields["auto_paused"] = *p.AutoPaused
			}
		}

		res := tx.Model(&companymodel.CompanyScanSettings{}).
			Where("company_id = ?", id).
			Updates(fields)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return apperr.ErrNotFound.WithMessage("scan settings not found")
		}
		return nil
	})
	if err != nil {
		var se *apperr.ServiceError
		if errors.As(err, &se) {
			return err
		}
		r.log.Warn("failed to update scan settings",
			logger.Str(logger.FieldCompany, id),
			logger.Str(logger.FieldAdapterType, adapterTypeFromInput(p)),
			logger.Err(err))
		return apperr.ErrInternal.WithMessage("failed to update scan settings")
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

	// No RowsAffected check: this is a global toggle, so matching 0 rows
	// (an empty company_scan_settings table) is a legitimate no-op, not a
	// missing-row error.
	if err = r.db.WithContext(ctx).
		Model(&companymodel.CompanyScanSettings{}).
		Where("1 = 1").
		Update("active", active).Error; err != nil {
		r.log.Warn("failed to set all scan settings active", logger.Bool(logger.FieldActive, active), logger.Err(err))
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
		view := companydto.CompanyAdminView{CompanyAdminCommon: companydto.CompanyAdminCommon{Company: c}}
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
		view := companydto.CompanyAdminView{CompanyAdminCommon: companydto.CompanyAdminCommon{Company: c}}
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
		r.log.Warn("failed to count companies", logger.Err(err))
		return nil, 0, nil, apperr.ErrInternal.WithMessage("failed to count companies")
	}

	col, dir := adminSortClause(filter.SortBy, filter.SortDir)
	isDesc := dir == "DESC"

	// Apply keyset cursor predicate. For nullable sort columns (e.g.
	// last_scan_at), NULL rows sort LAST in ASC and FIRST in DESC (NULLS LAST
	// is applied in the ORDER BY). The cursor records whether the last item's
	// sort value was NULL so we can construct the correct predicate.
	// The ORDER BY is always `col <dir> NULLS LAST, c.id <dir>` — so the NULL
	// bucket is ALWAYS the final bucket, regardless of direction. The keyset
	// predicate therefore has the same shape for ASC and DESC, differing only
	// in the comparison operator and the id tie-break direction.
	//
	// The comparison uses CAST(col AS TEXT): the cursor SV is the projected,
	// text-cast value of the previous page's last row (see the SELECT below),
	// so both sides of the comparison share one textual form. Comparing the
	// raw column against a text literal instead would mis-order on drivers
	// whose column text differs from their literal text (e.g. sqlite
	// timestamps), skipping or duplicating the page boundary.
	colText := "CAST(" + col + " AS TEXT)"
	cur := decodeAdminCursor(filter.Cursor)
	if cur.ID != "" {
		cmp := ">"
		if isDesc {
			cmp = "<"
		}
		if cur.IsNull {
			// Cursor row is in the final (NULL) bucket. Only remaining NULL
			// rows, ordered by id, can follow it.
			q = q.Where(col+" IS NULL AND c.id "+cmp+" ?", cur.ID)
		} else {
			// Cursor row is a non-NULL row. The next page is: rows with a
			// strictly-further value, OR equal value with a further id, OR the
			// entire NULL bucket (which always sorts after every non-NULL row).
			q = q.Where(
				"("+colText+" "+cmp+" ? OR ("+colText+" = ? AND c.id "+cmp+" ?)) OR "+col+" IS NULL",
				cur.SV, cur.SV, cur.ID)
		}
	}

	// Project the actual sort column alongside c.* so the next-page cursor can
	// encode the real sort value (and its NULL-ness) for EVERY sortable column,
	// not just name. Without this, paging by adapter_type / last_scan_at /
	// created_at would compare the column against an empty cursor value and
	// skip or duplicate rows. sortVal/sortNull are scanned from the aliased
	// projection; for the name sort they mirror c.name.
	var rows []adminCompanyRow
	q2 := q.Select("c.*, " + colText + " AS sort_val, (" + col + " IS NULL) AS sort_null").
		Order(col + " " + dir + " NULLS LAST").
		Order("c.id " + dir).
		Limit(limit + 1)
	if err := q2.Scan(&rows).Error; err != nil {
		r.log.Warn("failed to fetch companies", logger.Err(err))
		return nil, 0, nil, apperr.ErrInternal.WithMessage("failed to fetch companies")
	}

	companies := make([]companymodel.Company, 0, len(rows))
	hasNext := len(rows) > limit
	if hasNext {
		rows = rows[:limit]
	}
	for i := range rows {
		companies = append(companies, rows[i].Company)
	}
	if !hasNext {
		return companies, total, nil, nil
	}

	// Build next-page cursor from the last projected row: encode the real sort
	// value + IsNull so the next-page predicate (above) handles the NULL bucket
	// correctly for nullable columns.
	last := rows[len(rows)-1]
	enc := encodeAdminCursor(last.ID, sortValueString(last.SortVal), last.SortNull)

	return companies, total, &enc, nil
}

// adminCompanyRow is the scan target for the paginated page query: the public
// Company columns plus the projected sort value (SortVal) and its NULL-ness
// (SortNull). Keeping the projection in a dedicated struct keeps cursor
// construction correct for every sortable column without per-column GORM tags
// leaking onto the Company model.
type adminCompanyRow struct {
	companymodel.Company
	SortVal  sql.NullString `gorm:"column:sort_val"`
	SortNull bool           `gorm:"column:sort_null"`
}

// sortValueString renders a scanned sort value into the cursor's string form.
// A NULL value renders empty — the cursor's IsNull flag (not this string) is
// what drives the NULL-bucket predicate, so the empty string is never compared
// against a real column for NULL rows. timestamps come back from Postgres in a
// lexicographically-orderable text form, so string comparison preserves order.
func sortValueString(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
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
		r.log.Warn("failed to fetch scan settings", logger.Err(err))
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
		r.log.Warn("failed to fetch queue status", logger.Err(err))
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
