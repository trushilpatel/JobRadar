package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trushilpatel/jobradar/internal/model"
)

func TestDashboardStats(t *testing.T) {
	js := newFakeJobStore(
		jobWithID("1", model.JobStatusNew),
		jobWithID("2", model.JobStatusSaved),
		jobWithID("3", model.JobStatusApplied),
	)
	scanLog := &fakeScanLogReader{scannedToday: 42}
	insights := &fakeInsightsReader{}
	h := NewDashboardHandler(js, scanLog, insights)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/stats", nil)
	w := httptest.NewRecorder()
	h.Stats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}

	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)

	if body["jobs_scanned_today"].(float64) != 42 {
		t.Errorf("expected jobs_scanned_today=42, got %v", body["jobs_scanned_today"])
	}
	if body["jobs_new"].(float64) != 1 {
		t.Errorf("expected jobs_new=1, got %v", body["jobs_new"])
	}
	if body["pipeline_pending"].(float64) != 3 {
		t.Errorf("expected pipeline_pending=3, got %v", body["pipeline_pending"])
	}
	if body["pipeline_errors"].(float64) != 1 {
		t.Errorf("expected pipeline_errors=1, got %v", body["pipeline_errors"])
	}
}

func TestDashboardActivity_DefaultDays(t *testing.T) {
	h := NewDashboardHandler(newFakeJobStore(), &fakeScanLogReader{}, &fakeInsightsReader{})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity", nil)
	w := httptest.NewRecorder()
	h.Activity(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
}

func TestDashboardActivity_CustomDays(t *testing.T) {
	h := NewDashboardHandler(newFakeJobStore(), &fakeScanLogReader{}, &fakeInsightsReader{})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity?days=30", nil)
	w := httptest.NewRecorder()
	h.Activity(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
}
