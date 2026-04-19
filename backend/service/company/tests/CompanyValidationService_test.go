package company_test

import (
	"testing"

	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

func boolPtr(b bool) *bool    { return &b }
func strPtr(s string) *string { return &s }

func TestValidateCreate(t *testing.T) {
	v := companysvc.NewValidator()

	tests := []struct {
		name    string
		input   companydto.CreateCompanyInput
		wantErr bool
	}{
		{
			name:  "valid greenhouse company",
			input: companydto.CreateCompanyInput{ID: "stripe", Name: "Stripe", URL: "https://stripe.com/jobs", AdapterType: companycommon.AdapterGreenhouse},
		},
		{
			name:  "valid fixed-URL adapter without URL",
			input: companydto.CreateCompanyInput{ID: "google", Name: "Google", AdapterType: companycommon.AdapterGoogle},
		},
		{
			name:    "missing id",
			input:   companydto.CreateCompanyInput{Name: "Stripe", URL: "https://example.com", AdapterType: companycommon.AdapterGreenhouse},
			wantErr: true,
		},
		{
			name:    "missing name",
			input:   companydto.CreateCompanyInput{ID: "stripe", URL: "https://example.com", AdapterType: companycommon.AdapterGreenhouse},
			wantErr: true,
		},
		{
			name:    "missing url for non-fixed adapter",
			input:   companydto.CreateCompanyInput{ID: "stripe", Name: "Stripe", AdapterType: companycommon.AdapterGreenhouse},
			wantErr: true,
		},
		{
			name:    "invalid adapter_type",
			input:   companydto.CreateCompanyInput{ID: "stripe", Name: "Stripe", URL: "https://example.com", AdapterType: "invalid"},
			wantErr: true,
		},
		{
			name:  "netflix fixed adapter no URL",
			input: companydto.CreateCompanyInput{ID: "netflix", Name: "Netflix", AdapterType: companycommon.AdapterNetflix},
		},
		{
			name:  "rippling fixed adapter no URL",
			input: companydto.CreateCompanyInput{ID: "rippling", Name: "Rippling", AdapterType: companycommon.AdapterRippling},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := v.ValidateCreate(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateUpdate(t *testing.T) {
	v := companysvc.NewValidator()

	tests := []struct {
		name    string
		input   companydto.UpdateCompanyInput
		wantErr bool
	}{
		{
			name:  "valid name update",
			input: companydto.UpdateCompanyInput{Name: strPtr("New Name")},
		},
		{
			name:  "valid active update",
			input: companydto.UpdateCompanyInput{Active: boolPtr(false)},
		},
		{
			name:  "valid adapter_type update",
			input: companydto.UpdateCompanyInput{AdapterType: strPtr(companycommon.AdapterLever)},
		},
		{
			name:    "no fields provided",
			input:   companydto.UpdateCompanyInput{},
			wantErr: true,
		},
		{
			name:    "invalid adapter_type",
			input:   companydto.UpdateCompanyInput{AdapterType: strPtr("bad_adapter")},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := v.ValidateUpdate(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateUpdate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateList(t *testing.T) {
	v := companysvc.NewValidator()

	tests := []struct {
		name    string
		filter  companydto.CompanyFilter
		page    int
		limit   int
		wantErr bool
	}{
		{name: "empty filter", filter: companydto.CompanyFilter{}, page: 1, limit: 20},
		{name: "valid adapter_type", filter: companydto.CompanyFilter{AdapterType: companycommon.AdapterGreenhouse}, page: 1, limit: 20},
		{name: "valid active filter", filter: companydto.CompanyFilter{Active: boolPtr(true)}, page: 1, limit: 20},
		{name: "invalid adapter_type", filter: companydto.CompanyFilter{AdapterType: "bogus"}, page: 1, limit: 20, wantErr: true},
		{name: "page zero", filter: companydto.CompanyFilter{}, page: 0, limit: 20, wantErr: true},
		{name: "limit too large", filter: companydto.CompanyFilter{}, page: 1, limit: 200, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := v.ValidateList(tc.filter, tc.page, tc.limit)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateList() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateSetAllActive(t *testing.T) {
	v := companysvc.NewValidator()

	if err := v.ValidateSetAllActive(nil); err == nil {
		t.Error("expected error for nil active, got nil")
	}
	if err := v.ValidateSetAllActive(boolPtr(true)); err != nil {
		t.Errorf("unexpected error for non-nil active: %v", err)
	}
	if err := v.ValidateSetAllActive(boolPtr(false)); err != nil {
		t.Errorf("unexpected error for false active: %v", err)
	}
}
