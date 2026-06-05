package company_test

import (
	"context"
	"testing"

	"gorm.io/datatypes"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
	companycommon "github.com/trushilpatel/jobradar/service/company/common"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	scancommon "github.com/trushilpatel/jobradar/service/scan/common"
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

// mockAtsService is a configurable AtsService test double.
// Known adapters return a valid DTO; unknown names return ErrNotFound.
// Adapters listed in notCreatable have IsAdminCreatable=false.
type mockAtsService struct {
	known        map[string]atsdto.ATSDTO
	notCreatable map[string]bool
}

func newMockAts() *mockAtsService {
	known := map[string]atsdto.ATSDTO{}
	notCreatable := map[string]bool{}

	// Seed every adapter from the canonical constants list.
	for _, at := range companycommon.AdapterTypes {
		known[at] = atsdto.ATSDTO{
			Name:             at,
			IsAdminCreatable: !companycommon.AdminUnsupportedAdapters[at],
		}
		if companycommon.AdminUnsupportedAdapters[at] {
			notCreatable[at] = true
		}
	}
	return &mockAtsService{known: known, notCreatable: notCreatable}
}

func (m *mockAtsService) ListATS(_ context.Context) ([]atsdto.ATSDTO, error) {
	out := make([]atsdto.ATSDTO, 0, len(m.known))
	for _, v := range m.known {
		out = append(out, v)
	}
	return out, nil
}

func (m *mockAtsService) GetATS(_ context.Context, name string) (atsdto.ATSDTO, error) {
	dto, ok := m.known[name]
	if !ok {
		return atsdto.ATSDTO{}, apperr.ErrNotFound.WithMessage("ats not found: " + name)
	}
	return dto, nil
}

func (m *mockAtsService) UpdateATS(_ context.Context, _ string, _ atsdto.UpdateATSInput) (atsdto.ATSDTO, error) {
	return atsdto.ATSDTO{}, nil
}

var _ atssvc.AtsService = (*mockAtsService)(nil)

func newAdminValidator() companysvc.CompanyAdminService {
	return companysvc.NewAdminValidationService(&noopAdmin{}, newMockAts())
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
		{
			// P1.2: Create requires adapter_type when a non-empty scan_settings
			// block is supplied — Active=true alone is not enough.
			name:     "non-empty settings without adapter_type rejects on Create",
			public:   companydto.CompanyPublicInput{ID: "stripe", Name: "Stripe", Slug: "stripe"},
			settings: companydto.CompanyScanSettingsInput{Active: boolPtr(true)},
			wantErr:  true,
		},
		{
			name:     "empty settings is allowed on Create (un-onboarded company)",
			public:   companydto.CompanyPublicInput{ID: "stripe", Name: "Stripe", Slug: "stripe"},
			settings: companydto.CompanyScanSettingsInput{},
		},
		{
			// eightfold is registered but not admin-creatable.
			name:     "eightfold adapter rejects on Create (not admin creatable)",
			public:   companydto.CompanyPublicInput{ID: "stripe", Name: "Stripe", Slug: "stripe"},
			settings: companydto.CompanyScanSettingsInput{AdapterType: strPtr(companycommon.AdapterEightfold)},
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
		// P1.2: Update does NOT require adapter_type — partial-patch of
		// Active alone is a legitimate update (requireAdapter=false).
		{name: "active-only patch allowed on Update", input: companydto.CompanyScanSettingsInput{Active: boolPtr(true)}},
		// eightfold is registered but not admin-creatable — UpdateScanSettings
		// uses requireAdapter=false so the is_admin_creatable check is skipped.
		{name: "eightfold allowed on Update (requireAdapter=false)", input: companydto.CompanyScanSettingsInput{AdapterType: strPtr(companycommon.AdapterEightfold)}},
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
		// P1.6: scan_status is now validated against the allowed set.
		{name: "scan_status empty allowed", filter: companydto.CompanyFilter{ScanStatus: ""}, limit: 20},
		{name: "scan_status queued allowed", filter: companydto.CompanyFilter{ScanStatus: scancommon.QueueStatusQueued}, limit: 20},
		{name: "scan_status running allowed", filter: companydto.CompanyFilter{ScanStatus: scancommon.QueueStatusRunning}, limit: 20},
		{name: "scan_status error allowed", filter: companydto.CompanyFilter{ScanStatus: scancommon.ScanStatusError}, limit: 20},
		{name: "scan_status ok allowed", filter: companydto.CompanyFilter{ScanStatus: scancommon.ScanStatusOK}, limit: 20},
		{name: "scan_status bogus rejects", filter: companydto.CompanyFilter{ScanStatus: "bogus"}, limit: 20, wantErr: true},
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

// TestAdminValidator_AdapterAllowList_DerivedFromMockAts verifies that every
// adapter in common.AdapterTypes (except admin-unsupported ones) passes
// validation, admin-unsupported adapters are rejected on Create, and an
// unknown adapter string is always rejected.
func TestAdminValidator_AdapterAllowList_DerivedFromMockAts(t *testing.T) {
	v := newAdminValidator()
	for _, at := range companycommon.AdapterTypes {
		at := at
		t.Run(at, func(t *testing.T) {
			settings := companydto.CompanyScanSettingsInput{AdapterType: strPtr(at)}
			_, err := v.Create(context.Background(),
				companydto.CompanyPublicInput{ID: "c", Name: "C", Slug: "c"}, settings)
			wantRejected := companycommon.AdminUnsupportedAdapters[at]
			if wantRejected && err == nil {
				t.Errorf("adapter %q is admin-unsupported but validated ok", at)
			}
			if !wantRejected && err != nil {
				t.Errorf("adapter %q should validate, got %v", at, err)
			}
		})
	}

	// eightfold is the canonical admin-unsupported adapter — must be rejected on Create.
	_, err := v.Create(context.Background(),
		companydto.CompanyPublicInput{ID: "c", Name: "C", Slug: "c"},
		companydto.CompanyScanSettingsInput{AdapterType: strPtr(companycommon.AdapterEightfold)})
	if err == nil {
		t.Errorf("eightfold must be rejected by the admin validator on Create")
	}

	// Unknown string is rejected.
	_, err = v.Create(context.Background(),
		companydto.CompanyPublicInput{ID: "c", Name: "C", Slug: "c"},
		companydto.CompanyScanSettingsInput{AdapterType: strPtr("totally-unknown")})
	if err == nil {
		t.Errorf("unknown adapter must be rejected")
	}
}

// TestAdapterTypes_NoDuplicates verifies the canonical adapter list
// has no duplicate entries.
func TestAdapterTypes_NoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, at := range companycommon.AdapterTypes {
		if seen[at] {
			t.Errorf("duplicate adapter in common.AdapterTypes: %q", at)
		}
		seen[at] = true
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
