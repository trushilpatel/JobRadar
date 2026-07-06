package services

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/common/identity"
	applyqueuecommon "github.com/trushilpatel/jobradar/service/applyqueue/common"
	"github.com/trushilpatel/jobradar/service/applyqueue/dto"
)

// RestService is the HTTP adapter for the apply-queue Service chain.
type RestService struct {
	next Service
}

// NewRestService constructs a RestService.
func NewRestService(next Service) *RestService {
	if next == nil {
		panic("applyqueue.NewRestService: next must not be nil")
	}
	return &RestService{next: next}
}

// list serves GET /api/apply-queue?filter=all|saved&cursor=&limit=.
func (h *RestService) list(w http.ResponseWriter, r *http.Request) {
	userID := identity.MustFromContext(r.Context()).UserID
	q := r.URL.Query()

	filter := q.Get("filter")
	if filter == "" {
		filter = applyqueuecommon.FilterAll
	}

	limit := 20
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	result, err := h.next.List(r.Context(), userID, filter, q.Get("cursor"), limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, result)
}

// setResume serves PUT /api/apply-queue/{jobId}/resume. A null resume_id
// clears the attachment (204 No Content); a non-null resume_id attaches it
// (200 with the resulting AttachmentView).
func (h *RestService) setResume(w http.ResponseWriter, r *http.Request) {
	userID := identity.MustFromContext(r.Context()).UserID
	jobID := chi.URLParam(r, "jobId")

	input, ok := handler.DecodeBody[dto.SetResumeInput](w, r)
	if !ok {
		return
	}

	view, err := h.next.SetResume(r.Context(), userID, jobID, input.ResumeID)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	if view == nil {
		handler.RespondNoContent(w)
		return
	}
	handler.Respond(w, http.StatusOK, view)
}
