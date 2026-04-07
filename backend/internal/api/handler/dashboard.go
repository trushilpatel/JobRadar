package handler

import (
	"net/http"
	"strconv"

	"github.com/trushilpatel/jobradar/internal/store"
)

type DashboardHandler struct {
	jobs     store.JobReader
	scanLog  store.JobScanLogReader
	insights store.InsightsReader
}

func NewDashboardHandler(jobs store.JobReader, scanLog store.JobScanLogReader, insights store.InsightsReader) *DashboardHandler {
	return &DashboardHandler{jobs: jobs, scanLog: scanLog, insights: insights}
}

// GET /dashboard/stats
func (h *DashboardHandler) Stats(w http.ResponseWriter, r *http.Request) {
	scannedToday, err := h.scanLog.GetJobsScannedToday()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch scanned today")
		return
	}

	newCount, savedCount, appliedCount, err := h.jobs.GetJobCounts()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch job counts")
		return
	}

	pipeline, err := h.insights.GetPipelineCounts()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch pipeline counts")
		return
	}

	relevant := newCount + savedCount + appliedCount

	respond(w, http.StatusOK, map[string]any{
		"jobs_scanned_today": scannedToday,
		"jobs_new":           newCount,
		"jobs_saved":         savedCount,
		"jobs_applied":       appliedCount,
		"pipeline_pending":   pipeline.Pending,
		"pipeline_errors":    pipeline.Errors,
		"funnel": map[string]any{
			"scanned":  scannedToday,
			"relevant": relevant,
			"saved":    savedCount,
			"applied":  appliedCount,
		},
	})
}

// GET /dashboard/activity?days=7
func (h *DashboardHandler) Activity(w http.ResponseWriter, r *http.Request) {
	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	activity, err := h.scanLog.GetScansByDay(days)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch activity")
		return
	}

	respond(w, http.StatusOK, activity)
}
