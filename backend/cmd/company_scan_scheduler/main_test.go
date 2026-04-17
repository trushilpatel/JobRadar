package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/model"
	companysvc "github.com/trushilpatel/jobradar/service/company"
	scansvc "github.com/trushilpatel/jobradar/service/scan"
)

// ── fakes ─────────────────────────────────────────────────────────────────────

type fakeCompanyReader struct {
	due []model.Company
	err error
}

func (f *fakeCompanyReader) GetCompaniesDueForScan(_ int) ([]model.Company, error) {
	return f.due, f.err
}
func (f *fakeCompanyReader) GetActiveCompanies() ([]model.Company, error) { return nil, nil }
func (f *fakeCompanyReader) GetAllCompanies() ([]model.Company, error)    { return nil, nil }
func (f *fakeCompanyReader) GetCompanyByID(_ string) (model.Company, error) {
	return model.Company{}, nil
}
func (f *fakeCompanyReader) GetCompanies(_ companysvc.CompanyFilter, _, _ int) ([]model.Company, int64, error) {
	return nil, 0, nil
}
func (f *fakeCompanyReader) GetCompanyCounts() (total, active int64, err error) { return 0, 0, nil }

type fakeScanQueueStore struct {
	enqueued []string // company IDs enqueued
	err      error
}

func (f *fakeScanQueueStore) Enqueue(companyID, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.enqueued = append(f.enqueued, companyID)
	return nil
}
func (f *fakeScanQueueStore) ClaimNext(_ context.Context) (*scansvc.ScanQueueItem, error) {
	return nil, f.err
}
func (f *fakeScanQueueStore) Complete(_ string) error                     { return f.err }
func (f *fakeScanQueueStore) ResetStuck(_ time.Duration) error            { return f.err }
func (f *fakeScanQueueStore) ListQueue() ([]scansvc.ScanQueueItem, error) { return nil, f.err }
func (f *fakeScanQueueStore) MoveDown(_ string) error                     { return f.err }
func (f *fakeScanQueueStore) MoveUp(_ string) error                       { return f.err }
func (f *fakeScanQueueStore) Remove(_ string) error                       { return f.err }
func (f *fakeScanQueueStore) UpdateHeartbeat(_ string) error              { return f.err }

// ── helpers ───────────────────────────────────────────────────────────────────

func company(id, name string) model.Company {
	return model.Company{ID: id, Name: name}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestEnqueueDue_NoCompaniesDue(t *testing.T) {
	reader := &fakeCompanyReader{due: []model.Company{}}
	q := &fakeScanQueueStore{}

	enqueuedue(context.Background(), logger.NewNoop(), reader, q, 1440)

	if len(q.enqueued) != 0 {
		t.Errorf("expected no enqueues, got %d", len(q.enqueued))
	}
}

func TestEnqueueDue_EnqueuesAllDueCompanies(t *testing.T) {
	reader := &fakeCompanyReader{due: []model.Company{
		company("company-1", "Acme"),
		company("company-2", "Globex"),
		company("company-3", "Initech"),
	}}
	q := &fakeScanQueueStore{}

	enqueuedue(context.Background(), logger.NewNoop(), reader, q, 1440)

	if len(q.enqueued) != 3 {
		t.Fatalf("expected 3 enqueues, got %d", len(q.enqueued))
	}
	want := map[string]bool{"company-1": true, "company-2": true, "company-3": true}
	for _, id := range q.enqueued {
		if !want[id] {
			t.Errorf("unexpected company enqueued: %q", id)
		}
	}
}

func TestEnqueueDue_StoreError_EnqueuesNothing(t *testing.T) {
	reader := &fakeCompanyReader{err: errors.New("db down")}
	q := &fakeScanQueueStore{}

	enqueuedue(context.Background(), logger.NewNoop(), reader, q, 1440)

	if len(q.enqueued) != 0 {
		t.Errorf("expected no enqueues on store error, got %d", len(q.enqueued))
	}
}

func TestEnqueueDue_EnqueueError_ContinuesToNext(t *testing.T) {
	reader := &fakeCompanyReader{due: []model.Company{
		company("company-1", "Acme"),
		company("company-2", "Globex"),
	}}
	q := &fakeScanQueueStore{err: errors.New("queue full")}

	// Should not panic — logs and continues
	enqueuedue(context.Background(), logger.NewNoop(), reader, q, 1440)

	if len(q.enqueued) != 0 {
		t.Errorf("expected no successful enqueues, got %d", len(q.enqueued))
	}
}
