package services

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/api/request"
	"github.com/trushilpatel/jobradar/service/application/dto"
)

// RestService handles HTTP requests for the application domain. It owns
// no business logic — it parses the request, calls next, and writes the
// response via handler.Respond / handler.RespondPaged / handler.HandleError.
type RestService struct {
	next ApplicationService
}

// NewRestService constructs a RestService.
func NewRestService(next ApplicationService) *RestService {
	return &RestService{next: next}
}

// GET /jobs/{id}/applications
func (h *RestService) list(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	apps, err := h.next.ListByJob(r.Context(), jobID)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, apps)
}

// POST /jobs/{id}/applications
func (h *RestService) create(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	input, ok := handler.DecodeBody[dto.CreateApplicationInput](w, r)
	if !ok {
		return
	}

	app, err := h.next.Create(r.Context(), jobID, input)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusCreated, app)
}

// GET /applications
func (h *RestService) listAll(w http.ResponseWriter, r *http.Request) {
	p := request.ParsePagination(r)

	apps, total, err := h.next.ListAll(r.Context(), p.Page, p.Limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.RespondPaged(w, http.StatusOK, apps, total, p.Page, p.Limit)
}

// PATCH /jobs/{id}/applications/{app_id}/stage
func (h *RestService) updateStage(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	appID := chi.URLParam(r, "app_id")

	input, ok := handler.DecodeBody[dto.UpdateStageInput](w, r)
	if !ok {
		return
	}

	if err := h.next.UpdateStage(r.Context(), jobID, appID, input); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"app_id": appID, "stage": input.Stage})
}

// PATCH /applications/{id}/stage — top-level shortcut used by the dashboard interview tracker.
func (h *RestService) updateStageByID(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")

	input, ok := handler.DecodeBody[dto.UpdateStageInput](w, r)
	if !ok {
		return
	}

	if err := h.next.UpdateStageByID(r.Context(), appID, input); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"app_id": appID, "stage": input.Stage})
}

// PATCH /jobs/{id}/applications/{app_id}/note
func (h *RestService) updateNote(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	appID := chi.URLParam(r, "app_id")

	input, ok := handler.DecodeBody[dto.UpdateNoteInput](w, r)
	if !ok {
		return
	}

	if err := h.next.UpdateNote(r.Context(), jobID, appID, input); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"app_id": appID, "note": input.Note})
}
