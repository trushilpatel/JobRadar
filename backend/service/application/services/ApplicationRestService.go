package services

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/apps/api/request"
	"github.com/trushilpatel/jobradar/service/application/dto"
)

// RestService handles HTTP requests for the application domain.
type RestService struct {
	next ApplicationService
}

// NewApplicationRestService constructs a RestService.
func NewApplicationRestService(next ApplicationService) *RestService {
	return &RestService{next: next}
}

// GET /jobs/:id/applications
func (h *RestService) list(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	apps, err := h.next.ListByJob(jobID)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, apps)
}

// POST /jobs/:id/applications
func (h *RestService) create(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	var input dto.CreateApplicationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
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

	apps, total, err := h.next.ListAll(p.Page, p.Limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.RespondPaged(w, http.StatusOK, apps, total, p.Page, p.Limit)
}

// PATCH /jobs/:id/applications/:app_id/stage
func (h *RestService) updateStage(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	appID := chi.URLParam(r, "app_id")

	var input dto.UpdateStageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.next.UpdateStage(jobID, appID, input); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"app_id": appID, "stage": input.Stage})
}

// PATCH /applications/:id/stage — top-level shortcut used by the dashboard interviewing tracker
func (h *RestService) updateStageByID(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")

	var input dto.UpdateStageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	// jobID unused by UpdateStage — pass empty string
	if err := h.next.UpdateStage("", appID, input); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"app_id": appID, "stage": input.Stage})
}

// PATCH /jobs/:id/applications/:app_id/note
func (h *RestService) updateNote(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	appID := chi.URLParam(r, "app_id")

	var input dto.UpdateNoteInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.next.UpdateNote(jobID, appID, input); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"app_id": appID, "note": input.Note})
}
