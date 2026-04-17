package scan

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- fakes ---

type fakeScanLogWriter struct {
	called bool
	err    error

	companyID   string
	jobsFetched int
	jobsNew     int
	durationMs  int
	errMsg      *string
}

func (f *fakeScanLogWriter) InsertCompanyScanLog(_ context.Context, companyID string, jobsFetched, jobsNew, durationMs int, errMsg *string) error {
	f.called = true
	f.companyID = companyID
	f.jobsFetched = jobsFetched
	f.jobsNew = jobsNew
	f.durationMs = durationMs
	f.errMsg = errMsg
	return f.err
}

type fakeCompanyScanUpdater struct {
	called bool
	err    error

	id     string
	status string
}

func (f *fakeCompanyScanUpdater) UpdateScanStatus(_ context.Context, id, status string) error {
	f.called = true
	f.id = id
	f.status = status
	return f.err
}

// --- tests ---

func TestRecordScanResult_HappyPath(t *testing.T) {
	logWriter := &fakeScanLogWriter{}
	companyUpdater := &fakeCompanyScanUpdater{}
	svc := NewCrawlerService(logWriter, companyUpdater)

	result := ScanResult{
		FetchedCount: 10,
		NewCount:     3,
		Duration:     500 * time.Millisecond,
		ErrMsg:       "",
	}

	err := svc.RecordScanResult(context.Background(), "company-1", result)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !logWriter.called {
		t.Error("expected InsertCompanyScanLog to be called")
	}
	if logWriter.companyID != "company-1" {
		t.Errorf("expected companyID=company-1, got %q", logWriter.companyID)
	}
	if logWriter.jobsFetched != 10 {
		t.Errorf("expected jobsFetched=10, got %d", logWriter.jobsFetched)
	}
	if logWriter.jobsNew != 3 {
		t.Errorf("expected jobsNew=3, got %d", logWriter.jobsNew)
	}
	if logWriter.durationMs != 500 {
		t.Errorf("expected durationMs=500, got %d", logWriter.durationMs)
	}
	if logWriter.errMsg != nil {
		t.Errorf("expected errMsg=nil, got %v", logWriter.errMsg)
	}

	if !companyUpdater.called {
		t.Error("expected UpdateScanStatus to be called")
	}
	if companyUpdater.id != "company-1" {
		t.Errorf("expected id=company-1, got %q", companyUpdater.id)
	}
	if companyUpdater.status != "ok" {
		t.Errorf("expected status=ok, got %q", companyUpdater.status)
	}
}

func TestRecordScanResult_LogWriteFailure(t *testing.T) {
	writeErr := errors.New("db write failed")
	logWriter := &fakeScanLogWriter{err: writeErr}
	companyUpdater := &fakeCompanyScanUpdater{}
	svc := NewCrawlerService(logWriter, companyUpdater)

	result := ScanResult{
		FetchedCount: 5,
		NewCount:     1,
		Duration:     200 * time.Millisecond,
		ErrMsg:       "",
	}

	err := svc.RecordScanResult(context.Background(), "company-2", result)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("expected writeErr, got %v", err)
	}

	// Company status must NOT be updated when log write fails.
	if companyUpdater.called {
		t.Error("expected UpdateScanStatus NOT to be called when log write fails")
	}
}

func TestRecordScanResult_CompanyUpdateFailure(t *testing.T) {
	updateErr := errors.New("company update failed")
	logWriter := &fakeScanLogWriter{}
	companyUpdater := &fakeCompanyScanUpdater{err: updateErr}
	svc := NewCrawlerService(logWriter, companyUpdater)

	result := ScanResult{
		FetchedCount: 7,
		NewCount:     2,
		Duration:     300 * time.Millisecond,
		ErrMsg:       "fetch failed: timeout",
	}

	err := svc.RecordScanResult(context.Background(), "company-3", result)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, updateErr) {
		t.Errorf("expected updateErr, got %v", err)
	}

	if !logWriter.called {
		t.Error("expected InsertCompanyScanLog to have been called")
	}
	// errMsg should be non-nil when ErrMsg is set
	if logWriter.errMsg == nil {
		t.Error("expected errMsg to be non-nil for error scan result")
	} else if *logWriter.errMsg != "fetch failed: timeout" {
		t.Errorf("expected errMsg=%q, got %q", "fetch failed: timeout", *logWriter.errMsg)
	}

	if !companyUpdater.called {
		t.Error("expected UpdateScanStatus to be called")
	}
	if companyUpdater.status != "error" {
		t.Errorf("expected status=error, got %q", companyUpdater.status)
	}
}
