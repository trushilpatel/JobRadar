package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/orchestrator/nodeidentity"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
)

// withChiID injects a chi route param {id} = nodeID into the request context.
func withChiID(r *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// withNodeIdentity injects a NodeIdentity into the request context.
func withNodeIdentity(r *http.Request, id, name string) *http.Request {
	ctx := nodeidentity.NewContext(r.Context(), nodeidentity.NodeIdentity{ID: id, Name: name})
	return r.WithContext(ctx)
}

// jsonBody encodes v as JSON into an *bytes.Buffer.
func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("jsonBody: %v", err)
	}
	return bytes.NewBuffer(b)
}

// --- NodeRestService ---

// getWork: 200 when service returns a work response.
func TestNodeRestService_GetWork_Returns200(t *testing.T) {
	svc := &mockFullNodeService{
		getWorkFn: func(_ context.Context, _, _ string, _ []string) (*dto.WorkResponse, error) {
			return &dto.WorkResponse{}, nil
		},
	}
	rs := services.NewNodeRestService(svc)
	req := withNodeIdentity(httptest.NewRequest(http.MethodGet, "/node/work?adapter=greenhouse", nil), "node-1", "n1")
	rr := httptest.NewRecorder()
	rs.GetWork(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GetWork status = %d, want 200", rr.Code)
	}
}

// getWork: missing adapter query param → 400.
func TestNodeRestService_GetWork_MissingAdapter_Returns400(t *testing.T) {
	svc := &mockFullNodeService{}
	rs := services.NewNodeRestService(svc)
	req := withNodeIdentity(httptest.NewRequest(http.MethodGet, "/node/work", nil), "node-1", "n1")
	rr := httptest.NewRecorder()
	rs.GetWork(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GetWork (no adapter) status = %d, want 400", rr.Code)
	}
}

// getWork: service error propagates via HandleError.
func TestNodeRestService_GetWork_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		getWorkFn: func(_ context.Context, _, _ string, _ []string) (*dto.WorkResponse, error) {
			return nil, domainerrs.ErrInternal.WithMessage("db down")
		},
	}
	rs := services.NewNodeRestService(svc)
	req := withNodeIdentity(httptest.NewRequest(http.MethodGet, "/node/work?adapter=greenhouse", nil), "node-1", "n1")
	rr := httptest.NewRecorder()
	rs.GetWork(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("GetWork (service error) status = %d, want 500", rr.Code)
	}
}

// submitResults: 200 on success.
func TestNodeRestService_SubmitResults_Returns200(t *testing.T) {
	svc := &mockFullNodeService{
		submitResultsFn: func(_ context.Context, _ string, _ dto.ResultsRequest) (*dto.ResultsResponse, error) {
			return &dto.ResultsResponse{Published: 3}, nil
		},
	}
	rs := services.NewNodeRestService(svc)
	body := jsonBody(t, dto.ResultsRequest{CompanyID: "co-1", AdapterType: "greenhouse"})
	req := withNodeIdentity(httptest.NewRequest(http.MethodPost, "/node/results", body), "node-1", "n1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rs.SubmitResults(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("SubmitResults status = %d, want 200", rr.Code)
	}
}

// submitResults: malformed JSON → 400.
func TestNodeRestService_SubmitResults_MalformedBody_Returns400(t *testing.T) {
	svc := &mockFullNodeService{}
	rs := services.NewNodeRestService(svc)
	req := withNodeIdentity(httptest.NewRequest(http.MethodPost, "/node/results", bytes.NewBufferString("{bad json")), "node-1", "n1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rs.SubmitResults(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("SubmitResults (bad JSON) status = %d, want 400", rr.Code)
	}
}

// submitResults: service error propagates.
func TestNodeRestService_SubmitResults_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		submitResultsFn: func(_ context.Context, _ string, _ dto.ResultsRequest) (*dto.ResultsResponse, error) {
			return nil, domainerrs.ErrInternal.WithMessage("fail")
		},
	}
	rs := services.NewNodeRestService(svc)
	body := jsonBody(t, dto.ResultsRequest{CompanyID: "co-1", AdapterType: "greenhouse"})
	req := withNodeIdentity(httptest.NewRequest(http.MethodPost, "/node/results", body), "node-1", "n1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rs.SubmitResults(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("SubmitResults (service error) status = %d, want 500", rr.Code)
	}
}

// reportError: 204 on success.
func TestNodeRestService_ReportError_Returns204(t *testing.T) {
	svc := &mockFullNodeService{
		reportErrorFn: func(_ context.Context, _ string, _ dto.ErrorRequest) error { return nil },
	}
	rs := services.NewNodeRestService(svc)
	body := jsonBody(t, dto.ErrorRequest{AssignmentID: "q-1"})
	req := withNodeIdentity(httptest.NewRequest(http.MethodPost, "/node/error", body), "node-1", "n1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rs.ReportError(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("ReportError status = %d, want 204", rr.Code)
	}
}

// reportError: malformed JSON → 400.
func TestNodeRestService_ReportError_MalformedBody_Returns400(t *testing.T) {
	svc := &mockFullNodeService{}
	rs := services.NewNodeRestService(svc)
	req := withNodeIdentity(httptest.NewRequest(http.MethodPost, "/node/error", bytes.NewBufferString("{bad")), "node-1", "n1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rs.ReportError(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("ReportError (bad JSON) status = %d, want 400", rr.Code)
	}
}

// reportError: service error propagates.
func TestNodeRestService_ReportError_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		reportErrorFn: func(_ context.Context, _ string, _ dto.ErrorRequest) error {
			return domainerrs.ErrInternal.WithMessage("fail")
		},
	}
	rs := services.NewNodeRestService(svc)
	body := jsonBody(t, dto.ErrorRequest{AssignmentID: "q-1"})
	req := withNodeIdentity(httptest.NewRequest(http.MethodPost, "/node/error", body), "node-1", "n1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rs.ReportError(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("ReportError (service error) status = %d, want 500", rr.Code)
	}
}

// --- AdminRestService ---

// listNodes: 200 with nodes.
func TestAdminRestService_ListNodes_Returns200(t *testing.T) {
	svc := &mockFullNodeService{
		listNodesFn: func(_ context.Context, _ string) ([]dto.NodeView, error) {
			return []dto.NodeView{{ID: "node-1"}}, nil
		},
	}
	rs := services.NewAdminRestService(svc)
	req := httptest.NewRequest(http.MethodGet, "/admin/crawler-nodes", nil)
	rr := httptest.NewRecorder()
	rs.ListNodes(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("ListNodes status = %d, want 200", rr.Code)
	}
}

// listNodes: service error propagates.
func TestAdminRestService_ListNodes_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		listNodesFn: func(_ context.Context, _ string) ([]dto.NodeView, error) {
			return nil, domainerrs.ErrInternal.WithMessage("db down")
		},
	}
	rs := services.NewAdminRestService(svc)
	req := httptest.NewRequest(http.MethodGet, "/admin/crawler-nodes", nil)
	rr := httptest.NewRecorder()
	rs.ListNodes(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("ListNodes (error) status = %d, want 500", rr.Code)
	}
}

// provisionNode: 201 with token response.
func TestAdminRestService_ProvisionNode_Returns201(t *testing.T) {
	svc := &mockFullNodeService{
		provisionFn: func(_ context.Context, _ string) (*dto.CreateNodeResponse, error) {
			return &dto.CreateNodeResponse{Token: "abc"}, nil
		},
	}
	rs := services.NewAdminRestService(svc)
	body := jsonBody(t, map[string]string{"name": "prod-node"})
	req := httptest.NewRequest(http.MethodPost, "/admin/crawler-nodes", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rs.ProvisionNode(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("ProvisionNode status = %d, want 201", rr.Code)
	}
}

// provisionNode: malformed JSON → 400.
func TestAdminRestService_ProvisionNode_MalformedBody_Returns400(t *testing.T) {
	svc := &mockFullNodeService{}
	rs := services.NewAdminRestService(svc)
	req := httptest.NewRequest(http.MethodPost, "/admin/crawler-nodes", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rs.ProvisionNode(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("ProvisionNode (bad JSON) status = %d, want 400", rr.Code)
	}
}

// provisionNode: service error propagates.
func TestAdminRestService_ProvisionNode_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		provisionFn: func(_ context.Context, _ string) (*dto.CreateNodeResponse, error) {
			return nil, domainerrs.ErrInternal.WithMessage("fail")
		},
	}
	rs := services.NewAdminRestService(svc)
	body := jsonBody(t, map[string]string{"name": "n"})
	req := httptest.NewRequest(http.MethodPost, "/admin/crawler-nodes", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rs.ProvisionNode(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("ProvisionNode (error) status = %d, want 500", rr.Code)
	}
}

// setNodeStatus: 200 on success.
func TestAdminRestService_SetNodeStatus_Returns200(t *testing.T) {
	svc := &mockFullNodeService{
		setStatusFn: func(_ context.Context, _, _ string) error { return nil },
	}
	rs := services.NewAdminRestService(svc)
	body := jsonBody(t, map[string]string{"status": "disabled"})
	req := withChiID(httptest.NewRequest(http.MethodPatch, "/admin/crawler-nodes/node-1", body), "node-1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rs.SetNodeStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("SetNodeStatus status = %d, want 200", rr.Code)
	}
}

// setNodeStatus: malformed JSON → 400.
func TestAdminRestService_SetNodeStatus_MalformedBody_Returns400(t *testing.T) {
	svc := &mockFullNodeService{}
	rs := services.NewAdminRestService(svc)
	req := withChiID(httptest.NewRequest(http.MethodPatch, "/admin/crawler-nodes/node-1", bytes.NewBufferString("{bad")), "node-1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rs.SetNodeStatus(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("SetNodeStatus (bad JSON) status = %d, want 400", rr.Code)
	}
}

// setNodeStatus: service error propagates.
func TestAdminRestService_SetNodeStatus_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		setStatusFn: func(_ context.Context, _, _ string) error {
			return domainerrs.ErrInternal.WithMessage("fail")
		},
	}
	rs := services.NewAdminRestService(svc)
	body := jsonBody(t, map[string]string{"status": "disabled"})
	req := withChiID(httptest.NewRequest(http.MethodPatch, "/admin/crawler-nodes/node-1", body), "node-1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	rs.SetNodeStatus(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("SetNodeStatus (error) status = %d, want 500", rr.Code)
	}
}

// retireNode: 204 on success.
func TestAdminRestService_RetireNode_Returns204(t *testing.T) {
	svc := &mockFullNodeService{
		retireFn: func(_ context.Context, _ string) error { return nil },
	}
	rs := services.NewAdminRestService(svc)
	req := withChiID(httptest.NewRequest(http.MethodDelete, "/admin/crawler-nodes/node-1", nil), "node-1")
	rr := httptest.NewRecorder()
	rs.RetireNode(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("RetireNode status = %d, want 204", rr.Code)
	}
}

// retireNode: service error propagates.
func TestAdminRestService_RetireNode_ServiceError_Returns500(t *testing.T) {
	svc := &mockFullNodeService{
		retireFn: func(_ context.Context, _ string) error {
			return domainerrs.ErrInternal.WithMessage("fail")
		},
	}
	rs := services.NewAdminRestService(svc)
	req := withChiID(httptest.NewRequest(http.MethodDelete, "/admin/crawler-nodes/node-1", nil), "node-1")
	rr := httptest.NewRecorder()
	rs.RetireNode(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("RetireNode (error) status = %d, want 500", rr.Code)
	}
}

// Compile-time interface checks.
var _ *services.NodeRestService = (*services.NodeRestService)(nil)
var _ *services.AdminRestService = (*services.AdminRestService)(nil)
