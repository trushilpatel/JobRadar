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
// Defined here alongside the implementation it abstracts.
type AdapterConfigStore interface {
	ListConfigs(ctx context.Context) ([]scanmodel.AdapterConfig, error)
	GetConfig(ctx context.Context, adapterType string) (scanmodel.AdapterConfig, error)
	UpdateConfig(ctx context.Context, adapterType string, fields map[string]any) (scanmodel.AdapterConfig, error)
	GetCompanyWithAdapter(ctx context.Context, companyID string) (companymodel.Company, error)
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

func (s *adapterConfigStore) GetCompanyWithAdapter(ctx context.Context, companyID string) (_ companymodel.Company, err error) {
	ctx, span := tracing.Start(ctx, "adapterConfig.store.GetCompanyWithAdapter")
	defer tracing.End(span, &err)
	var company companymodel.Company
	err = s.db.WithContext(ctx).First(&company, "id = ?", companyID).Error
	return company, err
}

func (s *adapterConfigStore) UpdateCompanyScanIntervalOverride(ctx context.Context, companyID string, override *int) (err error) {
	ctx, span := tracing.Start(ctx, "adapterConfig.store.UpdateCompanyScanIntervalOverride")
	defer tracing.End(span, &err)
	result := s.db.WithContext(ctx).Model(&companymodel.Company{}).
		Where("id = ?", companyID).
		Update("scan_interval_minutes_override", override)
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
	company, err := r.store.GetCompanyWithAdapter(ctx, companyID)
	if err != nil {
		if errors.IsNotFound(err) {
			return dto.EffectiveScanConfigDTO{}, errors.ErrNotFound.WithMessage("company not found")
		}
		return dto.EffectiveScanConfigDTO{}, errors.ErrInternal.WithMessage("failed to fetch company")
	}
	adapterCfg, err := r.store.GetConfig(ctx, company.AdapterType)
	if err != nil {
		if errors.IsNotFound(err) {
			return dto.EffectiveScanConfigDTO{}, errors.ErrNotFound.WithMessage("adapter config not found")
		}
		return dto.EffectiveScanConfigDTO{}, errors.ErrInternal.WithMessage("failed to fetch adapter config")
	}
	effective := adapterCfg.ScanIntervalMinutes
	if company.ScanIntervalMinutesOverride != nil {
		effective = *company.ScanIntervalMinutesOverride
	}
	return dto.EffectiveScanConfigDTO{
		CompanyID:                    companyID,
		AdapterType:                  company.AdapterType,
		AdapterScanIntervalMinutes:   adapterCfg.ScanIntervalMinutes,
		EffectiveScanIntervalMinutes: effective,
		ScanIntervalMinutesOverride:  company.ScanIntervalMinutesOverride,
		InterCompanyDelayS:           adapterCfg.InterCompanyDelayS,
	}, nil
}

func (r *adapterConfigRepositoryService) UpdateScanIntervalOverride(ctx context.Context, companyID string, override *int) (err error) {
	ctx, span := tracing.Start(ctx, "adapterConfig.repository.UpdateScanIntervalOverride")
	defer tracing.End(span, &err)
	if err := r.store.UpdateCompanyScanIntervalOverride(ctx, companyID, override); err != nil {
		if errors.IsNotFound(err) {
			return errors.ErrNotFound.WithMessage("company not found")
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
