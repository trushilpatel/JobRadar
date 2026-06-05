package services

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
)

// AtsRestService is the HTTP adapter for the ats domain. It owns request
// decoding and response shaping for the three /ats endpoints; routing is
// the responsibility of AtsRestRouter.
//
// AtsRestService does NOT implement AtsService — it consumes one. Per the
// slice convention, the next field is the only dependency it holds.
type AtsRestService struct {
	next AtsService
}

// NewAtsRestService constructs the HTTP adapter.
func NewAtsRestService(next AtsService) *AtsRestService {
	if next == nil {
		panic("ats.NewAtsRestService: next must not be nil")
	}
	return &AtsRestService{next: next}
}

// ListATS handles GET /ats — returns all ATS rows.
func (h *AtsRestService) ListATS(w http.ResponseWriter, r *http.Request) {
	items, err := h.next.ListATS(r.Context())
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, items)
}

// GetATS handles GET /ats/{name} — returns a single ATS row by name.
func (h *AtsRestService) GetATS(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	item, err := h.next.GetATS(r.Context(), name)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, item)
}

// UpdateATS handles PATCH /ats/{name} — applies a partial update to an ATS row.
func (h *AtsRestService) UpdateATS(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var input atsdto.UpdateATSInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	item, err := h.next.UpdateATS(r.Context(), name, input)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, item)
}
