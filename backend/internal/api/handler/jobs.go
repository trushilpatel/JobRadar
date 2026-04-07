package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
)

type JobHandler struct {
	jobs     store.JobStore
	ledger   store.SeenLedgerWriter
	discard  store.DiscardStore
	snapshot store.SnapshotWriter
}

func NewJobHandler(jobs store.JobStore, ledger store.SeenLedgerWriter, discard store.DiscardStore, snapshot store.SnapshotWriter) *JobHandler {
	return &JobHandler{jobs: jobs, ledger: ledger, discard: discard, snapshot: snapshot}
}

// GET /jobs?status=new&saved=true
func (h *JobHandler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = model.JobStatusNew
	}

	jobs, err := h.jobs.GetJobsByStatus(status)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch jobs")
		return
	}

	respond(w, http.StatusOK, jobs)
}

// GET /jobs/:id
func (h *JobHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	job, err := h.jobs.GetJobByID(id)
	if err != nil {
		if store.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch job")
		return
	}

	respond(w, http.StatusOK, job)
}

// PATCH /jobs/:id/save
// Body: {"saved": true}
func (h *JobHandler) SetSaved(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Saved bool `json:"saved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.jobs.UpdateJobSaved(id, body.Saved); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update job")
		return
	}

	if body.Saved {
		// Non-fatal — snapshot may not exist for older jobs
		_ = h.snapshot.UpdateSnapshotAction(id, "saved")
	}

	respond(w, http.StatusOK, map[string]bool{"saved": body.Saved})
}

// PATCH /jobs/:id/status
// Body: {"status": "applied"}
func (h *JobHandler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	valid := map[string]bool{
		model.JobStatusNew:     true,
		model.JobStatusSaved:   true,
		model.JobStatusApplied: true,
	}
	if !valid[body.Status] {
		respondError(w, http.StatusBadRequest, "invalid status value")
		return
	}

	if err := h.jobs.UpdateJobStatus(id, body.Status); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update status")
		return
	}

	if body.Status == model.JobStatusApplied {
		_ = h.snapshot.UpdateSnapshotAction(id, "applied")
	}

	respond(w, http.StatusOK, map[string]string{"status": body.Status})
}

// PATCH /jobs/:id/notes
// Body: {"notes": "some text"}
func (h *JobHandler) SetNotes(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Notes string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.jobs.UpdateJobNotes(id, body.Notes); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update notes")
		return
	}

	respond(w, http.StatusOK, map[string]string{"notes": body.Notes})
}

// DELETE /jobs/:id
// Body: {"reason": "user_dismissed", "note": "optional"}
func (h *JobHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Reason string `json:"reason"`
		Note   string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	valid := map[string]bool{
		model.DiscardReasonDismissed:   true,
		model.DiscardReasonNotRelevant: true,
	}
	if !valid[body.Reason] {
		respondError(w, http.StatusBadRequest, "invalid reason value")
		return
	}

	// Get the job to find its job_post_id for seen_ledger
	job, err := h.jobs.GetJobByID(id)
	if err != nil {
		if store.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "job not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch job")
		return
	}

	// Insert into discarded table
	if err := h.discard.InsertDiscard(id, body.Reason, body.Note); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to discard job")
		return
	}

	// Block re-entry into pipeline via seen_ledger
	if err := h.ledger.UpdateSeenReason(job.JobPostID, model.SeenReasonUserDismissed); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update seen ledger")
		return
	}

	// Hide from feed without deleting (preserves insight/analytics data)
	if err := h.jobs.UpdateJobStatus(id, model.JobStatusDismissed); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update job status")
		return
	}

	_ = h.snapshot.UpdateSnapshotAction(id, "dismissed")

	respond(w, http.StatusOK, map[string]string{"id": id, "reason": body.Reason})
}
