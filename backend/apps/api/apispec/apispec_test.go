package apispec

import "testing"

func TestBuild_ReturnsValidSpec(t *testing.T) {
	spec, err := Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec == nil {
		t.Fatal("expected a non-nil spec")
	}
	if spec.Info == nil || spec.Info.Version != Version {
		t.Fatalf("expected spec.Info.Version %q, got %+v", Version, spec.Info)
	}
}

func TestBuild_NoMigratedZonesYetProducesEmptyPaths(t *testing.T) {
	// No slice has migrated to Huma yet (docs/superpowers/plans/2026-06-13-huma-migration-plan.md,
	// Section 5) — this test documents that today's baseline is an empty but
	// valid document, and must be updated (not deleted) as the first zone
	// (Slice 1: company public) is added to Build.
	spec, err := Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(spec.Paths) != 0 {
		t.Fatalf("expected zero paths before any slice migrates, got %d: %v", len(spec.Paths), spec.Paths)
	}
}
