package company_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gorm.io/datatypes"

	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
)

// sampleScanSettings returns a fully-populated scan_settings model with a
// non-empty adapter_config, used to verify credential redaction in the list
// view.
func sampleScanSettings() *companymodel.CompanyScanSettings {
	now := time.Now()
	return &companymodel.CompanyScanSettings{
		CompanyID:         "acme",
		AdapterType:       "greenhouse",
		AdapterConfig:     datatypes.JSON([]byte(`{"board":"acme","token":"secret"}`)),
		Active:            true,
		LastScanAt:        &now,
		ConsecutiveErrors: 0,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

// TestScanSettingsView_DetailIncludesAdapterConfig verifies P2: the detail
// view (CompanyScanSettingsView) carries adapter_config in its JSON.
func TestScanSettingsView_DetailIncludesAdapterConfig(t *testing.T) {
	v := companydto.ScanSettingsViewFromModel(sampleScanSettings())
	if v == nil {
		t.Fatal("expected non-nil view")
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), "adapter_config") {
		t.Errorf("detail view JSON must contain adapter_config, got %s", b)
	}
	if !strings.Contains(string(b), "secret") {
		t.Errorf("detail view must carry the adapter_config payload, got %s", b)
	}
}

// TestScanSettingsListView_OmitsAdapterConfig verifies P2: the list view
// (CompanyScanSettingsListView) NEVER serialises adapter_config — credential
// redaction in paginated responses.
func TestScanSettingsListView_OmitsAdapterConfig(t *testing.T) {
	v := companydto.ScanSettingsListViewFromModel(sampleScanSettings())
	if v == nil {
		t.Fatal("expected non-nil view")
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "adapter_config") {
		t.Errorf("list view JSON must NOT contain adapter_config, got %s", b)
	}
	if strings.Contains(string(b), "secret") {
		t.Errorf("list view leaked adapter_config payload, got %s", b)
	}
}

// TestAdminListView_OmitsAdapterConfig verifies P2 end-to-end: the full admin
// LIST item shape (embeds the list scan-settings view) has no adapter_config,
// while the DETAIL item shape does.
func TestAdminListView_OmitsAdapterConfig(t *testing.T) {
	listItem := companydto.CompanyAdminListView{
		CompanyAdminCommon: companydto.CompanyAdminCommon{Company: companymodel.Company{ID: "acme", Name: "Acme", Slug: "acme"}},
		ScanSettings:       companydto.ScanSettingsListViewFromModel(sampleScanSettings()),
	}
	lb, _ := json.Marshal(listItem)
	if strings.Contains(string(lb), "adapter_config") {
		t.Errorf("admin list item must not expose adapter_config, got %s", lb)
	}
	// Public fields still promote flat (embedded), so id is present at top level.
	if !strings.Contains(string(lb), `"id":"acme"`) {
		t.Errorf("embedded company fields must promote flat, got %s", lb)
	}

	detailItem := companydto.CompanyAdminView{
		CompanyAdminCommon: companydto.CompanyAdminCommon{Company: companymodel.Company{ID: "acme", Name: "Acme", Slug: "acme"}},
		ScanSettings:       companydto.ScanSettingsViewFromModel(sampleScanSettings()),
	}
	db, _ := json.Marshal(detailItem)
	if !strings.Contains(string(db), "adapter_config") {
		t.Errorf("admin detail item must expose adapter_config, got %s", db)
	}
}

// TestScanSettingsViewFromModel_Nil verifies P2: both converters return nil on
// a nil model so LEFT-JOIN absence passes straight through.
func TestScanSettingsViewFromModel_Nil(t *testing.T) {
	if companydto.ScanSettingsViewFromModel(nil) != nil {
		t.Error("ScanSettingsViewFromModel(nil) must be nil")
	}
	if companydto.ScanSettingsListViewFromModel(nil) != nil {
		t.Error("ScanSettingsListViewFromModel(nil) must be nil")
	}
}

// TestScanSettingsView_EmbedsListView verifies P2: the detail view embeds the
// list view, so a base field set once is visible through the detail view too.
func TestScanSettingsView_EmbedsListView(t *testing.T) {
	v := companydto.ScanSettingsViewFromModel(sampleScanSettings())
	if v.AdapterType != "greenhouse" {
		t.Errorf("promoted base field AdapterType = %q, want greenhouse", v.AdapterType)
	}
	if v.CompanyID != "acme" {
		t.Errorf("promoted base field CompanyID = %q, want acme", v.CompanyID)
	}
}
