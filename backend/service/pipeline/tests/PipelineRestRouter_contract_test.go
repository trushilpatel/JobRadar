package tests

import (
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	pipelinesvc "github.com/trushilpatel/jobradar/service/pipeline/services"
)

// TestPipelineRest_ContractAppearsInOpenAPI is Slice 9's per-slice quality
// gate contract test — verifies both /pipeline operations register at their
// expected relative paths in the zone spec.
func TestPipelineRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
	rest := pipelinesvc.NewRestService(&stubPipelineService{})
	router := pipelinesvc.NewRestRouter(rest, zoneCfg)
	router.Register(chi.NewRouter())

	statsItem, ok := zoneCfg.Paths["/pipeline/stats"]
	if !ok || statsItem.Get == nil {
		t.Fatalf("expected GET /pipeline/stats in the generated spec, got: %v", zoneCfg.Paths)
	}
	if statsItem.Get.OperationID != "getPipelineStats" {
		t.Errorf("get OperationID = %q, want getPipelineStats", statsItem.Get.OperationID)
	}

	retryItem, ok := zoneCfg.Paths["/pipeline/retry"]
	if !ok || retryItem.Post == nil {
		t.Fatalf("expected POST /pipeline/retry in the generated spec, got: %v", zoneCfg.Paths)
	}
	if retryItem.Post.OperationID != "triggerPipelineRetry" {
		t.Errorf("post OperationID = %q, want triggerPipelineRetry", retryItem.Post.OperationID)
	}
}
