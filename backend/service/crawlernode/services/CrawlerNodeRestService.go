package services

import (
	"net/http"
	"strings"

	"github.com/trushilpatel/jobradar/apps/api/handler"
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

// GetWork handles GET /node/work?adapter=<type>&holding=<id,id,...>
// adapter is optional — when omitted the orchestrator claims any available adapter type.
func (h *NodeRestService) GetWork(w http.ResponseWriter, r *http.Request) {
	adapterType := r.URL.Query().Get("adapter")

	nodeID := nodeidentity.IDFrom(r.Context())

	var holdingIDs []string
	if raw := r.URL.Query().Get("holding"); raw != "" {
		holdingIDs = strings.Split(raw, ",")
	}

	resp, err := h.next.GetWork(r.Context(), nodeID, adapterType, holdingIDs)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, resp)
}

// SubmitResults handles POST /node/results
func (h *NodeRestService) SubmitResults(w http.ResponseWriter, r *http.Request) {
	req, ok := handler.DecodeBody[dto.ResultsRequest](w, r)
	if !ok {
		return
	}

	nodeID := nodeidentity.IDFrom(r.Context())
	resp, err := h.next.SubmitResults(r.Context(), nodeID, req)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, resp)
}

// ReportError handles POST /node/error
func (h *NodeRestService) ReportError(w http.ResponseWriter, r *http.Request) {
	req, ok := handler.DecodeBody[dto.ErrorRequest](w, r)
	if !ok {
		return
	}

	nodeID := nodeidentity.IDFrom(r.Context())
	if err := h.next.ReportError(r.Context(), nodeID, req); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.RespondNoContent(w)
}
