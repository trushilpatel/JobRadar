package services

import (
	"context"
	"fmt"
	"slices"

	"github.com/trushilpatel/jobradar/common/errors"
	crawlernodecommon "github.com/trushilpatel/jobradar/service/crawlernode/common"
	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
)

type validationService struct {
	BaseNodeService
}

// NewValidationService returns a NodeService decorator that validates inputs
// before delegating to next.
func NewValidationService(next NodeService) NodeService {
	return &validationService{BaseNodeService: NewBaseNodeService(next)}
}

func (s *validationService) GetWork(ctx context.Context, nodeID, adapterType string, holdingIDs []string) (*dto.WorkResponse, error) {
	if nodeID == "" {
		return nil, errors.ErrBadRequest.WithMessage("GetWork: nodeID is required")
	}
	if adapterType == "" {
		return nil, errors.ErrBadRequest.WithMessage("GetWork: adapterType is required")
	}
	return s.next.GetWork(ctx, nodeID, adapterType, holdingIDs)
}

func (s *validationService) SubmitResults(ctx context.Context, nodeID string, req dto.ResultsRequest) (*dto.ResultsResponse, error) {
	if nodeID == "" {
		return nil, errors.ErrBadRequest.WithMessage("SubmitResults: nodeID is required")
	}
	if req.CompanyID == "" {
		return nil, errors.ErrBadRequest.WithMessage("SubmitResults: companyID is required")
	}
	if req.AdapterType == "" {
		return nil, errors.ErrBadRequest.WithMessage("SubmitResults: adapterType is required")
	}
	if len(req.Jobs) > crawlernodecommon.MaxJobsPerBatch {
		return nil, errors.ErrBadRequest.WithMessage(fmt.Sprintf(
			"SubmitResults: jobs count %d exceeds maximum %d", len(req.Jobs), crawlernodecommon.MaxJobsPerBatch,
		))
	}
	return s.next.SubmitResults(ctx, nodeID, req)
}

func (s *validationService) ReportError(ctx context.Context, nodeID string, req dto.ErrorRequest) error {
	if nodeID == "" {
		return errors.ErrBadRequest.WithMessage("ReportError: nodeID is required")
	}
	if req.AssignmentID == "" {
		return errors.ErrBadRequest.WithMessage("ReportError: assignmentID is required")
	}
	return s.next.ReportError(ctx, nodeID, req)
}

func (s *validationService) ProvisionNode(ctx context.Context, name string) (*dto.CreateNodeResponse, error) {
	if name == "" {
		return nil, errors.ErrBadRequest.WithMessage("ProvisionNode: name is required")
	}
	if len(name) > crawlernodecommon.MaxNodeNameLen {
		return nil, errors.ErrBadRequest.WithMessage(fmt.Sprintf(
			"ProvisionNode: name length %d exceeds maximum %d", len(name), crawlernodecommon.MaxNodeNameLen,
		))
	}
	return s.next.ProvisionNode(ctx, name)
}

func (s *validationService) ListNodes(ctx context.Context, status string) ([]dto.NodeView, error) {
	if status != "" && !slices.Contains(crawlernodecommon.ValidNodeStatuses, status) {
		return nil, errors.ErrBadRequest.WithMessage(fmt.Sprintf(
			"ListNodes: invalid status %q; valid values: %v", status, crawlernodecommon.ValidNodeStatuses,
		))
	}
	return s.next.ListNodes(ctx, status)
}

func (s *validationService) SetNodeStatus(ctx context.Context, nodeID, status string) error {
	if nodeID == "" {
		return errors.ErrBadRequest.WithMessage("SetNodeStatus: nodeID is required")
	}
	if !slices.Contains(crawlernodecommon.ValidNodeStatuses, status) {
		return errors.ErrBadRequest.WithMessage(fmt.Sprintf(
			"SetNodeStatus: invalid status %q; valid values: %v", status, crawlernodecommon.ValidNodeStatuses,
		))
	}
	return s.next.SetNodeStatus(ctx, nodeID, status)
}

func (s *validationService) RetireNode(ctx context.Context, nodeID string) error {
	if nodeID == "" {
		return errors.ErrBadRequest.WithMessage("RetireNode: nodeID is required")
	}
	return s.next.RetireNode(ctx, nodeID)
}
