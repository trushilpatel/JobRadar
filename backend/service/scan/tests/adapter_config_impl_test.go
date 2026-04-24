package scan

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// errAdapterConfigService always returns an error.
type errAdapterConfigService struct {
	err error
}

func (e *errAdapterConfigService) ListAdapterConfigs(_ context.Context) ([]dto.AdapterConfigDTO, error) {
	return nil, e.err
}

func (e *errAdapterConfigService) UpdateAdapterConfig(_ context.Context, _ string, _ dto.UpdateAdapterConfigInput) (dto.AdapterConfigDTO, error) {
	return dto.AdapterConfigDTO{}, e.err
}

func (e *errAdapterConfigService) GetEffectiveScanConfig(_ context.Context, _ string) (dto.EffectiveScanConfigDTO, error) {
	return dto.EffectiveScanConfigDTO{}, e.err
}

func (e *errAdapterConfigService) UpdateScanIntervalOverride(_ context.Context, _ string, _ *int) error {
	return e.err
}

var _ svc.AdapterConfigService = (*errAdapterConfigService)(nil)

func TestAdapterConfigImpl_ListAdapterConfigs_Delegates(t *testing.T) {
	expected := []dto.AdapterConfigDTO{
		{AdapterType: "greenhouse", ScanIntervalMinutes: 1440, InterCompanyDelayS: 3},
	}
	noop := &noopAdapterConfigService{listResult: expected}
	impl := svc.NewAdapterConfigImplService(noop)

	result, err := impl.ListAdapterConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListAdapterConfigs() error = %v", err)
	}
	if len(result) != 1 || result[0].AdapterType != "greenhouse" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestAdapterConfigImpl_UpdateAdapterConfig_NotFoundPropagates(t *testing.T) {
	notFoundSvc := &errAdapterConfigService{
		err: apperr.ErrNotFound.WithMessage("adapter config not found"),
	}
	impl := svc.NewAdapterConfigImplService(notFoundSvc)

	_, err := impl.UpdateAdapterConfig(context.Background(), "unknown", dto.UpdateAdapterConfigInput{
		ScanIntervalMinutes: ptr(60),
	})
	if err == nil {
		t.Fatal("expected error for not found")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND error, got %v", err)
	}
}

func TestAdapterConfigImpl_GetEffectiveScanConfig_Delegates(t *testing.T) {
	override := 60
	expected := dto.EffectiveScanConfigDTO{
		CompanyID:                    "company-1",
		AdapterType:                  "greenhouse",
		AdapterScanIntervalMinutes:   1440,
		EffectiveScanIntervalMinutes: 60,
		ScanIntervalMinutesOverride:  &override,
		InterCompanyDelayS:           3,
	}
	noop := &noopAdapterConfigService{configResult: expected}
	impl := svc.NewAdapterConfigImplService(noop)

	result, err := impl.GetEffectiveScanConfig(context.Background(), "company-1")
	if err != nil {
		t.Fatalf("GetEffectiveScanConfig() error = %v", err)
	}
	if result.EffectiveScanIntervalMinutes != 60 {
		t.Errorf("expected effective=60, got %d", result.EffectiveScanIntervalMinutes)
	}
}

func TestAdapterConfigImpl_UpdateScanIntervalOverride_NotFoundPropagates(t *testing.T) {
	notFoundSvc := &errAdapterConfigService{
		err: apperr.ErrNotFound.WithMessage("company not found"),
	}
	impl := svc.NewAdapterConfigImplService(notFoundSvc)

	err := impl.UpdateScanIntervalOverride(context.Background(), "missing", nil)
	if err == nil {
		t.Fatal("expected error for not found")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND error, got %v", err)
	}
}
