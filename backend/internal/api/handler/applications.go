package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
)

type ApplicationHandler struct {
	apps store.ApplicationStore
	jobs store.JobWriter
}

func NewApplicationHandler(apps store.ApplicationStore, jobs store.JobWriter) *ApplicationHandler {
	return &ApplicationHandler{apps: apps, jobs: jobs}
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
