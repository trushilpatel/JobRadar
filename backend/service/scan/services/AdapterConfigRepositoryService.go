package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	scanmodel "github.com/trushilpatel/jobradar/service/scan/models"
	"gorm.io/gorm"
)

// AdapterConfigStore is the interface for adapter_configs DB operations.
//
// Phase 2: the per-company adapter selection and per-company scan-interval
// override moved off `companies` onto `company_scan_settings`. The two
// company-scoped methods below now read and write that table — the global
// `adapter_configs` table itself is unchanged.
type AdapterConfigStore interface {
	ListConfigs(ctx context.Context) ([]scanmodel.AdapterConfig, error)
	GetConfig(ctx context.Context, adapterType string) (scanmodel.AdapterConfig, error)
	UpdateConfig(ctx context.Context, adapterType string, fields map[string]any) (scanmodel.AdapterConfig, error)
	// GetCompanyScanSettings returns the scan_settings row for companyID.
	// Used by GetEffectiveScanConfig to compute the effective scan interval
	// from the company's adapter_type + override + global adapter_configs row.
	GetCompanyScanSettings(ctx context.Context, companyID string) (companymodel.CompanyScanSettings, error)
	// UpdateCompanyScanIntervalOverride patches the per-company interval
	// override on `company_scan_settings`. Returns NOT_FOUND when the row
	// does not exist — admins must onboard the company first.
	UpdateCompanyScanIntervalOverride(ctx context.Context, companyID string, override *int) error
}

type adapterConfigStore struct {
	db *gorm.DB
}

// NewAdapterConfigStore constructs an AdapterConfigStore backed by GORM.
func NewAdapterConfigStore(db *gorm.DB) AdapterConfigStore {
	return &adapterConfigStore{db: db}
}

func (s *adapterConfigStore) ListConfigs(ctx context.Context) (_ []scanmodel.AdapterConfig, err error) {
	ctx, span := tracing.Start(ctx, "adapterConfig.store.ListConfigs")
	defer tracing.End(span, &err)
	var configs []scanmodel.AdapterConfig
	err = s.db.WithContext(ctx).Order("adapter_type ASC").Find(&configs).Error
	return configs, err
}

func (s *adapterConfigStore) GetConfig(ctx context.Context, adapterType string) (_ scanmodel.AdapterConfig, err error) {
	ctx, span := tracing.Start(ctx, "adapterConfig.store.GetConfig")
	defer tracing.End(span, &err)
	var cfg scanmodel.AdapterConfig
	err = s.db.WithContext(ctx).First(&cfg, "adapter_type = ?", adapterType).Error
	return cfg, err
}

func (s *adapterConfigStore) UpdateConfig(ctx context.Context, adapterType string, fields map[string]any) (_ scanmodel.AdapterConfig, err error) {
	ctx, span := tracing.Start(ctx, "adapterConfig.store.UpdateConfig")
	defer tracing.End(span, &err)
	result := s.db.WithContext(ctx).Model(&scanmodel.AdapterConfig{}).
		Where("adapter_type = ?", adapterType).
		Updates(fields)
	if result.Error != nil {
		return scanmodel.AdapterConfig{}, result.Error
	}
	if result.RowsAffected == 0 {
		return scanmodel.AdapterConfig{}, gorm.ErrRecordNotFound
	}
	var cfg scanmodel.AdapterConfig
	err = s.db.WithContext(ctx).First(&cfg, "adapter_type = ?", adapterType).Error
	return cfg, err
}

func (s *adapterConfigStore) GetCompanyScanSettings(ctx context.Context, companyID string) (_ companymodel.CompanyScanSettings, err error) {
	ctx, span := tracing.Start(ctx, "adapterConfig.store.GetCompanyScanSettings")
	defer tracing.End(span, &err)
	var settings companymodel.CompanyScanSettings
	err = s.db.WithContext(ctx).First(&settings, "company_id = ?", companyID).Error
	return settings, err
}

func (s *adapterConfigStore) UpdateCompanyScanIntervalOverride(ctx context.Context, companyID string, override *int) (err error) {
	ctx, span := tracing.Start(ctx, "adapterConfig.store.UpdateCompanyScanIntervalOverride")
	defer tracing.End(span, &err)
	result := s.db.WithContext(ctx).Model(&companymodel.CompanyScanSettings{}).
		Where("company_id = ?", companyID).
		Update("scan_interval_override", override)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// adapterConfigRepositoryService is the terminal AdapterConfigService implementation.
// It has no next and does not embed BaseAdapterConfigService.
type adapterConfigRepositoryService struct {
	store AdapterConfigStore
}

// NewAdapterConfigRepositoryService constructs the terminal AdapterConfigService backed by a store.
func NewAdapterConfigRepositoryService(store AdapterConfigStore) AdapterConfigService {
	return &adapterConfigRepositoryService{store: store}
}

func (r *adapterConfigRepositoryService) ListAdapterConfigs(ctx context.Context) (_ []dto.AdapterConfigDTO, err error) {
	ctx, span := tracing.Start(ctx, "adapterConfig.repository.ListAdapterConfigs")
	defer tracing.End(span, &err)
	configs, err := r.store.ListConfigs(ctx)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to list adapter configs")
	}
	result := make([]dto.AdapterConfigDTO, 0, len(configs))
	for _, c := range configs {
		result = append(result, toAdapterConfigDTO(c))
	}
	return result, nil
}

func (r *adapterConfigRepositoryService) UpdateAdapterConfig(ctx context.Context, adapterType string, input dto.UpdateAdapterConfigInput) (_ dto.AdapterConfigDTO, err error) {
	ctx, span := tracing.Start(ctx, "adapterConfig.repository.UpdateAdapterConfig")
	defer tracing.End(span, &err)
	fields := make(map[string]any)
	if input.ScanIntervalMinutes != nil {
		fields["scan_interval_minutes"] = *input.ScanIntervalMinutes
	}
	if input.InterCompanyDelayS != nil {
		fields["inter_company_delay_s"] = *input.InterCompanyDelayS
	}
	updated, err := r.store.UpdateConfig(ctx, adapterType, fields)
	if err != nil {
		if errors.IsNotFound(err) {
			return dto.AdapterConfigDTO{}, errors.ErrNotFound.WithMessage("adapter config not found")
		}
		return dto.AdapterConfigDTO{}, errors.ErrInternal.WithMessage("failed to update adapter config")
	}
	return toAdapterConfigDTO(updated), nil
}

func (r *adapterConfigRepositoryService) GetEffectiveScanConfig(ctx context.Context, companyID string) (_ dto.EffectiveScanConfigDTO, err error) {
	ctx, span := tracing.Start(ctx, "adapterConfig.repository.GetEffectiveScanConfig")
	defer tracing.End(span, &err)
	settings, err := r.store.GetCompanyScanSettings(ctx, companyID)
	if err != nil {
		if errors.IsNotFound(err) {
			// Phase 2: a company without scan_settings is un-onboarded —
			// surface the same NOT_FOUND so the API contract is unchanged.
			return dto.EffectiveScanConfigDTO{}, errors.ErrNotFound.WithMessage("company has no scan settings configured")
		}
		return dto.EffectiveScanConfigDTO{}, errors.ErrInternal.WithMessage("failed to fetch company scan settings")
	}
	adapterCfg, err := r.store.GetConfig(ctx, settings.AdapterType)
	if err != nil {
		if errors.IsNotFound(err) {
			return dto.EffectiveScanConfigDTO{}, errors.ErrNotFound.WithMessage("adapter config not found")
		}
		return dto.EffectiveScanConfigDTO{}, errors.ErrInternal.WithMessage("failed to fetch adapter config")
	}
	effective := adapterCfg.ScanIntervalMinutes
	if settings.ScanIntervalOverride != nil {
		effective = *settings.ScanIntervalOverride
	}
	return dto.EffectiveScanConfigDTO{
		CompanyID:                    companyID,
		AdapterType:                  settings.AdapterType,
		AdapterScanIntervalMinutes:   adapterCfg.ScanIntervalMinutes,
		EffectiveScanIntervalMinutes: effective,
		ScanIntervalMinutesOverride:  settings.ScanIntervalOverride,
		InterCompanyDelayS:           adapterCfg.InterCompanyDelayS,
	}, nil
}

func (r *adapterConfigRepositoryService) UpdateScanIntervalOverride(ctx context.Context, companyID string, override *int) (err error) {
	ctx, span := tracing.Start(ctx, "adapterConfig.repository.UpdateScanIntervalOverride")
	defer tracing.End(span, &err)
	if err := r.store.UpdateCompanyScanIntervalOverride(ctx, companyID, override); err != nil {
		if errors.IsNotFound(err) {
			return errors.ErrNotFound.WithMessage("company has no scan settings configured")
		}
		return errors.ErrInternal.WithMessage("failed to update scan interval override")
	}
	return nil
}

func toAdapterConfigDTO(c scanmodel.AdapterConfig) dto.AdapterConfigDTO {
	return dto.AdapterConfigDTO{
		AdapterType:         c.AdapterType,
		ScanIntervalMinutes: c.ScanIntervalMinutes,
		InterCompanyDelayS:  c.InterCompanyDelayS,
		LastScheduledAt:     c.LastScheduledAt,
		CreatedAt:           c.CreatedAt,
		UpdatedAt:           c.UpdatedAt,
	}
}
