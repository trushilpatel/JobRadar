package tests

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	resumesvc "github.com/trushilpatel/jobradar/service/resume/services"
)

// TestResumeRest_ContractAppearsInOpenAPI is the Slice 5 per-slice quality
// gate's contract test.
func TestResumeRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
	rest := resumesvc.NewRestService(&fakeService{})
	router := resumesvc.NewRestRouter(rest, zoneCfg)
	router.Register(chi.NewRouter())

	listItem, ok := zoneCfg.Paths["/resumes"]
	if !ok || listItem.Get == nil || listItem.Post == nil {
		t.Fatalf("expected GET+POST /resumes in the generated spec, got: %v", zoneCfg.Paths)
	}
	if listItem.Get.OperationID != "listResumes" {
		t.Errorf("list OperationID = %q, want listResumes", listItem.Get.OperationID)
	}
	if listItem.Post.OperationID != "uploadResume" {
		t.Errorf("upload OperationID = %q, want uploadResume", listItem.Post.OperationID)
	}
	if listItem.Post.DefaultStatus != http.StatusCreated {
		t.Errorf("upload DefaultStatus = %d, want 201", listItem.Post.DefaultStatus)
	}

	// The multipart upload form must be documented — file + label fields.
	reqBody := listItem.Post.RequestBody
	if reqBody == nil || reqBody.Content["multipart/form-data"] == nil {
		t.Fatalf("expected upload to document a multipart/form-data request body, got: %+v", reqBody)
	}
	schema := reqBody.Content["multipart/form-data"].Schema
	if _, ok := schema.Properties["file"]; !ok {
		t.Errorf("expected multipart schema to document a 'file' field, got: %v", schema.Properties)
	}
	if _, ok := schema.Properties["label"]; !ok {
		t.Errorf("expected multipart schema to document a 'label' field, got: %v", schema.Properties)
	}

	idItem, ok := zoneCfg.Paths["/resumes/{id}"]
	if !ok || idItem.Patch == nil || idItem.Delete == nil {
		t.Fatalf("expected PATCH+DELETE /resumes/{id} in the generated spec, got: %v", zoneCfg.Paths)
	}
	if idItem.Patch.OperationID != "renameResume" {
		t.Errorf("rename OperationID = %q, want renameResume", idItem.Patch.OperationID)
	}
	if idItem.Delete.OperationID != "deleteResume" {
		t.Errorf("delete OperationID = %q, want deleteResume", idItem.Delete.OperationID)
	}

	downloadItem, ok := zoneCfg.Paths["/resumes/{id}/download"]
	if !ok || downloadItem.Get == nil {
		t.Fatalf("expected GET /resumes/{id}/download in the generated spec, got: %v", zoneCfg.Paths)
	}
	if downloadItem.Get.OperationID != "downloadResume" {
		t.Errorf("download OperationID = %q, want downloadResume", downloadItem.Get.OperationID)
	}
}
