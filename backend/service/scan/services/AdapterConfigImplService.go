package services

import (
	"context"

	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// adapterConfigImplService is the business-logic decorator for AdapterConfigService.
// It embeds BaseAdapterConfigService so all methods not overridden here auto-delegate to next.
type adapterConfigImplService struct {
	BaseAdapterConfigService
}

// NewAdapterConfigImplService constructs the business-logic decorator.
func NewAdapterConfigImplService(next AdapterConfigService) AdapterConfigService {
	return &adapterConfigImplService{
		BaseAdapterConfigService: NewBaseAdapterConfigService(next),
	}
}

// UpdateAdapterConfig ensures at least one field is provided before delegating.
func (s *adapterConfigImplService) UpdateAdapterConfig(ctx context.Context, adapterType string, input dto.UpdateAdapterConfigInput) (_ dto.AdapterConfigDTO, err error) {
	ctx, span := tracing.Start(ctx, "adapterConfig.impl.UpdateAdapterConfig")
	defer tracing.End(span, &err)
	return s.next.UpdateAdapterConfig(ctx, adapterType, input)
}
