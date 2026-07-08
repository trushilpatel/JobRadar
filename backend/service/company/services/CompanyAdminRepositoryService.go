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
//
// Round-trip budget: this method issues exactly 2 DB round-trips — one
// combined count+total_active query, and one page query that already carries
// each company's scan_settings and queue_status via the joins queryCompanies
// needs anyway for filtering (previously 5: separate count, page, settings,
// queue-status, and total_active queries, each paying full network latency to
// the remote Postgres instance). GetScanHistories is a 3rd, unavoidable
// round-trip: company_scan_log is owned by the scan domain and read only
// through the ScanHistoryReader interface (see company/CLAUDE.md) — folding
// it into this query would mean this package embedding scan's schema
// directly, the exact cross-domain coupling that interface exists to prevent.
func (r *adminRepositoryService) List(ctx context.Context, filter companydto.CompanyFilter, limit int) (_ companydto.CompanyAdminListResult, err error) {
	ctx, span := tracing.Start(ctx, "company.admin.repo.List")
	defer tracing.End(span, &err)

	rows, total, totalActive, nextCursor, err := r.queryCompanies(ctx, filter, limit)
	if err != nil {
		return companydto.CompanyAdminListResult{}, err
	}

	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	historiesByID, err := r.scanHistoryReader.GetScanHistories(ctx, ids)
	if err != nil {
		r.log.Warn("failed to fetch scan histories", logger.Err(err))
		return companydto.CompanyAdminListResult{}, apperr.ErrInternal.WithMessage("failed to fetch scan histories")
	}

	views := make([]companydto.CompanyAdminListView, 0, len(rows))
	for _, row := range rows {
		view := companydto.CompanyAdminListView{
			CompanyAdminCommon: companydto.CompanyAdminCommon{Company: row.Company},
			ScanSettings:       row.scanSettingsListView(),
		}
		if row.QueueStatus.Valid {
			s := row.QueueStatus.String
			view.QueueStatus = &s
		}
		if h, ok := historiesByID[row.ID]; ok {
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

// queryCompanies returns the cursor-paginated companies matching filter, each
// already carrying its scan_settings and queue_status (both already joined
// for filtering — projecting them here is free), plus the unbounded matching
// total, the global total_active, and an opaque next-page cursor (nil when no
// next page exists).
//
// Both scan_settings and company_scan_queue are joined 1:1 with companies:
// company_scan_settings.company_id is its primary key, and the queue join's
// ON clause restricts to status IN (queued, running) — the exact set
// uq_company_scan_queue_active enforces at most one row per company_id for —
// so neither join can fan out a company's row. That guarantee is what makes
// projecting their columns onto the same row (instead of N+1 follow-up
// queries) safe.
func (r *adminRepositoryService) queryCompanies(ctx context.Context, filter companydto.CompanyFilter, limit int) ([]adminCompanyJoinRow, int64, int64, *string, error) {
	q := r.db.WithContext(ctx).
		Table("companies AS c").
		Joins("LEFT JOIN company_scan_settings s ON s.company_id = c.id").
		Joins("LEFT JOIN company_scan_queue csq ON csq.company_id = c.id AND csq.status IN (?, ?)",
			scancommon.QueueStatusQueued, scancommon.QueueStatusRunning)

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
		q = q.Where("csq.status = ?", scancommon.QueueStatusQueued)
	case scancommon.QueueStatusRunning:
		q = q.Where("csq.status = ?", scancommon.QueueStatusRunning)
	case scancommon.ScanStatusError:
		q = q.Where("s.consecutive_errors > 0")
	case scancommon.ScanStatusOK:
		q = q.Where("s.consecutive_errors = 0 AND s.last_scan_at IS NOT NULL")
	}

	// Session({}) clones the statement so this Select doesn't leak into the
	// page query below. COUNT(...) always returns exactly one row (even when
	// zero companies match), so pairing total_active in as an uncorrelated
	// scalar subquery on the same row is safe regardless of match count —
	// unlike the page query, there's no "zero rows means we lose the
	// aggregate" edge case to worry about here.
	var counts struct {
		Total       int64 `gorm:"column:total"`
		TotalActive int64 `gorm:"column:total_active"`
	}
	if err := q.Session(&gorm.Session{}).
		Select("COUNT(DISTINCT c.id) AS total, " +
			"(SELECT COUNT(*) FROM company_scan_settings WHERE active = true) AS total_active").
		Scan(&counts).Error; err != nil {
		r.log.Warn("failed to count companies", logger.Err(err))
		return nil, 0, 0, nil, apperr.ErrInternal.WithMessage("failed to count companies")
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
	//
	// scan_settings and queue_status columns are aliased (ss_*, queue_status)
	// alongside c.* here — both joins already exist above for filtering, so
	// this is the same query that was already going to run; it just returns
	// the columns the two now-removed follow-up queries used to fetch
	// separately (adapter_config is deliberately excluded: the list view never
	// exposes it, so there's no reason to pull it off the wire for every row).
	var rows []adminCompanyJoinRow
	q2 := q.Select("c.*, " + colText + " AS sort_val, (" + col + " IS NULL) AS sort_null, " +
		"s.company_id AS ss_company_id, s.adapter_type AS ss_adapter_type, " +
		"s.scan_interval_override AS ss_scan_interval_override, s.active AS ss_active, " +
		"s.auto_paused AS ss_auto_paused, s.last_scan_at AS ss_last_scan_at, " +
		"s.last_success_at AS ss_last_success_at, s.last_error_at AS ss_last_error_at, " +
		"s.last_error_message AS ss_last_error_message, s.consecutive_errors AS ss_consecutive_errors, " +
		"s.created_at AS ss_created_at, s.updated_at AS ss_updated_at, " +
		"csq.status AS queue_status").
		Order(col + " " + dir + " NULLS LAST").
		Order("c.id " + dir).
		Limit(limit + 1)
	if err := q2.Scan(&rows).Error; err != nil {
		r.log.Warn("failed to fetch companies", logger.Err(err))
		return nil, 0, 0, nil, apperr.ErrInternal.WithMessage("failed to fetch companies")
	}

	hasNext := len(rows) > limit
	if hasNext {
		rows = rows[:limit]
	}
	if !hasNext {
		return rows, counts.Total, counts.TotalActive, nil, nil
	}

	// Build next-page cursor from the last projected row: encode the real sort
	// value + IsNull so the next-page predicate (above) handles the NULL bucket
	// correctly for nullable columns.
	last := rows[len(rows)-1]
	enc := encodeAdminCursor(last.ID, sortValueString(last.SortVal), last.SortNull)

	return rows, counts.Total, counts.TotalActive, &enc, nil
}

// adminCompanyJoinRow is the scan target for the paginated page query: the
// public Company columns, the projected sort value (SortVal) and its
// NULL-ness (SortNull), the credential-free scan_settings columns (ss_*
// prefix — adapter_config is intentionally never selected here, see
// queryCompanies), and queue_status. Keeping the projection in a dedicated
// struct keeps cursor construction correct for every sortable column without
// per-column GORM tags leaking onto the Company model, and keeps the
// scan_settings/companies column sets from colliding (both tables have
// created_at/updated_at).
type adminCompanyJoinRow struct {
	companymodel.Company
	SortVal  sql.NullString `gorm:"column:sort_val"`
	SortNull bool           `gorm:"column:sort_null"`

	SSCompanyID            sql.NullString `gorm:"column:ss_company_id"`
	SSAdapterType          sql.NullString `gorm:"column:ss_adapter_type"`
	SSScanIntervalOverride sql.NullInt64  `gorm:"column:ss_scan_interval_override"`
	SSActive               sql.NullBool   `gorm:"column:ss_active"`
	SSAutoPaused           sql.NullBool   `gorm:"column:ss_auto_paused"`
	SSLastScanAt           sql.NullTime   `gorm:"column:ss_last_scan_at"`
	SSLastSuccessAt        sql.NullTime   `gorm:"column:ss_last_success_at"`
	SSLastErrorAt          sql.NullTime   `gorm:"column:ss_last_error_at"`
	SSLastErrorMessage     sql.NullString `gorm:"column:ss_last_error_message"`
	SSConsecutiveErrors    sql.NullInt64  `gorm:"column:ss_consecutive_errors"`
	SSCreatedAt            sql.NullTime   `gorm:"column:ss_created_at"`
	SSUpdatedAt            sql.NullTime   `gorm:"column:ss_updated_at"`

	QueueStatus sql.NullString `gorm:"column:queue_status"`
}

// scanSettingsListView projects the row's ss_* columns into the list-safe DTO
// view, or nil when the company has no scan_settings row (ss_company_id
// NULL — the LEFT JOIN produced no match).
func (row adminCompanyJoinRow) scanSettingsListView() *companydto.CompanyScanSettingsListView {
	if !row.SSCompanyID.Valid {
		return nil
	}
	view := &companydto.CompanyScanSettingsListView{
		CompanyID:         row.SSCompanyID.String,
		AdapterType:       row.SSAdapterType.String,
		Active:            row.SSActive.Bool,
		AutoPaused:        row.SSAutoPaused.Bool,
		ConsecutiveErrors: int(row.SSConsecutiveErrors.Int64),
		CreatedAt:         row.SSCreatedAt.Time,
		UpdatedAt:         row.SSUpdatedAt.Time,
	}
	if row.SSScanIntervalOverride.Valid {
		v := int(row.SSScanIntervalOverride.Int64)
		view.ScanIntervalOverride = &v
	}
	if row.SSLastScanAt.Valid {
		v := row.SSLastScanAt.Time
		view.LastScanAt = &v
	}
	if row.SSLastSuccessAt.Valid {
		v := row.SSLastSuccessAt.Time
		view.LastSuccessAt = &v
	}
	if row.SSLastErrorAt.Valid {
		v := row.SSLastErrorAt.Time
		view.LastErrorAt = &v
	}
	if row.SSLastErrorMessage.Valid {
		v := row.SSLastErrorMessage.String
		view.LastErrorMessage = &v
	}
	return view
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
