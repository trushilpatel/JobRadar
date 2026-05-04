package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/trushilpatel/jobradar/common/identity"
	"github.com/trushilpatel/jobradar/common/logger"
	auditmodel "github.com/trushilpatel/jobradar/service/audit/models"
)

// recordedAuditService captures every Append call so tests can assert what
// the middleware decided to record (or not). Safe for concurrent use because
// httptest can run handlers in parallel.
type recordedAuditService struct {
	mu      sync.Mutex
	entries []auditmodel.Entry
	err     error
}

func (r *recordedAuditService) Append(_ context.Context, e auditmodel.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
	return r.err
}

func (r *recordedAuditService) snapshot() []auditmodel.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]auditmodel.Entry, len(r.entries))
	copy(out, r.entries)
	return out
}

func TestAuditAdminMutation_RecordsOn2xxMutation(t *testing.T) {
	svc := &recordedAuditService{}
	mw := AuditAdminMutation(svc, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/admin/companies/abc", nil)
	req = req.WithContext(identity.WithTestIdentity(req.Context(), "actor-1", "admin"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("got %d want 201", rr.Code)
	}
	got := svc.snapshot()
	if len(got) != 1 {
		t.Fatalf("want 1 audit entry, got %d", len(got))
	}
	if got[0].ActorUserID == nil || *got[0].ActorUserID != "actor-1" {
		t.Fatalf("actor=%v want actor-1", got[0].ActorUserID)
	}
	if got[0].Action != "POST /admin/companies/abc" {
		t.Fatalf("action=%q", got[0].Action)
	}
}

func TestAuditAdminMutation_SkipsGet(t *testing.T) {
	svc := &recordedAuditService{}
	mw := AuditAdminMutation(svc, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req = req.WithContext(identity.WithTestIdentity(req.Context(), "actor-1", "admin"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got := svc.snapshot(); len(got) != 0 {
		t.Fatalf("GET must not be audited, got %d entries", len(got))
	}
}

func TestAuditAdminMutation_Skips4xx(t *testing.T) {
	svc := &recordedAuditService{}
	mw := AuditAdminMutation(svc, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	req := httptest.NewRequest(http.MethodPost, "/admin/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got := svc.snapshot(); len(got) != 0 {
		t.Fatalf("4xx must not be audited, got %d entries", len(got))
	}
}

func TestAuditAdminMutation_Skips5xx(t *testing.T) {
	svc := &recordedAuditService{}
	mw := AuditAdminMutation(svc, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodDelete, "/admin/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got := svc.snapshot(); len(got) != 0 {
		t.Fatalf("5xx must not be audited, got %d entries", len(got))
	}
}

func TestAuditAdminMutation_AppendErrorDoesNotAffectResponse(t *testing.T) {
	svc := &recordedAuditService{err: errors.New("db down")}
	mw := AuditAdminMutation(svc, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPatch, "/admin/x", nil)
	req = req.WithContext(identity.WithTestIdentity(req.Context(), "a", "admin"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d want 200 — append error must not affect response", rr.Code)
	}
}

func TestAuditAdminMutation_NilService_NoOp(t *testing.T) {
	mw := AuditAdminMutation(nil, logger.NewNoop())
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/admin/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !called || rr.Code != http.StatusOK {
		t.Fatalf("nil svc must pass through, called=%v code=%d", called, rr.Code)
	}
}

func TestAuditAdminMutation_AnonymousActor(t *testing.T) {
	svc := &recordedAuditService{}
	mw := AuditAdminMutation(svc, logger.NewNoop())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No identity attached — actor should be nil, not panic.
	req := httptest.NewRequest(http.MethodPost, "/admin/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	got := svc.snapshot()
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	if got[0].ActorUserID != nil {
		t.Fatalf("actor=%v want nil", got[0].ActorUserID)
	}
}
