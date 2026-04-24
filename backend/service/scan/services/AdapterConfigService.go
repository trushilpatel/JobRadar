package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// AdapterConfigService defines all business operations for the adapter_configs domain.
type AdapterConfigService interface {
	ListAdapterConfigs(ctx context.Context) ([]dto.AdapterConfigDTO, error)
	UpdateAdapterConfig(ctx context.Context, adapterType string, input dto.UpdateAdapterConfigInput) (dto.AdapterConfigDTO, error)
	GetEffectiveScanConfig(ctx context.Context, companyID string) (dto.EffectiveScanConfigDTO, error)
	UpdateScanIntervalOverride(ctx context.Context, companyID string, override *int) error
}

// BaseAdapterConfigService auto-delegates all AdapterConfigService methods to next.
// Embed this in decorators and override only the methods they own.
type BaseAdapterConfigService struct {
	next AdapterConfigService
}

// NewBaseAdapterConfigService constructs a BaseAdapterConfigService wrapping next.
// Panics if next is nil to surface wiring mistakes early.
func NewBaseAdapterConfigService(next AdapterConfigService) BaseAdapterConfigService {
	if next == nil {
		panic("BaseAdapterConfigService: next must not be nil — check wire setup")
	}
	return BaseAdapterConfigService{next: next}
}

func (b *BaseAdapterConfigService) ListAdapterConfigs(ctx context.Context) ([]dto.AdapterConfigDTO, error) {
	return b.next.ListAdapterConfigs(ctx)
}

func (b *BaseAdapterConfigService) UpdateAdapterConfig(ctx context.Context, adapterType string, input dto.UpdateAdapterConfigInput) (dto.AdapterConfigDTO, error) {
	return b.next.UpdateAdapterConfig(ctx, adapterType, input)
}

func (b *BaseAdapterConfigService) GetEffectiveScanConfig(ctx context.Context, companyID string) (dto.EffectiveScanConfigDTO, error) {
	return b.next.GetEffectiveScanConfig(ctx, companyID)
}

func (b *BaseAdapterConfigService) UpdateScanIntervalOverride(ctx context.Context, companyID string, override *int) error {
	return b.next.UpdateScanIntervalOverride(ctx, companyID, override)
}
