package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/store"
)

type ApplicationHandler struct {
	apps store.ApplicationStore
	jobs store.JobWriter
}

func NewApplicationHandler(apps store.ApplicationStore, jobs store.JobWriter) *ApplicationHandler {
	return &ApplicationHandler{apps: apps, jobs: jobs}
}

// Register mounts all application routes onto r.
// Applications span two prefixes — both are owned here.
func (h *ApplicationHandler) Register(r chi.Router) {
	r.Get("/jobs/{id}/applications", h.List)
	r.Post("/jobs/{id}/applications", h.Create)
	r.Patch("/jobs/{id}/applications/{app_id}/stage", h.UpdateStage)
	r.Patch("/jobs/{id}/applications/{app_id}/note", h.UpdateNote)
	r.Get("/applications", h.ListAll)
}

// GET /jobs/:id/applications
func (h *ApplicationHandler) List(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	applications, err := h.apps.GetApplicationsByJobID(jobID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch applications")
		return
	}

	respond(w, http.StatusOK, applications)
}

// POST /jobs/:id/applications
// Body: {"stage": "phone_screen", "note": "optional"}
func (h *ApplicationHandler) Create(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	var body struct {
		Stage string `json:"stage"`
		Note  string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	valid := map[string]bool{
		model.StageApplied:     true,
		model.StagePhoneScreen: true,
		model.StageTechnical:   true,
		model.StageFinalRound:  true,
		model.StageOffer:       true,
		model.StageRejected:    true,
	}
	if !valid[body.Stage] {
		respondError(w, http.StatusBadRequest, "invalid stage value")
		return
	}

	if err := h.apps.InsertApplication(jobID, body.Stage, body.Note); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create application stage")
		return
	}

	// When the first stage is added, mark the job as applied
	if body.Stage == model.StageApplied {
		if err := h.jobs.UpdateJobStatus(jobID, model.JobStatusApplied); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to update job status")
			return
		}
	}

	respond(w, http.StatusCreated, map[string]string{"job_id": jobID, "stage": body.Stage})
}

// GET /applications
func (h *ApplicationHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	applications, err := h.apps.GetLatestApplicationsWithJob()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch applications")
		return
	}
	respond(w, http.StatusOK, applications)
}

// PATCH /jobs/:id/applications/:app_id/stage
// Body: {"stage": "phone_screen"}
func (h *ApplicationHandler) UpdateStage(w http.ResponseWriter, r *http.Request) {
	appIDStr := chi.URLParam(r, "app_id")
	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid app_id")
		return
	}

	var body struct {
		Stage string `json:"stage"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	valid := map[string]bool{
		model.StageApplied:     true,
		model.StagePhoneScreen: true,
		model.StageTechnical:   true,
		model.StageFinalRound:  true,
		model.StageOffer:       true,
		model.StageRejected:    true,
	}
	if !valid[body.Stage] {
		respondError(w, http.StatusBadRequest, "invalid stage value")
		return
	}

	if err := h.apps.UpdateApplicationStage(appID, body.Stage); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update stage")
		return
	}

	respond(w, http.StatusOK, map[string]any{"id": appID, "stage": body.Stage})
}

// PATCH /jobs/:id/applications/:app_id/note
// Body: {"note": "Had great call"}
func (h *ApplicationHandler) UpdateNote(w http.ResponseWriter, r *http.Request) {
	appIDStr := chi.URLParam(r, "app_id")
	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid app_id")
		return
	}

	var body struct {
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.apps.UpdateApplicationNote(appID, body.Note); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update note")
		return
	}

	respond(w, http.StatusOK, map[string]any{"id": appID, "note": body.Note})
}
