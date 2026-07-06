package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSpecYAML_ContainsMigratedSliceOperations(t *testing.T) {
	yamlBytes, err := buildSpecYAML()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(yamlBytes)
	if !strings.Contains(got, "/api/companies:") {
		t.Errorf("expected /api/companies in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/api/companies/{id}:") {
		t.Errorf("expected /api/companies/{id} in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/admin/ats:") {
		t.Errorf("expected /admin/ats (first migrated /admin-zone slice) in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/admin/ats/{name}:") {
		t.Errorf("expected /admin/ats/{name} in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/admin/pipeline/stats:") {
		t.Errorf("expected /admin/pipeline/stats in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/admin/pipeline/retry:") {
		t.Errorf("expected /admin/pipeline/retry in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/admin/companies/active:") {
		t.Errorf("expected /admin/companies/active (literal, distinct from {id}) in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/admin/companies/{id}/scan-settings:") {
		t.Errorf("expected /admin/companies/{id}/scan-settings in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/admin/users/{id}/roles:") {
		t.Errorf("expected /admin/users/{id}/roles in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/admin/users/{id}/status:") {
		t.Errorf("expected /admin/users/{id}/status in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/admin/audit-log:") {
		t.Errorf("expected /admin/audit-log (final migrated slice) in generated spec, got:\n%s", got)
	}
}

// TestBuildSpecYAML_DocumentsLegacyErrorShape guards the registration-order
// finding: InstallLegacyErrorModel must run before InitializeApp/Register,
// or the documented error schema would show huma's default RFC 9457 shape
// (ErrorModel) instead of the legacy {"error":{"code","message"}} envelope
// the API actually returns.
func TestBuildSpecYAML_DocumentsLegacyErrorShape(t *testing.T) {
	yamlBytes, err := buildSpecYAML()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(yamlBytes)
	if strings.Contains(got, "ErrorModel:") {
		t.Errorf("expected huma's default RFC 9457 ErrorModel schema to be absent, got:\n%s", got)
	}
	if !strings.Contains(got, "ServiceError:") {
		t.Errorf("expected the legacy-shaped ServiceError schema to be documented, got:\n%s", got)
	}
}

func TestPrintSpec_WritesToDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openapi.yaml")
	original := specOutputPath
	specOutputPath = path
	defer func() { specOutputPath = original }()

	if err := printSpec(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected spec file to be written: %v", err)
	}
	if !strings.Contains(string(data), "/api/companies:") {
		t.Errorf("written file missing expected content:\n%s", data)
	}
}

func TestPrintSpec_WriteFailurePropagatesError(t *testing.T) {
	original := specOutputPath
	// A directory that doesn't exist as a parent guarantees os.WriteFile fails.
	specOutputPath = filepath.Join(t.TempDir(), "nonexistent-subdir", "openapi.yaml")
	defer func() { specOutputPath = original }()

	if err := printSpec(); err == nil {
		t.Fatal("expected an error when the output path's directory doesn't exist")
	}
}
