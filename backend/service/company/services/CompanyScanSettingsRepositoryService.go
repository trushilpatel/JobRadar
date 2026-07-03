package services

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/tracing"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	"gorm.io/gorm"
)

// scanSettingsRepository is the GORM-backed terminal implementation.
type scanSettingsRepository struct {
	db  *gorm.DB
	log logger.Logger
}

// NewScanSettingsRepositoryService constructs the ScanSettingsRepository.
// A nil log silences all structured logging.
func NewScanSettingsRepositoryService(db *gorm.DB) ScanSettingsRepository {
	return &scanSettingsRepository{db: db, log: logger.NewNoop()}
}

// errNoCompany is the canonical guard error for empty companyID. Returned
// from every method so tests have a stable check.
func errNoCompany() error {
	return apperr.ErrInternal.WithMessage("company id required")
}

// Get returns the scan_settings row for companyID, or NOT_FOUND when no
// row exists yet (post-cutover companies start without settings).
func (r *scanSettingsRepository) Get(ctx context.Context, companyID string) (_ *companymodel.CompanyScanSettings, err error) {
	ctx, span := tracing.Start(ctx, "company.scan_settings.repo.Get")
	defer tracing.End(span, &err)

	if companyID == "" {
		return nil, errNoCompany()
	}

	var row companymodel.CompanyScanSettings
	if err = r.db.WithContext(ctx).First(&row, "company_id = ?", companyID).Error; err != nil {
		if apperr.IsNotFound(err) {
			return nil, apperr.ErrNotFound.WithMessage("scan settings not found")
		}
		r.log.Warn("failed to fetch scan settings", logger.Str(logger.FieldCompany, companyID), logger.Err(err))
		return nil, apperr.ErrInternal.WithMessage("failed to fetch scan settings")
	}
	return &row, nil
}

// GetMissing returns the IDs of every `companies` row that does NOT have
// a matching `company_scan_settings` row. Admin UIs call this to surface
// un-onboarded companies. Returns an empty slice (not nil) when none.
func (r *scanSettingsRepository) GetMissing(ctx context.Context) (_ []string, err error) {
	ctx, span := tracing.Start(ctx, "company.scan_settings.repo.GetMissing")
	defer tracing.End(span, &err)

	var ids []string
	if err = r.db.WithContext(ctx).Raw(`
		SELECT c.id
		FROM companies c
		LEFT JOIN company_scan_settings s ON s.company_id = c.id
		WHERE s.company_id IS NULL
		ORDER BY c.id
	`).Scan(&ids).Error; err != nil {
		return nil, apperr.ErrInternal.WithMessage("failed to fetch companies missing scan settings")
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}

// Create inserts a new scan_settings row. Returns CONFLICT when a row
// already exists for the company.
func (r *scanSettingsRepository) Create(ctx context.Context, s companymodel.CompanyScanSettings) (err error) {
	if s.CompanyID == "" {
		return errNoCompany()
	}
	ctx, span := tracing.Start(ctx, "company.scan_settings.repo.Create")
	defer tracing.End(span, &err)

	if err = r.db.WithContext(ctx).Create(&s).Error; err != nil {
		if isUniqueViolation(err) {
			return apperr.ErrConflict.WithMessage("scan settings already exist for company")
		}
		r.log.Warn("failed to create scan settings",
			logger.Str(logger.FieldCompany, s.CompanyID),
			logger.Str(logger.FieldAdapterType, s.AdapterType),
			logger.Err(err))
		return apperr.ErrInternal.WithMessage("failed to create scan settings")
	}
	return nil
}

// Update applies a partial-patch to an existing scan_settings row. Returns
// NOT_FOUND when no row exists yet — admins must Create first.
func (r *scanSettingsRepository) Update(ctx context.Context, companyID string, p companydto.CompanyScanSettingsInput) (err error) {
	if companyID == "" {
		return errNoCompany()
	}
	if p.IsEmpty() {
		return apperr.ErrBadRequest.WithMessage("at least one field must be provided")
	}
	ctx, span := tracing.Start(ctx, "company.scan_settings.repo.Update")
	defer tracing.End(span, &err)

	fields := scanSettingsInputToFields(p)
	res := r.db.WithContext(ctx).
		Model(&companymodel.CompanyScanSettings{}).
		Where("company_id = ?", companyID).
		Updates(fields)
	if res.Error != nil {
		r.log.Warn("failed to update scan settings",
			logger.Str(logger.FieldCompany, companyID),
			logger.Str(logger.FieldAdapterType, adapterTypeFromInput(p)),
			logger.Err(res.Error))
		return apperr.ErrInternal.WithMessage("failed to update scan settings")
	}
	if res.RowsAffected == 0 {
		return apperr.ErrNotFound.WithMessage("scan settings not found")
	}
	return nil
}

// adapterTypeFromInput returns the patched adapter_type, or empty when the
// patch does not touch it. Used to enrich scan-settings update error logs.
func adapterTypeFromInput(p companydto.CompanyScanSettingsInput) string {
	if p.AdapterType != nil {
		return *p.AdapterType
	}
	return ""
}

// UpdateScanState is the worker-only write path. It applies a partial
// state patch (last_scan_at / last_success_at / last_error_*) to a
// scan_settings row. Returns NOT_FOUND when the row is missing — workers
// should already have skipped such companies, so this is a defensive
// signal.
func (r *scanSettingsRepository) UpdateScanState(ctx context.Context, companyID string, st companydto.CompanyScanStateUpdate) (err error) {
	if companyID == "" {
		return errNoCompany()
	}
	ctx, span := tracing.Start(ctx, "company.scan_settings.repo.UpdateScanState")
	defer tracing.End(span, &err)

	fields := make(map[string]any)
	if st.LastScanAt != nil {
		fields["last_scan_at"] = *st.LastScanAt
	}
	if st.LastSuccessAt != nil {
		fields["last_success_at"] = *st.LastSuccessAt
	}
	if st.LastErrorAt != nil {
		fields["last_error_at"] = *st.LastErrorAt
	}
	if st.LastErrorMessage != nil {
		fields["last_error_message"] = *st.LastErrorMessage
	}
	if st.ConsecutiveErrors != nil {
		fields["consecutive_errors"] = *st.ConsecutiveErrors
	}
	if len(fields) == 0 {
		return nil
	}

	res := r.db.WithContext(ctx).
		Model(&companymodel.CompanyScanSettings{}).
		Where("company_id = ?", companyID).
		Updates(fields)
	if res.Error != nil {
		return apperr.ErrInternal.WithMessage("failed to update scan state")
	}
	if res.RowsAffected == 0 {
		return apperr.ErrNotFound.WithMessage("scan settings not found")
	}
	return nil
}

// Delete removes the scan_settings row. Idempotent: missing row is not an
// error since CASCADE may have already removed it via a company delete.
func (r *scanSettingsRepository) Delete(ctx context.Context, companyID string) (err error) {
	if companyID == "" {
		return errNoCompany()
	}
	ctx, span := tracing.Start(ctx, "company.scan_settings.repo.Delete")
	defer tracing.End(span, &err)

	if err = r.db.WithContext(ctx).
		Delete(&companymodel.CompanyScanSettings{}, "company_id = ?", companyID).
		Error; err != nil {
		return apperr.ErrInternal.WithMessage("failed to delete scan settings")
	}
	return nil
}

// scanSettingsInputToFields converts a partial-patch input into the GORM
// field map used by Update. Shared by the admin chain and the repo so the
// column projection lives in one place.
func scanSettingsInputToFields(p companydto.CompanyScanSettingsInput) map[string]any {
	fields := make(map[string]any)
	if p.AdapterType != nil {
		fields["adapter_type"] = *p.AdapterType
	}
	if p.AdapterConfig != nil {
		fields["adapter_config"] = *p.AdapterConfig
	}
	if p.ScanIntervalOverride != nil {
		fields["scan_interval_override"] = *p.ScanIntervalOverride
	}
	if p.Active != nil {
		fields["active"] = *p.Active
	}
	if p.AutoPaused != nil {
		fields["auto_paused"] = *p.AutoPaused
		// AC1.9: clearing auto_paused (admin unpause) also resets
		// consecutive_errors so the next scheduler sweep does not immediately
		// re-pause the company on its historical error count. Re-pausing is only
		// deliberate, never intended.
		if !*p.AutoPaused {
			fields["consecutive_errors"] = 0
		}
	}
	return fields
}

// isUniqueViolation detects PostgreSQL unique-constraint failures across
// the GORM driver landscape. Errors carry a SQLSTATE 23505 — pgx wraps
// the code on a *pgconn.PgError; gorm's generic wrapper hides it behind
// ErrDuplicatedKey on recent versions. We check both for portability
// across local dev (pgx direct) and CI.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	// Fallback for pgconn errors that GORM doesn't wrap.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// isForeignKeyViolation detects PostgreSQL foreign-key-constraint failures
// (SQLSTATE 23503). The company Create path uses this to surface an
// unregistered adapter_type (FK → adapter_configs) as a 400 rather than a
// generic 500. GORM exposes ErrForeignKeyViolated on recent versions; we also
// inspect the raw pgconn code for portability across local dev and CI.
func isForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503"
	}
	return false
}
