package company_test

import (
	"context"
	"testing"

	"gorm.io/datatypes"

	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// noopAdmin is the terminal pass-through used under the admin validator.
type noopAdmin struct{}

func (n *noopAdmin) List(_ context.Context, _ companydto.CompanyFilter, _ int) (companydto.CompanyAdminListResult, error) {
	return companydto.CompanyAdminListResult{}, nil
}
func (n *noopAdmin) Get(_ context.Context, _ string) (*companydto.CompanyAdminView, error) {
	return nil, nil
}
func (n *noopAdmin) Create(_ context.Context, _ companydto.CompanyPublicInput, _ companydto.CompanyScanSettingsInput) (*companydto.CompanyAdminView, error) {
	return nil, nil
}
func (n *noopAdmin) UpdatePublic(_ context.Context, _ string, _ companydto.CompanyPublicInput) error {
	return nil
}
func (n *noopAdmin) UpdateScanSettings(_ context.Context, _ string, _ companydto.CompanyScanSettingsInput) error {
	return nil
}
func (n *noopAdmin) Delete(_ context.Context, _ string) error     { return nil }
func (n *noopAdmin) SetAllActive(_ context.Context, _ bool) error { return nil }
func (n *noopAdmin) GetByIDs(_ context.Context, _ []string) ([]companydto.CompanyAdminView, error) {
	return nil, nil
}
func (n *noopAdmin) GetAll(_ context.Context) ([]companydto.CompanyAdminView, error) {
	return nil, nil
}

func newAdminValidator() companysvc.CompanyAdminService {
	return companysvc.NewAdminValidationService(&noopAdmin{})
}

func TestAdminValidator_Create(t *testing.T) {
	v := newAdminValidator()

	cases := []struct {
		name     string
		public   companydto.CompanyPublicInput
		settings companydto.CompanyScanSettingsInput
		wantErr  bool
	}{
		{
			name:   "ok minimal public",
			public: companydto.CompanyPublicInput{ID: "stripe", Name: "Stripe", Slug: "stripe"},
		},
		{
			name:    "missing id",
			public:  companydto.CompanyPublicInput{Name: "Stripe", Slug: "stripe"},
			wantErr: true,
		},
		{
			name:    "missing name",
			public:  companydto.CompanyPublicInput{ID: "stripe", Slug: "stripe"},
			wantErr: true,
		},
		{
			name:    "missing slug",
			public:  companydto.CompanyPublicInput{ID: "stripe", Name: "Stripe"},
			wantErr: true,
		},
		{
			name:     "ok with valid scan settings",
			public:   companydto.CompanyPublicInput{ID: "stripe", Name: "Stripe", Slug: "stripe"},
			settings: companydto.CompanyScanSettingsInput{AdapterType: strPtr(companycommon.AdapterGreenhouse)},
		},
		{
			name:     "invalid adapter_type rejects",
			public:   companydto.CompanyPublicInput{ID: "stripe", Name: "Stripe", Slug: "stripe"},
			settings: companydto.CompanyScanSettingsInput{AdapterType: strPtr("nope")},
			wantErr:  true,
		},
		{
			name:     "non-positive scan_interval_override rejects",
			public:   companydto.CompanyPublicInput{ID: "stripe", Name: "Stripe", Slug: "stripe"},
			settings: companydto.CompanyScanSettingsInput{ScanIntervalOverride: intPtr(0)},
			wantErr:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := v.Create(context.Background(), tc.public, tc.settings)
			if (err != nil) != tc.wantErr {
				t.Errorf("Create: err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestAdminValidator_UpdatePublic(t *testing.T) {
	v := newAdminValidator()

	cases := []struct {
		name    string
		input   companydto.CompanyPublicInput
		wantErr bool
	}{
		{name: "ok name", input: companydto.CompanyPublicInput{Name: "Renamed"}},
		{name: "ok website", input: companydto.CompanyPublicInput{Website: strPtr("https://x.com")}},
		{name: "no fields", input: companydto.CompanyPublicInput{}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.UpdatePublic(context.Background(), "any", tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("UpdatePublic: err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestAdminValidator_UpdateScanSettings(t *testing.T) {
	v := newAdminValidator()

	cases := []struct {
		name    string
		input   companydto.CompanyScanSettingsInput
		wantErr bool
	}{
		{name: "ok adapter_type", input: companydto.CompanyScanSettingsInput{AdapterType: strPtr(companycommon.AdapterLever)}},
		{name: "ok active", input: companydto.CompanyScanSettingsInput{Active: boolPtr(false)}},
		{name: "ok adapter_config", input: companydto.CompanyScanSettingsInput{AdapterConfig: jsonPtr(`{"k":"v"}`)}},
		{name: "no fields", input: companydto.CompanyScanSettingsInput{}, wantErr: true},
		{name: "invalid adapter_type", input: companydto.CompanyScanSettingsInput{AdapterType: strPtr("nope")}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.UpdateScanSettings(context.Background(), "id", tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("UpdateScanSettings: err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestAdminValidator_List(t *testing.T) {
	v := newAdminValidator()

	cases := []struct {
		name    string
		filter  companydto.CompanyFilter
		limit   int
		wantErr bool
	}{
		{name: "ok defaults", limit: 20},
		{name: "invalid adapter rejects", filter: companydto.CompanyFilter{AdapterTypes: []string{"nope"}}, limit: 20, wantErr: true},
		{name: "limit=0 rejects", limit: 0, wantErr: true},
		{name: "limit=1 minimum valid", limit: 1},
		{name: "limit=101 over-max rejects", limit: 101, wantErr: true},
		{name: "search empty is valid", filter: companydto.CompanyFilter{Search: ""}, limit: 20},
		{name: "search exactly 100 chars passes", filter: companydto.CompanyFilter{Search: stringOfLen(100)}, limit: 20},
		{name: "search 101 chars rejects", filter: companydto.CompanyFilter{Search: stringOfLen(101)}, limit: 20, wantErr: true},
		{name: "valid adapter_types pass", filter: companydto.CompanyFilter{AdapterTypes: []string{companycommon.AdapterGreenhouse}}, limit: 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := v.List(context.Background(), tc.filter, tc.limit)
			if (err != nil) != tc.wantErr {
				t.Errorf("List: err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func intPtr(n int) *int       { return &n }
func jsonPtr(s string) *datatypes.JSON {
	j := datatypes.JSON([]byte(s))
	return &j
}

var _ companysvc.CompanyAdminService = (*noopAdmin)(nil)
