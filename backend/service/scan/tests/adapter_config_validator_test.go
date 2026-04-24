package scan

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// noopAdapterConfigService is a terminal AdapterConfigService that always succeeds.
type noopAdapterConfigService struct {
	listResult   []dto.AdapterConfigDTO
	updateResult dto.AdapterConfigDTO
	configResult dto.EffectiveScanConfigDTO
}

func (n *noopAdapterConfigService) ListAdapterConfigs(_ context.Context) ([]dto.AdapterConfigDTO, error) {
	return n.listResult, nil
}

func (n *noopAdapterConfigService) UpdateAdapterConfig(_ context.Context, _ string, _ dto.UpdateAdapterConfigInput) (dto.AdapterConfigDTO, error) {
	return n.updateResult, nil
}

func (n *noopAdapterConfigService) GetEffectiveScanConfig(_ context.Context, _ string) (dto.EffectiveScanConfigDTO, error) {
	return n.configResult, nil
}

func (n *noopAdapterConfigService) UpdateScanIntervalOverride(_ context.Context, _ string, _ *int) error {
	return nil
}

var _ svc.AdapterConfigService = (*noopAdapterConfigService)(nil)

func ptr(i int) *int { return &i }

func TestAdapterConfigValidator_UpdateAdapterConfig_NoFields(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	_, err := v.UpdateAdapterConfig(context.Background(), "greenhouse", dto.UpdateAdapterConfigInput{})
	if err == nil {
		t.Fatal("expected error when no fields provided")
	}
	var vErrs apperr.ValidationErrors
	if !errors.As(err, &vErrs) {
		t.Fatalf("expected ValidationErrors, got %T: %v", err, err)
	}
}

func TestAdapterConfigValidator_UpdateAdapterConfig_EmptyAdapterType(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	_, err := v.UpdateAdapterConfig(context.Background(), "", dto.UpdateAdapterConfigInput{
		ScanIntervalMinutes: ptr(60),
	})
	if err == nil {
		t.Fatal("expected error for empty adapter_type")
	}
	var vErrs apperr.ValidationErrors
	if !errors.As(err, &vErrs) {
		t.Fatalf("expected ValidationErrors, got %T: %v", err, err)
	}
}

func TestAdapterConfigValidator_UpdateAdapterConfig_ScanIntervalBelowMin(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	_, err := v.UpdateAdapterConfig(context.Background(), "greenhouse", dto.UpdateAdapterConfigInput{
		ScanIntervalMinutes: ptr(0),
	})
	if err == nil {
		t.Fatal("expected error for scan_interval_minutes = 0")
	}
}

func TestAdapterConfigValidator_UpdateAdapterConfig_ScanIntervalAboveMax(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	_, err := v.UpdateAdapterConfig(context.Background(), "greenhouse", dto.UpdateAdapterConfigInput{
		ScanIntervalMinutes: ptr(10081),
	})
	if err == nil {
		t.Fatal("expected error for scan_interval_minutes > 10080")
	}
}

func TestAdapterConfigValidator_UpdateAdapterConfig_InterCompanyDelayBelowMin(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	_, err := v.UpdateAdapterConfig(context.Background(), "greenhouse", dto.UpdateAdapterConfigInput{
		InterCompanyDelayS: ptr(-1),
	})
	if err == nil {
		t.Fatal("expected error for inter_company_delay_s < 0")
	}
}

func TestAdapterConfigValidator_UpdateAdapterConfig_InterCompanyDelayAboveMax(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	_, err := v.UpdateAdapterConfig(context.Background(), "greenhouse", dto.UpdateAdapterConfigInput{
		InterCompanyDelayS: ptr(3601),
	})
	if err == nil {
		t.Fatal("expected error for inter_company_delay_s > 3600")
	}
}

func TestAdapterConfigValidator_UpdateAdapterConfig_ValidMinBoundary(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	_, err := v.UpdateAdapterConfig(context.Background(), "greenhouse", dto.UpdateAdapterConfigInput{
		ScanIntervalMinutes: ptr(1),
	})
	if err != nil {
		t.Errorf("unexpected error for valid min scan_interval_minutes=1: %v", err)
	}
}

func TestAdapterConfigValidator_UpdateAdapterConfig_ValidMaxBoundary(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	_, err := v.UpdateAdapterConfig(context.Background(), "greenhouse", dto.UpdateAdapterConfigInput{
		ScanIntervalMinutes: ptr(10080),
	})
	if err != nil {
		t.Errorf("unexpected error for valid max scan_interval_minutes=10080: %v", err)
	}
}

func TestAdapterConfigValidator_UpdateAdapterConfig_ValidDelay(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	_, err := v.UpdateAdapterConfig(context.Background(), "greenhouse", dto.UpdateAdapterConfigInput{
		InterCompanyDelayS: ptr(0),
	})
	if err != nil {
		t.Errorf("unexpected error for valid inter_company_delay_s=0: %v", err)
	}
}

func TestAdapterConfigValidator_UpdateScanIntervalOverride_EmptyCompanyID(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	err := v.UpdateScanIntervalOverride(context.Background(), "", ptr(60))
	if err == nil {
		t.Fatal("expected error for empty company_id")
	}
}

func TestAdapterConfigValidator_UpdateScanIntervalOverride_NilOverrideClearsIt(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	err := v.UpdateScanIntervalOverride(context.Background(), "company-1", nil)
	if err != nil {
		t.Errorf("expected nil error for nil override (clear): %v", err)
	}
}

func TestAdapterConfigValidator_UpdateScanIntervalOverride_ValidOverride(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	err := v.UpdateScanIntervalOverride(context.Background(), "company-1", ptr(60))
	if err != nil {
		t.Errorf("expected no error for valid override=60: %v", err)
	}
}

func TestAdapterConfigValidator_UpdateScanIntervalOverride_ZeroOverrideInvalid(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	err := v.UpdateScanIntervalOverride(context.Background(), "company-1", ptr(0))
	if err == nil {
		t.Fatal("expected error for override=0 (below minimum of 1)")
	}
}

func TestAdapterConfigValidator_UpdateScanIntervalOverride_AboveMaxInvalid(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	err := v.UpdateScanIntervalOverride(context.Background(), "company-1", ptr(10081))
	if err == nil {
		t.Fatal("expected error for override=10081 (above maximum of 10080)")
	}
}

func TestAdapterConfigValidator_UpdateScanIntervalOverride_MaxBoundaryValid(t *testing.T) {
	v := svc.NewAdapterConfigValidator(&noopAdapterConfigService{})

	err := v.UpdateScanIntervalOverride(context.Background(), "company-1", ptr(10080))
	if err != nil {
		t.Errorf("expected no error for override=10080: %v", err)
	}
}
