package tests

import (
	"context"
	"errors"
	"testing"

	auditmodel "github.com/trushilpatel/jobradar/service/audit/models"
	auditsvc "github.com/trushilpatel/jobradar/service/audit/services"
)

// fakeAuditRepo records every call and returns canned responses. It is the
// terminal service double for the audit decorator chain in tests.
type fakeAuditRepo struct {
	appendCalled bool
	appendEntry  auditmodel.Entry
	appendErr    error

	listCalled  bool
	listFilter  auditsvc.ListFilter
	listEntries []auditmodel.Entry
	listTotal   int64
	listErr     error
}

func (f *fakeAuditRepo) Append(_ context.Context, entry auditmodel.Entry) error {
	f.appendCalled = true
	f.appendEntry = entry
	return f.appendErr
}

func (f *fakeAuditRepo) List(_ context.Context, filter auditsvc.ListFilter) ([]auditmodel.Entry, int64, error) {
	f.listCalled = true
	f.listFilter = filter
	return f.listEntries, f.listTotal, f.listErr
}

// Ensure the fake satisfies the contract.
var _ auditsvc.Service = (*fakeAuditRepo)(nil)

// TestAuditImpl_AppendDelegates asserts the impl layer passes the entry to
// the next service untouched.
func TestAuditImpl_AppendDelegates(t *testing.T) {
	t.Parallel()
	repo := &fakeAuditRepo{}
	s := auditsvc.NewImplService(repo)

	entry := auditmodel.Entry{Action: auditmodel.ActionUserDelete}
	if err := s.Append(context.Background(), entry); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if !repo.appendCalled {
		t.Fatal("expected delegation to repo.Append")
	}
	if repo.appendEntry.Action != auditmodel.ActionUserDelete {
		t.Errorf("entry.Action = %q, want %q", repo.appendEntry.Action, auditmodel.ActionUserDelete)
	}
}

// TestAuditImpl_AppendPropagatesError asserts errors from the next layer
// surface unchanged (errors.Is must match the original).
func TestAuditImpl_AppendPropagatesError(t *testing.T) {
	t.Parallel()
	want := errors.New("boom")
	repo := &fakeAuditRepo{appendErr: want}
	s := auditsvc.NewImplService(repo)
	if err := s.Append(context.Background(), auditmodel.Entry{Action: "x"}); !errors.Is(err, want) {
		t.Errorf("err = %v, want wrapped %v", err, want)
	}
}

// TestAuditImpl_ListDelegates asserts the filter is forwarded verbatim and
// the row+total pair is returned.
func TestAuditImpl_ListDelegates(t *testing.T) {
	t.Parallel()
	repo := &fakeAuditRepo{
		listEntries: []auditmodel.Entry{{Action: "x"}},
		listTotal:   1,
	}
	s := auditsvc.NewImplService(repo)
	actor := "user-42"
	in := auditsvc.ListFilter{Limit: 10, Offset: 5, ActorUserID: &actor}
	rows, total, err := s.List(context.Background(), in)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Errorf("rows=%v total=%d", rows, total)
	}
	if !repo.listCalled {
		t.Fatal("expected delegation to repo.List")
	}
	if repo.listFilter.Limit != 10 || repo.listFilter.Offset != 5 {
		t.Errorf("filter limit/offset = %d/%d, want 10/5", repo.listFilter.Limit, repo.listFilter.Offset)
	}
	if repo.listFilter.ActorUserID == nil || *repo.listFilter.ActorUserID != "user-42" {
		t.Errorf("filter.ActorUserID = %v, want user-42", repo.listFilter.ActorUserID)
	}
}

// TestAuditImpl_ListPropagatesError asserts the impl layer does not swallow
// repo errors and does not rewrite them.
func TestAuditImpl_ListPropagatesError(t *testing.T) {
	t.Parallel()
	want := errors.New("kapow")
	repo := &fakeAuditRepo{listErr: want}
	s := auditsvc.NewImplService(repo)
	rows, total, err := s.List(context.Background(), auditsvc.ListFilter{})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want wrapped %v", err, want)
	}
	if rows != nil || total != 0 {
		t.Errorf("rows/total leak on error: rows=%v total=%d", rows, total)
	}
}
