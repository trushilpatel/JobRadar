package tests

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
)

// TestNodeRest_ContractAppearsInOpenAPI is the crawlernode migration's
// per-slice quality gate contract test for the node-facing zone.
func TestNodeRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
	rest := services.NewNodeRestService(&mockFullNodeService{})
	noopAuth := func(next http.Handler) http.Handler { return next }
	router := services.NewNodeRestRouter(rest, noopAuth, zoneCfg)
	router.Register(chi.NewRouter())

	workItem, ok := zoneCfg.Paths["/work"]
	if !ok || workItem.Get == nil {
		t.Fatalf("expected GET /work in the generated spec, got: %v", zoneCfg.Paths)
	}
	if workItem.Get.OperationID != "getWork" {
		t.Errorf("get OperationID = %q, want getWork", workItem.Get.OperationID)
	}

	resultsItem, ok := zoneCfg.Paths["/results"]
	if !ok || resultsItem.Post == nil {
		t.Fatalf("expected POST /results in the generated spec, got: %v", zoneCfg.Paths)
	}
	if resultsItem.Post.OperationID != "submitResults" {
		t.Errorf("post OperationID = %q, want submitResults", resultsItem.Post.OperationID)
	}

	errorItem, ok := zoneCfg.Paths["/error"]
	if !ok || errorItem.Post == nil {
		t.Fatalf("expected POST /error in the generated spec, got: %v", zoneCfg.Paths)
	}
	if errorItem.Post.OperationID != "reportError" {
		t.Errorf("post OperationID = %q, want reportError", errorItem.Post.OperationID)
	}
}

// TestAdminCrawlerNodeRest_ContractAppearsInOpenAPI is the per-slice quality
// gate contract test for the admin-facing zone.
func TestAdminCrawlerNodeRest_ContractAppearsInOpenAPI(t *testing.T) {
	zoneCfg := apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry())
	rest := services.NewAdminRestService(&mockFullNodeService{})
	router := services.NewAdminRestRouter(rest, zoneCfg)
	router.Register(chi.NewRouter())

	rootItem, ok := zoneCfg.Paths["/"]
	if !ok || rootItem.Get == nil || rootItem.Post == nil {
		t.Fatalf("expected GET+POST / in the generated spec, got: %v", zoneCfg.Paths)
	}
	if rootItem.Get.OperationID != "listCrawlerNodes" {
		t.Errorf("list OperationID = %q, want listCrawlerNodes", rootItem.Get.OperationID)
	}
	if rootItem.Post.OperationID != "provisionCrawlerNode" {
		t.Errorf("create OperationID = %q, want provisionCrawlerNode", rootItem.Post.OperationID)
	}
	if rootItem.Post.DefaultStatus != http.StatusCreated {
		t.Errorf("create DefaultStatus = %d, want 201", rootItem.Post.DefaultStatus)
	}

	idItem, ok := zoneCfg.Paths["/{id}"]
	if !ok || idItem.Patch == nil || idItem.Delete == nil {
		t.Fatalf("expected PATCH+DELETE /{id} in the generated spec, got: %v", zoneCfg.Paths)
	}
	if idItem.Patch.OperationID != "setCrawlerNodeStatus" {
		t.Errorf("patch OperationID = %q, want setCrawlerNodeStatus", idItem.Patch.OperationID)
	}
	if idItem.Delete.OperationID != "retireCrawlerNode" {
		t.Errorf("delete OperationID = %q, want retireCrawlerNode", idItem.Delete.OperationID)
	}
}
