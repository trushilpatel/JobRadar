package adaptermeta_test

import (
	"testing"

	"github.com/trushilpatel/jobradar/service/ats/adaptermeta"
)

func TestRequiresConfig_KnownConfigurableAdapters(t *testing.T) {
	for _, at := range []string{"workday", "eightfold"} {
		if !adaptermeta.RequiresConfig(at) {
			t.Errorf("RequiresConfig(%q) = false, want true", at)
		}
	}
}

func TestRequiresConfig_NonConfigurableAdapters(t *testing.T) {
	for _, at := range []string{
		"greenhouse", "lever", "ashby", "google", "meta", "netflix",
		"uber", "rippling", "microsoft", "apple", "gem", "deel",
	} {
		if adaptermeta.RequiresConfig(at) {
			t.Errorf("RequiresConfig(%q) = true, want false", at)
		}
	}
}

func TestRequiresConfig_UnknownAdapterDefaultsFalse(t *testing.T) {
	if adaptermeta.RequiresConfig("totally-unknown-adapter") {
		t.Error("RequiresConfig(unknown) = true, want false (permissive default)")
	}
}

func TestRequiresConfig_EmptyStringDefaultsFalse(t *testing.T) {
	if adaptermeta.RequiresConfig("") {
		t.Error(`RequiresConfig("") = true, want false`)
	}
}

func TestRequiresConfig_IsCaseSensitiveExactMatch(t *testing.T) {
	// No case normalization — matches the exact-match convention used
	// elsewhere for adapter type strings (e.g. the ats table's seed data).
	if adaptermeta.RequiresConfig("Workday") {
		t.Error(`RequiresConfig("Workday") = true, want false (exact match only, no case folding)`)
	}
}
