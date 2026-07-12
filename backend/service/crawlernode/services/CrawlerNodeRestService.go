package services

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/apps/orchestrator/nodeidentity"
	"github.com/trushilpatel/jobradar/service/crawlernode/dto"
)

// NodeRestService is the HTTP adapter for node-facing endpoints.
// Depends only on NodeService; validation is handled by the decorator chain.
type NodeRestService struct {
	next NodeService
}

// NewNodeRestService constructs the node-facing HTTP adapter.
func NewNodeRestService(next NodeService) *NodeRestService {
	return &NodeRestService{next: next}
}

// getWorkInput is the Huma input for GET /node/work. Adapter is optional —
// when omitted the orchestrator claims any available adapter type. Holding
// is a raw comma-separated string, split in Resolve to match the pre-Huma
// handler's strings.Split(raw, ",") exactly.
type getWorkInput struct {
	Adapter string `query:"adapter" doc:"Adapter type filter; omit to claim any available type"`
	Holding string `query:"holding" doc:"Comma-separated lease IDs the node currently holds"`

	holdingIDs []string
}

func (in *getWorkInput) Resolve(_ huma.Context) []error {
	if in.Holding != "" {
		in.holdingIDs = strings.Split(in.Holding, ",")
	}
	return nil
}

// getWork serves GET /node/work.
func (h *NodeRestService) getWork(ctx context.Context, in *getWorkInput) (*apihuma.SingleOutput[*dto.WorkResponse], error) {
	nodeID := nodeidentity.IDFrom(ctx)
	resp, err := h.next.GetWork(ctx, nodeID, in.Adapter, in.holdingIDs)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[*dto.WorkResponse]{}
	out.Body.Data = resp
	return out, nil
}

// submitResultsInput is the Huma input for POST /node/results.
type submitResultsInput struct {
	Body dto.ResultsRequest
}

// submitResults serves POST /node/results.
func (h *NodeRestService) submitResults(ctx context.Context, in *submitResultsInput) (*apihuma.SingleOutput[*dto.ResultsResponse], error) {
	nodeID := nodeidentity.IDFrom(ctx)
	resp, err := h.next.SubmitResults(ctx, nodeID, in.Body)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[*dto.ResultsResponse]{}
	out.Body.Data = resp
	return out, nil
}

// reportErrorInput is the Huma input for POST /node/error.
type reportErrorInput struct {
	Body dto.ErrorRequest
}

// reportErrorOutput has no Body field, so Huma defaults its status to 204
// (see huma's own op.DefaultStatus inference: no output body -> StatusNoContent).
type reportErrorOutput struct{}

// reportError serves POST /node/error.
func (h *NodeRestService) reportError(ctx context.Context, in *reportErrorInput) (*reportErrorOutput, error) {
	nodeID := nodeidentity.IDFrom(ctx)
	if err := h.next.ReportError(ctx, nodeID, in.Body); err != nil {
		return nil, apihuma.MapError(err)
	}
	return &reportErrorOutput{}, nil
}
