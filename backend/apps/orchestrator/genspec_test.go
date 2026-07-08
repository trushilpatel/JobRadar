package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoopPublisher_PublishWithMsgID_AlwaysReturnsNil(t *testing.T) {
	if err := (noopPublisher{}).PublishWithMsgID(context.Background(), "subject", []byte("payload"), "msg-1"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestBuildSpecYAML_ContainsAllOperations(t *testing.T) {
	yamlBytes, err := buildSpecYAML()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(yamlBytes)
	if !strings.Contains(got, "/node/work:") {
		t.Errorf("expected /node/work in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/node/results:") {
		t.Errorf("expected /node/results in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/node/error:") {
		t.Errorf("expected /node/error in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/admin/crawler-nodes/:") {
		t.Errorf("expected /admin/crawler-nodes/ in generated spec, got:\n%s", got)
	}
	if !strings.Contains(got, "/admin/crawler-nodes/{id}:") {
		t.Errorf("expected /admin/crawler-nodes/{id} in generated spec, got:\n%s", got)
	}
}

// TestBuildSpecYAML_DocumentsLegacyErrorShape guards the same
// registration-order finding apps/api's genspec_test.go guards: the
// documented error schema must show the legacy {"error":{"code","message"}}
// envelope, not huma's default RFC 9457 shape.
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
	if !strings.Contains(string(data), "/node/work:") {
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
