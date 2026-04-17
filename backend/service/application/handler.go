package application

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/api/handler"
	"github.com/trushilpatel/jobradar/api/request"
)

// Handler handles HTTP requests for the application domain.
type Handler struct {
	validator    Validator
	applications Service
}

// NewHandler constructs a Handler.
func NewHandler(v Validator, svc Service) *Handler {
	return &Handler{validator: v, applications: svc}
}

// Register mounts all application routes onto r.
func (h *Handler) Register(r chi.Router) {
	r.Get("/jobs/{id}/applications", h.List)
	r.Post("/jobs/{id}/applications", h.Create)
	r.Patch("/jobs/{id}/applications/{app_id}/stage", h.UpdateStage)
	r.Patch("/jobs/{id}/applications/{app_id}/note", h.UpdateNote)
	r.Get("/applications", h.ListAll)
	r.Patch("/applications/{id}/stage", h.UpdateStageByID)
}

// GET /jobs/:id/applications
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	apps, err := h.applications.ListByJob(jobID)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, apps)
}

// POST /jobs/:id/applications
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	var input CreateApplicationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.validator.ValidateCreate(input); err != nil {
		handler.HandleError(w, err)
		return
	}

	app, err := h.applications.Create(jobID, input)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusCreated, app)
}

// GET /applications
func (h *Handler) ListAll(w http.ResponseWriter, r *http.Request) {
	p := request.ParsePagination(r)

	if err := h.validator.ValidateListAll(p.Page, p.Limit); err != nil {
		handler.HandleError(w, err)
		return
	}

	apps, total, err := h.applications.ListAll(p.Page, p.Limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.RespondPaged(w, http.StatusOK, apps, total, p.Page, p.Limit)
}

// PATCH /jobs/:id/applications/:app_id/stage
func (h *Handler) UpdateStage(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	appID := chi.URLParam(r, "app_id")

	var input UpdateStageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.validator.ValidateUpdateStage(input); err != nil {
		handler.HandleError(w, err)
		return
	}

	if err := h.applications.UpdateStage(jobID, appID, input); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"app_id": appID, "stage": input.Stage})
}

// PATCH /applications/:id/stage — top-level shortcut used by the dashboard interviewing tracker
func (h *Handler) UpdateStageByID(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")

	var input UpdateStageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.validator.ValidateUpdateStage(input); err != nil {
		handler.HandleError(w, err)
		return
	}

	// jobID unused by UpdateStage — pass empty string
	if err := h.applications.UpdateStage("", appID, input); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"app_id": appID, "stage": input.Stage})
}

// PATCH /jobs/:id/applications/:app_id/note
func (h *Handler) UpdateNote(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	appID := chi.URLParam(r, "app_id")

	var input UpdateNoteInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.validator.ValidateUpdateNote(input); err != nil {
		handler.HandleError(w, err)
		return
	}

	if err := h.applications.UpdateNote(jobID, appID, input); err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]string{"app_id": appID, "note": input.Note})
}
