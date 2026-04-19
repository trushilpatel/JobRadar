package company_test

import (
	"context"
	"testing"

	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// noopService is the terminal pass-through used under the validation layer in tests.
type noopService struct{}

func (n *noopService) List(_ context.Context, _ companydto.CompanyFilter, _, _ int) (companydto.CompanyListResult, error) {
	return companydto.CompanyListResult{}, nil
}
func (n *noopService) GetAll(_ context.Context) ([]companydto.CompanyView, error) { return nil, nil }
func (n *noopService) Get(_ context.Context, _ string) (*companymodel.Company, error) {
	return nil, nil
}
func (n *noopService) Create(_ context.Context, _ companydto.CreateCompanyInput) (*companymodel.Company, error) {
	return nil, nil
}
func (n *noopService) Update(_ context.Context, _ string, _ companydto.UpdateCompanyInput) error {
	return nil
}
func (n *noopService) Delete(_ context.Context, _ string) error     { return nil }
func (n *noopService) SetAllActive(_ context.Context, _ bool) error { return nil }

func newValidator() companysvc.Service {
	return companysvc.NewValidator(&noopService{})
}

func boolPtr(b bool) *bool    { return &b }
func strPtr(s string) *string { return &s }

func TestValidateCreate(t *testing.T) {
	v := newValidator()

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
			_, err := v.Create(context.Background(), tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateUpdate(t *testing.T) {
	v := newValidator()

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
			err := v.Update(context.Background(), "any-id", tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("Update() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateList(t *testing.T) {
	v := newValidator()

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
			_, err := v.List(context.Background(), tc.filter, tc.page, tc.limit)
			if (err != nil) != tc.wantErr {
				t.Errorf("List() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateSetAllActive(t *testing.T) {
	v := newValidator()

	// nil active pointer — the REST layer guards this before calling SetAllActive;
	// at the Service level the bool is already unwrapped, so SetAllActive always passes.
	if err := v.SetAllActive(context.Background(), true); err != nil {
		t.Errorf("unexpected error for true active: %v", err)
	}
	if err := v.SetAllActive(context.Background(), false); err != nil {
		t.Errorf("unexpected error for false active: %v", err)
	}
}
