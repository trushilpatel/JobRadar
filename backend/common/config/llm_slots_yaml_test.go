package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLLMSlotsYAML_ExpandsEnvRefs(t *testing.T) {
	t.Setenv("GEMINI_KEY_A", "sk-real-A")
	t.Setenv("GEMINI_KEY_B", "sk-real-B")

	yaml := `filter:
  slots:
    - key_id: k1
      api_key: ${GEMINI_KEY_A}
      model: gemini-2.5-flash-lite
      rpm: 15
      rpd: 1500
      tpm_in: 1000000
enricher:
  slots:
    - key_id: k2
      api_key: ${GEMINI_KEY_B}
      model: gemini-2.5-pro
      rpm: 5
      rpd: 500
      tpm_in: 250000
`
	path := filepath.Join(t.TempDir(), "slots.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	filter, enricher, err := LoadLLMSlotsYAML(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(filter) != 1 || filter[0].APIKey != "sk-real-A" || filter[0].Model != "gemini-2.5-flash-lite" {
		t.Fatalf("filter slot mismatch: %+v", filter)
	}
	if len(enricher) != 1 || enricher[0].APIKey != "sk-real-B" || enricher[0].RPM != 5 {
		t.Fatalf("enricher slot mismatch: %+v", enricher)
	}
}

func TestLoadLLMSlotsYAML_EmptyPathReturnsNil(t *testing.T) {
	f, e, err := LoadLLMSlotsYAML("")
	if err != nil || f != nil || e != nil {
		t.Fatalf("want nil nil nil, got (%v, %v, %v)", f, e, err)
	}
}

func TestLoadLLMSlotsYAML_MissingFile(t *testing.T) {
	_, _, err := LoadLLMSlotsYAML("/does/not/exist.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadLLMSlotsYAML_UnresolvedEnvRef(t *testing.T) {
	// Ensure target var is unset; t.Setenv does not provide Unsetenv, use os.Unsetenv directly.
	_ = os.Unsetenv("MISSING_VAR_XYZ")

	yaml := `filter:
  slots:
    - key_id: k1
      api_key: ${MISSING_VAR_XYZ}
      model: gemini-2.5-flash-lite
      rpm: 15
      rpd: 1500
      tpm_in: 1000000
enricher:
  slots:
    - key_id: k2
      api_key: ${MISSING_VAR_XYZ}
      model: gemini-2.5-pro
      rpm: 5
      rpd: 500
      tpm_in: 250000
`
	path := filepath.Join(t.TempDir(), "slots.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := LoadLLMSlotsYAML(path)
	if err == nil {
		t.Fatal("expected error for unresolved env ref")
	}
	if !strings.Contains(err.Error(), "MISSING_VAR_XYZ") {
		t.Fatalf("expected error to mention MISSING_VAR_XYZ, got: %v", err)
	}
	// De-dupe: should appear exactly once even though YAML references it twice.
	if strings.Count(err.Error(), "MISSING_VAR_XYZ") != 1 {
		t.Fatalf("expected MISSING_VAR_XYZ to appear once (de-duped), got: %v", err)
	}
}
