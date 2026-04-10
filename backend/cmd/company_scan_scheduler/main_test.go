package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/internal/logger"
	"github.com/trushilpatel/jobradar/internal/model"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeCompanyReader struct {
	due []model.Company
	err error
}

func (f *fakeCompanyReader) GetCompaniesDueForScan(_ int) ([]model.Company, error) {
	return f.due, f.err
}
func (f *fakeCompanyReader) GetActiveCompanies() ([]model.Company, error)        { return nil, nil }
func (f *fakeCompanyReader) GetAllCompanies() ([]model.Company, error)           { return nil, nil }
func (f *fakeCompanyReader) GetCompanyByID(_ string) (model.Company, error)      { return model.Company{}, nil }

type fakePublisher struct {
	payloads [][]byte
	err      error
}

func (f *fakePublisher) Publish(_ context.Context, _ string, value []byte) error {
	if f.err != nil {
		return f.err
	}
	f.payloads = append(f.payloads, value)
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func company(id, name string) model.Company {
	return model.Company{ID: id, Name: name}
}

func publishedIDs(t *testing.T, pub *fakePublisher) []string {
	t.Helper()
	type req struct {
		CompanyID string `json:"company_id"`
	}
	ids := make([]string, 0, len(pub.payloads))
	for _, p := range pub.payloads {
		var r req
		if err := json.Unmarshal(p, &r); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		ids = append(ids, r.CompanyID)
	}
	return ids
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestEnqueueDue_NoCompaniesDue(t *testing.T) {
	reader := &fakeCompanyReader{due: []model.Company{}}
	pub := &fakePublisher{}

	enqueuedue(context.Background(), logger.NewNoop(), reader, pub, 1440)

	if len(pub.payloads) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.payloads))
	}
}

func TestEnqueueDue_PublishesAllDueCompanies(t *testing.T) {
	reader := &fakeCompanyReader{due: []model.Company{
		company("company-1", "Acme"),
		company("company-2", "Globex"),
		company("company-3", "Initech"),
	}}
	pub := &fakePublisher{}

	enqueuedue(context.Background(), logger.NewNoop(), reader, pub, 1440)

	ids := publishedIDs(t, pub)
	if len(ids) != 3 {
		t.Fatalf("expected 3 publishes, got %d", len(ids))
	}
	want := map[string]bool{"company-1": true, "company-2": true, "company-3": true}
	for _, id := range ids {
		if !want[id] {
			t.Errorf("unexpected company_id published: %q", id)
		}
	}
}

func TestEnqueueDue_StoreError_PublishesNothing(t *testing.T) {
	reader := &fakeCompanyReader{err: errors.New("db down")}
	pub := &fakePublisher{}

	enqueuedue(context.Background(), logger.NewNoop(), reader, pub, 1440)

	if len(pub.payloads) != 0 {
		t.Errorf("expected no publishes on store error, got %d", len(pub.payloads))
	}
}

func TestEnqueueDue_PublishError_ContinuesToNext(t *testing.T) {
	reader := &fakeCompanyReader{due: []model.Company{
		company("company-1", "Acme"),
		company("company-2", "Globex"),
	}}
	// Publisher fails on every call
	pub := &fakePublisher{err: errors.New("kafka unavailable")}

	// Should not panic — just logs and continues
	enqueuedue(context.Background(), logger.NewNoop(), reader, pub, 1440)

	if len(pub.payloads) != 0 {
		t.Errorf("expected no successful publishes, got %d", len(pub.payloads))
	}
}

func TestEnqueueDue_PayloadShape(t *testing.T) {
	reader := &fakeCompanyReader{due: []model.Company{company("abc-123", "TestCo")}}
	pub := &fakePublisher{}

	enqueuedue(context.Background(), logger.NewNoop(), reader, pub, 1440)

	if len(pub.payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(pub.payloads))
	}

	ids := publishedIDs(t, pub)
	if ids[0] != "abc-123" {
		t.Errorf("expected company_id=%q, got %q", "abc-123", ids[0])
	}
}
