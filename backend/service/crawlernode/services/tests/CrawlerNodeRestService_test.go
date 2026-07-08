package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/apps/orchestrator/nodeidentity"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
)

// jsonBody encodes v as JSON into an *bytes.Buffer.
func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("jsonBody: %v", err)
	}
	return bytes.NewBuffer(b)
}

// newNodeTestRouter wires the node-facing RestRouter behind a middleware
// that injects a fixed NodeIdentity into the request context — the
// server-side equivalent of what RequireNodeToken would set on success.
// Uses a fresh, isolated zone config/registry per call (see
// apps/api/app_test.go for why never apispec.APIZoneConfig()-equivalents in
// tests).
func newNodeTestRouter(svc services.NodeService, nodeID, nodeName string) http.Handler {
	rest := services.NewNodeRestService(svc)
	authMw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := nodeidentity.NewContext(r.Context(), nodeidentity.NodeIdentity{ID: nodeID, Name: nodeName})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	router := services.NewNodeRestRouter(rest, authMw, apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry()))
	mux := chi.NewMux()
	router.Register(mux)
	return mux
}

// newAdminTestRouter wires the admin-facing RestRouter. Uses a fresh,
// isolated zone config/registry per call.
func newAdminTestRouter(svc services.NodeService) http.Handler {
	rest := services.NewAdminRestService(svc)
	router := services.NewAdminRestRouter(rest, apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry()))
	mux := chi.NewMux()
	router.Register(mux)
	return mux
}

// --- NodeRestService: GET /work ---

func TestNodeRestService_GetWork_Returns200(t *testing.T) {
	svc := &mockFullNodeService{
		getWorkFn: func(_ context.Context, _, _ string, _ []string) (*dto.WorkResponse, error) {
			return &dto.WorkResponse{}, nil
		},
	}
	srv := httptest.NewServer(newNodeTestRouter(svc, "node-1", "n1"))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/work?adapter=greenhouse")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GetWork status = %d, want 200", resp.StatusCode)
	}
}

func TestNodeRestService_GetWork_MissingAdapter_Returns200(t *testing.T) {
	svc := &mockFullNodeService{
		getWorkFn: func(_ context.Context, _, adapterType string, _ []string) (*dto.WorkResponse, error) {
			if adapterType != "" {
				t.Errorf("GetWork adapter = %q, want empty string for adapter-agnostic claim", adapterType)
			}
			return &dto.WorkResponse{}, nil
		},
	}
	srv := httptest.NewServer(newNodeTestRouter(svc, "node-1", "n1"))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/work")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GetWork (no adapter) status = %d, want 200", resp.StatusCode)
	}
}

func TestNodeRestService_GetWork_WithHoldingParam_ForwardsHoldingIDs(t *testing.T) {
	var gotHolding []string
	svc := &mockFullNodeService{
		getWorkFn: func(_ context.Context, _, _ string, holdingIDs []string) (*dto.WorkResponse, error) {
			gotHolding = holdingIDs
			return &dto.WorkResponse{}, nil
		},
	}
	srv := httptest.NewServer(newNodeTestRouter(svc, "node-1", "n1"))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/work?holding=id1,id2")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GetWork (holding) status = %d, want 200", resp.StatusCode)
	}
	if len(gotHolding) != 2 || gotHolding[0] != "id1" || gotHolding[1] != "id2" {
		t.Errorf("GetWork (holding) holdingIDs = %v, want [id1 id2]", gotHolding)
	}
}

func TestNodeRestService_GetWork_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		getWorkFn: func(_ context.Context, _, _ string, _ []string) (*dto.WorkResponse, error) {
			return nil, domainerrs.ErrInternal.WithMessage("db down")
		},
	}
	srv := httptest.NewServer(newNodeTestRouter(svc, "node-1", "n1"))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/work?adapter=greenhouse")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("GetWork (service error) status = %d, want 500", resp.StatusCode)
	}
}

// --- NodeRestService: POST /results ---

func TestNodeRestService_SubmitResults_Returns200(t *testing.T) {
	svc := &mockFullNodeService{
		submitResultsFn: func(_ context.Context, _ string, _ dto.ResultsRequest) (*dto.ResultsResponse, error) {
			return &dto.ResultsResponse{Published: 3}, nil
		},
	}
	srv := httptest.NewServer(newNodeTestRouter(svc, "node-1", "n1"))
	defer srv.Close()

	body := jsonBody(t, dto.ResultsRequest{CompanyID: "co-1", AdapterType: "greenhouse"})
	resp, err := http.Post(srv.URL+"/results", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("SubmitResults status = %d, want 200", resp.StatusCode)
	}
}

func TestNodeRestService_SubmitResults_MalformedBody_Returns400(t *testing.T) {
	svc := &mockFullNodeService{}
	srv := httptest.NewServer(newNodeTestRouter(svc, "node-1", "n1"))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/results", "application/json", bytes.NewBufferString("{bad json"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("SubmitResults (bad JSON) status = %d, want 400", resp.StatusCode)
	}
}

func TestNodeRestService_SubmitResults_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		submitResultsFn: func(_ context.Context, _ string, _ dto.ResultsRequest) (*dto.ResultsResponse, error) {
			return nil, domainerrs.ErrInternal.WithMessage("fail")
		},
	}
	srv := httptest.NewServer(newNodeTestRouter(svc, "node-1", "n1"))
	defer srv.Close()

	body := jsonBody(t, dto.ResultsRequest{CompanyID: "co-1", AdapterType: "greenhouse"})
	resp, err := http.Post(srv.URL+"/results", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("SubmitResults (service error) status = %d, want 500", resp.StatusCode)
	}
}

// --- NodeRestService: POST /error ---

func TestNodeRestService_ReportError_Returns204(t *testing.T) {
	svc := &mockFullNodeService{
		reportErrorFn: func(_ context.Context, _ string, _ dto.ErrorRequest) error { return nil },
	}
	srv := httptest.NewServer(newNodeTestRouter(svc, "node-1", "n1"))
	defer srv.Close()

	body := jsonBody(t, dto.ErrorRequest{AssignmentID: "q-1"})
	resp, err := http.Post(srv.URL+"/error", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("ReportError status = %d, want 204", resp.StatusCode)
	}
}

func TestNodeRestService_ReportError_MalformedBody_Returns400(t *testing.T) {
	svc := &mockFullNodeService{}
	srv := httptest.NewServer(newNodeTestRouter(svc, "node-1", "n1"))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/error", "application/json", bytes.NewBufferString("{bad"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("ReportError (bad JSON) status = %d, want 400", resp.StatusCode)
	}
}

func TestNodeRestService_ReportError_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		reportErrorFn: func(_ context.Context, _ string, _ dto.ErrorRequest) error {
			return domainerrs.ErrInternal.WithMessage("fail")
		},
	}
	srv := httptest.NewServer(newNodeTestRouter(svc, "node-1", "n1"))
	defer srv.Close()

	body := jsonBody(t, dto.ErrorRequest{AssignmentID: "q-1"})
	resp, err := http.Post(srv.URL+"/error", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("ReportError (service error) status = %d, want 500", resp.StatusCode)
	}
}

// --- AdminRestService: GET / ---

func TestAdminRestService_ListNodes_Returns200(t *testing.T) {
	svc := &mockFullNodeService{
		listNodesFn: func(_ context.Context, _ string) ([]dto.NodeView, error) {
			return []dto.NodeView{{ID: "node-1"}}, nil
		},
	}
	srv := httptest.NewServer(newAdminTestRouter(svc))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("ListNodes status = %d, want 200", resp.StatusCode)
	}
}

func TestAdminRestService_ListNodes_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		listNodesFn: func(_ context.Context, _ string) ([]dto.NodeView, error) {
			return nil, domainerrs.ErrInternal.WithMessage("db down")
		},
	}
	srv := httptest.NewServer(newAdminTestRouter(svc))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("ListNodes (error) status = %d, want 500", resp.StatusCode)
	}
}

// --- AdminRestService: POST / ---

func TestAdminRestService_ProvisionNode_Returns201(t *testing.T) {
	svc := &mockFullNodeService{
		provisionFn: func(_ context.Context, _ string) (*dto.CreateNodeResponse, error) {
			return &dto.CreateNodeResponse{Token: "abc"}, nil
		},
	}
	srv := httptest.NewServer(newAdminTestRouter(svc))
	defer srv.Close()

	body := jsonBody(t, map[string]string{"name": "prod-node"})
	resp, err := http.Post(srv.URL+"/", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("ProvisionNode status = %d, want 201", resp.StatusCode)
	}
}

func TestAdminRestService_ProvisionNode_MalformedBody_Returns400(t *testing.T) {
	svc := &mockFullNodeService{}
	srv := httptest.NewServer(newAdminTestRouter(svc))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/", "application/json", bytes.NewBufferString("{bad"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("ProvisionNode (bad JSON) status = %d, want 400", resp.StatusCode)
	}
}

func TestAdminRestService_ProvisionNode_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		provisionFn: func(_ context.Context, _ string) (*dto.CreateNodeResponse, error) {
			return nil, domainerrs.ErrInternal.WithMessage("fail")
		},
	}
	srv := httptest.NewServer(newAdminTestRouter(svc))
	defer srv.Close()

	body := jsonBody(t, map[string]string{"name": "n"})
	resp, err := http.Post(srv.URL+"/", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("ProvisionNode (error) status = %d, want 500", resp.StatusCode)
	}
}

// --- AdminRestService: PATCH /{id} ---

func TestAdminRestService_SetNodeStatus_Returns200(t *testing.T) {
	svc := &mockFullNodeService{
		setStatusFn: func(_ context.Context, _, _ string) error { return nil },
	}
	srv := httptest.NewServer(newAdminTestRouter(svc))
	defer srv.Close()

	body := jsonBody(t, map[string]string{"status": "disabled"})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/node-1", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("SetNodeStatus status = %d, want 200", resp.StatusCode)
	}
}

func TestAdminRestService_SetNodeStatus_MalformedBody_Returns400(t *testing.T) {
	svc := &mockFullNodeService{}
	srv := httptest.NewServer(newAdminTestRouter(svc))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/node-1", bytes.NewBufferString("{bad"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("SetNodeStatus (bad JSON) status = %d, want 400", resp.StatusCode)
	}
}

func TestAdminRestService_SetNodeStatus_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		setStatusFn: func(_ context.Context, _, _ string) error {
			return domainerrs.ErrInternal.WithMessage("fail")
		},
	}
	srv := httptest.NewServer(newAdminTestRouter(svc))
	defer srv.Close()

	body := jsonBody(t, map[string]string{"status": "disabled"})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/node-1", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("SetNodeStatus (error) status = %d, want 500", resp.StatusCode)
	}
}

// --- AdminRestService: DELETE /{id} ---

func TestAdminRestService_RetireNode_Returns204(t *testing.T) {
	svc := &mockFullNodeService{
		retireFn: func(_ context.Context, _ string) error { return nil },
	}
	srv := httptest.NewServer(newAdminTestRouter(svc))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/node-1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("RetireNode status = %d, want 204", resp.StatusCode)
	}
}

func TestAdminRestService_RetireNode_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		retireFn: func(_ context.Context, _ string) error {
			return domainerrs.ErrInternal.WithMessage("fail")
		},
	}
	srv := httptest.NewServer(newAdminTestRouter(svc))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/node-1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("RetireNode (error) status = %d, want 500", resp.StatusCode)
	}
}

// --- Router smoke tests ---

func TestNodeRestRouter_Register_MountsRoutes(t *testing.T) {
	svc := &mockFullNodeService{
		getWorkFn: func(_ context.Context, _, _ string, _ []string) (*dto.WorkResponse, error) {
			return &dto.WorkResponse{}, nil
		},
	}
	rs := services.NewNodeRestService(svc)
	authMw := func(next http.Handler) http.Handler { return next }
	router := services.NewNodeRestRouter(rs, authMw, apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry()))
	r := chi.NewRouter()
	router.Register(r) // must not panic
}

func TestAdminRestRouter_Register_MountsRoutes(t *testing.T) {
	svc := &mockFullNodeService{}
	rs := services.NewAdminRestService(svc)
	router := services.NewAdminRestRouter(rs, apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry()))
	r := chi.NewRouter()
	router.Register(r) // must not panic
}

// Compile-time interface checks.
var _ *services.NodeRestService = (*services.NodeRestService)(nil)
var _ *services.AdminRestService = (*services.AdminRestService)(nil)
