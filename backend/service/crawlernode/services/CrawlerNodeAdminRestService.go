package services

import (
	"context"

	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
)

// AdminRestService is the HTTP adapter for admin-facing crawler-node endpoints.
// Depends only on NodeService; validation is handled by the decorator chain.
type AdminRestService struct {
	next NodeService
}

// NewAdminRestService constructs the admin-facing HTTP adapter.
func NewAdminRestService(next NodeService) *AdminRestService {
	return &AdminRestService{next: next}
}

// listNodesInput is the Huma input for GET /admin/crawler-nodes.
type listNodesInput struct {
	Status string `query:"status" doc:"Filter by node status"`
}

// listNodes serves GET /admin/crawler-nodes.
func (h *AdminRestService) listNodes(ctx context.Context, in *listNodesInput) (*apihuma.SingleOutput[[]dto.NodeView], error) {
	nodes, err := h.next.ListNodes(ctx, in.Status)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[[]dto.NodeView]{}
	out.Body.Data = nodes
	return out, nil
}

// provisionNodeRequest is the JSON body for POST /admin/crawler-nodes.
// required:"false" matches the pre-Huma handler's plain json.Decode — an
// empty name reaches ValidationService.ProvisionNode's existing
// non-empty check rather than failing at the framework layer.
type provisionNodeRequest struct {
	Name string `json:"name" required:"false"`
}

// provisionNodeInput is the Huma input for POST /admin/crawler-nodes.
type provisionNodeInput struct {
	Body provisionNodeRequest
}

// provisionNode serves POST /admin/crawler-nodes.
func (h *AdminRestService) provisionNode(ctx context.Context, in *provisionNodeInput) (*apihuma.SingleOutput[*dto.CreateNodeResponse], error) {
	resp, err := h.next.ProvisionNode(ctx, in.Body.Name)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[*dto.CreateNodeResponse]{}
	out.Body.Data = resp
	return out, nil
}

// setNodeStatusRequest is the JSON body for PATCH /admin/crawler-nodes/{id}.
// required:"false" matches the pre-Huma handler's plain json.Decode — an
// empty status reaches ValidationService.SetNodeStatus's existing
// allow-list check rather than failing at the framework layer.
type setNodeStatusRequest struct {
	Status string `json:"status" required:"false"`
}

// setNodeStatusInput is the Huma input for PATCH /admin/crawler-nodes/{id}.
type setNodeStatusInput struct {
	ID   string `path:"id"`
	Body setNodeStatusRequest
}

// setNodeStatus serves PATCH /admin/crawler-nodes/{id}.
func (h *AdminRestService) setNodeStatus(ctx context.Context, in *setNodeStatusInput) (*apihuma.SingleOutput[struct{}], error) {
	if err := h.next.SetNodeStatus(ctx, in.ID, in.Body.Status); err != nil {
		return nil, apihuma.MapError(err)
	}
	return &apihuma.SingleOutput[struct{}]{}, nil
}

// retireNodeInput is the Huma input for DELETE /admin/crawler-nodes/{id}.
type retireNodeInput struct {
	ID string `path:"id"`
}

// retireNodeOutput has no Body field, so Huma defaults its status to 204
// (see huma's own op.DefaultStatus inference: no output body -> StatusNoContent).
type retireNodeOutput struct{}

// retireNode serves DELETE /admin/crawler-nodes/{id}.
func (h *AdminRestService) retireNode(ctx context.Context, in *retireNodeInput) (*retireNodeOutput, error) {
	if err := h.next.RetireNode(ctx, in.ID); err != nil {
		return nil, apihuma.MapError(err)
	}
	return &retireNodeOutput{}, nil
}
