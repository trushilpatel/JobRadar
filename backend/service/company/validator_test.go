package company

import (
	"testing"

	"github.com/trushilpatel/jobradar/model"
)

func boolPtr(b bool) *bool   { return &b }
func strPtr(s string) *string { return &s }

func TestValidateCreate(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		input   CreateCompanyInput
		wantErr bool
	}{
		{
			name:  "valid greenhouse company",
			input: CreateCompanyInput{ID: "stripe", Name: "Stripe", URL: "https://stripe.com/jobs", AdapterType: model.AdapterGreenhouse},
		},
		{
			name:  "valid fixed-URL adapter without URL",
			input: CreateCompanyInput{ID: "google", Name: "Google", AdapterType: model.AdapterGoogle},
		},
		{
			name:    "missing id",
			input:   CreateCompanyInput{Name: "Stripe", URL: "https://example.com", AdapterType: model.AdapterGreenhouse},
			wantErr: true,
		},
		{
			name:    "missing name",
			input:   CreateCompanyInput{ID: "stripe", URL: "https://example.com", AdapterType: model.AdapterGreenhouse},
			wantErr: true,
		},
		{
			name:    "missing url for non-fixed adapter",
			input:   CreateCompanyInput{ID: "stripe", Name: "Stripe", AdapterType: model.AdapterGreenhouse},
			wantErr: true,
		},
		{
			name:    "invalid adapter_type",
			input:   CreateCompanyInput{ID: "stripe", Name: "Stripe", URL: "https://example.com", AdapterType: "invalid"},
			wantErr: true,
		},
		{
			name:  "netflix fixed adapter no URL",
			input: CreateCompanyInput{ID: "netflix", Name: "Netflix", AdapterType: model.AdapterNetflix},
		},
		{
			name:  "rippling fixed adapter no URL",
			input: CreateCompanyInput{ID: "rippling", Name: "Rippling", AdapterType: model.AdapterRippling},
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
	v := NewValidator()

	tests := []struct {
		name    string
		input   UpdateCompanyInput
		wantErr bool
	}{
		{
			name:  "valid name update",
			input: UpdateCompanyInput{Name: strPtr("New Name")},
		},
		{
			name:  "valid active update",
			input: UpdateCompanyInput{Active: boolPtr(false)},
		},
		{
			name:  "valid adapter_type update",
			input: UpdateCompanyInput{AdapterType: strPtr(model.AdapterLever)},
		},
		{
			name:    "no fields provided",
			input:   UpdateCompanyInput{},
			wantErr: true,
		},
		{
			name:    "invalid adapter_type",
			input:   UpdateCompanyInput{AdapterType: strPtr("bad_adapter")},
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
	v := NewValidator()

	tests := []struct {
		name    string
		filter  CompanyFilter
		wantErr bool
	}{
		{name: "empty filter", filter: CompanyFilter{}},
		{name: "valid adapter_type", filter: CompanyFilter{AdapterType: model.AdapterGreenhouse}},
		{name: "valid active filter", filter: CompanyFilter{Active: boolPtr(true)}},
		{name: "invalid adapter_type", filter: CompanyFilter{AdapterType: "bogus"}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := v.ValidateList(tc.filter)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateList() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateSetAllActive(t *testing.T) {
	v := NewValidator()

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
