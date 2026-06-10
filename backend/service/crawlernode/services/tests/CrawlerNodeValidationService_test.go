package tests

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	crawlernodecommon "github.com/trushilpatel/jobradar/service/crawlernode/common"
	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
)

// newValidationSvc wires ValidationService → mockFullNodeService with no-op inner methods.
func newValidationSvc() services.NodeService {
	inner := &mockFullNodeService{
		getWorkFn: func(_ context.Context, _, _ string, _ []string) (*dto.WorkResponse, error) {
			return &dto.WorkResponse{}, nil
		},
		submitResultsFn: func(_ context.Context, _ string, _ dto.ResultsRequest) (*dto.ResultsResponse, error) {
			return &dto.ResultsResponse{}, nil
		},
		reportErrorFn: func(_ context.Context, _ string, _ dto.ErrorRequest) error { return nil },
		provisionFn: func(_ context.Context, _ string) (*dto.CreateNodeResponse, error) {
			return &dto.CreateNodeResponse{}, nil
		},
		listNodesFn: func(_ context.Context, _ string) ([]dto.NodeView, error) { return nil, nil },
		setStatusFn: func(_ context.Context, _, _ string) error { return nil },
		retireFn:    func(_ context.Context, _ string) error { return nil },
	}
	return services.NewValidationService(inner)
}

// --- GetWork validation ---

func TestValidationService_GetWork_EmptyNodeIDReturnsBadRequest(t *testing.T) {
	svc := newValidationSvc()
	_, err := svc.GetWork(context.Background(), "", "greenhouse", nil)
	assertBadRequest(t, "GetWork(nodeID='')", err)
}

// Empty adapterType is now valid — the orchestrator is adapter-agnostic and claims any queued item.
func TestValidationService_GetWork_EmptyAdapterTypePassesThrough(t *testing.T) {
	svc := newValidationSvc()
	_, err := svc.GetWork(context.Background(), "node-1", "", nil)
	if err != nil {
		t.Errorf("GetWork(adapterType=''): expected nil (adapter is optional), got %v", err)
	}
}

func TestValidationService_GetWork_ValidPassesThrough(t *testing.T) {
	svc := newValidationSvc()
	_, err := svc.GetWork(context.Background(), "node-1", "greenhouse", nil)
	if err != nil {
		t.Fatalf("GetWork() valid input unexpected error: %v", err)
	}
}

// --- SubmitResults validation ---

func TestValidationService_SubmitResults_EmptyNodeIDReturnsBadRequest(t *testing.T) {
	svc := newValidationSvc()
	_, err := svc.SubmitResults(context.Background(), "", dto.ResultsRequest{
		CompanyID: "co-1", AdapterType: "greenhouse",
	})
	assertBadRequest(t, "SubmitResults(nodeID='')", err)
}

func TestValidationService_SubmitResults_EmptyCompanyIDReturnsBadRequest(t *testing.T) {
	svc := newValidationSvc()
	_, err := svc.SubmitResults(context.Background(), "node-1", dto.ResultsRequest{
		CompanyID: "", AdapterType: "greenhouse",
	})
	assertBadRequest(t, "SubmitResults(companyID='')", err)
}

func TestValidationService_SubmitResults_EmptyAdapterTypeReturnsBadRequest(t *testing.T) {
	svc := newValidationSvc()
	_, err := svc.SubmitResults(context.Background(), "node-1", dto.ResultsRequest{
		CompanyID: "co-1", AdapterType: "",
	})
	assertBadRequest(t, "SubmitResults(adapterType='')", err)
}

func TestValidationService_SubmitResults_TooManyJobsReturnsBadRequest(t *testing.T) {
	svc := newValidationSvc()
	jobs := make([]dto.CrawledJob, crawlernodecommon.MaxJobsPerBatch+1)
	_, err := svc.SubmitResults(context.Background(), "node-1", dto.ResultsRequest{
		CompanyID: "co-1", AdapterType: "greenhouse", Jobs: jobs,
	})
	assertBadRequest(t, "SubmitResults(jobs>MaxJobsPerBatch)", err)
}

func TestValidationService_SubmitResults_ValidPassesThrough(t *testing.T) {
	svc := newValidationSvc()
	_, err := svc.SubmitResults(context.Background(), "node-1", dto.ResultsRequest{
		CompanyID: "co-1", AdapterType: "greenhouse",
	})
	if err != nil {
		t.Fatalf("SubmitResults() valid input unexpected error: %v", err)
	}
}

// --- ReportError validation ---

func TestValidationService_ReportError_EmptyNodeIDReturnsBadRequest(t *testing.T) {
	svc := newValidationSvc()
	err := svc.ReportError(context.Background(), "", dto.ErrorRequest{AssignmentID: "q-1"})
	assertBadRequest(t, "ReportError(nodeID='')", err)
}

func TestValidationService_ReportError_EmptyAssignmentIDReturnsBadRequest(t *testing.T) {
	svc := newValidationSvc()
	err := svc.ReportError(context.Background(), "node-1", dto.ErrorRequest{AssignmentID: ""})
	assertBadRequest(t, "ReportError(assignmentID='')", err)
}

func TestValidationService_ReportError_ValidPassesThrough(t *testing.T) {
	svc := newValidationSvc()
	err := svc.ReportError(context.Background(), "node-1", dto.ErrorRequest{AssignmentID: "q-1"})
	if err != nil {
		t.Fatalf("ReportError() valid input unexpected error: %v", err)
	}
}

// --- ProvisionNode validation ---

func TestValidationService_ProvisionNode_EmptyNameReturnsBadRequest(t *testing.T) {
	svc := newValidationSvc()
	_, err := svc.ProvisionNode(context.Background(), "")
	assertBadRequest(t, "ProvisionNode(name='')", err)
}

func TestValidationService_ProvisionNode_NameTooLongReturnsBadRequest(t *testing.T) {
	svc := newValidationSvc()
	long := strings.Repeat("a", crawlernodecommon.MaxNodeNameLen+1)
	_, err := svc.ProvisionNode(context.Background(), long)
	assertBadRequest(t, "ProvisionNode(name too long)", err)
}

func TestValidationService_ProvisionNode_ValidPassesThrough(t *testing.T) {
	svc := newValidationSvc()
	_, err := svc.ProvisionNode(context.Background(), "my-node")
	if err != nil {
		t.Fatalf("ProvisionNode() valid input unexpected error: %v", err)
	}
}

// --- ListNodes validation ---

func TestValidationService_ListNodes_EmptyStatusPassesThrough(t *testing.T) {
	svc := newValidationSvc()
	_, err := svc.ListNodes(context.Background(), "")
	if err != nil {
		t.Fatalf("ListNodes('') unexpected error: %v", err)
	}
}

func TestValidationService_ListNodes_ValidStatusPassesThrough(t *testing.T) {
	svc := newValidationSvc()
	for _, s := range crawlernodecommon.ValidNodeStatuses {
		if err := func(status string) error {
			_, err := svc.ListNodes(context.Background(), status)
			return err
		}(s); err != nil {
			t.Fatalf("ListNodes(%q) unexpected error: %v", s, err)
		}
	}
}

func TestValidationService_ListNodes_InvalidStatusReturnsBadRequest(t *testing.T) {
	svc := newValidationSvc()
	_, err := svc.ListNodes(context.Background(), "unknown-status")
	assertBadRequest(t, "ListNodes(invalid status)", err)
}

// --- SetNodeStatus validation ---

func TestValidationService_SetNodeStatus_EmptyNodeIDReturnsBadRequest(t *testing.T) {
	svc := newValidationSvc()
	err := svc.SetNodeStatus(context.Background(), "", crawlernodecommon.NodeStatusActive)
	assertBadRequest(t, "SetNodeStatus(nodeID='')", err)
}

func TestValidationService_SetNodeStatus_InvalidStatusReturnsBadRequest(t *testing.T) {
	svc := newValidationSvc()
	err := svc.SetNodeStatus(context.Background(), "node-1", "bogus")
	assertBadRequest(t, "SetNodeStatus(status=bogus)", err)
}

func TestValidationService_SetNodeStatus_ValidPassesThrough(t *testing.T) {
	svc := newValidationSvc()
	for _, s := range crawlernodecommon.ValidNodeStatuses {
		if err := svc.SetNodeStatus(context.Background(), "node-1", s); err != nil {
			t.Fatalf("SetNodeStatus(%q) unexpected error: %v", s, err)
		}
	}
}

// --- RetireNode validation ---

func TestValidationService_RetireNode_EmptyNodeIDReturnsBadRequest(t *testing.T) {
	svc := newValidationSvc()
	err := svc.RetireNode(context.Background(), "")
	assertBadRequest(t, "RetireNode(nodeID='')", err)
}

func TestValidationService_RetireNode_ValidPassesThrough(t *testing.T) {
	svc := newValidationSvc()
	err := svc.RetireNode(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("RetireNode() valid input unexpected error: %v", err)
	}
}

// --- helpers ---

func assertBadRequest(t *testing.T, label string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected ErrBadRequest, got nil", label)
	}
	if !isBadRequest(err) {
		t.Errorf("%s: expected ErrBadRequest, got %v", label, err)
	}
}

func isBadRequest(err error) bool {
	var svcErr *domainerrs.ServiceError
	if stderrors.As(err, &svcErr) {
		return svcErr.Status == 400
	}
	return false
}
