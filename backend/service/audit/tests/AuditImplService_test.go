package tests

import (
	"context"
	"errors"
	"testing"

	auditmodel "github.com/trushilpatel/jobradar/service/audit/models"
	auditsvc "github.com/trushilpatel/jobradar/service/audit/services"
)

type fakeAuditRepo struct {
	appendCalled bool
	appendErr    error
	listCalled   bool
	listEntries  []auditmodel.Entry
	listTotal    int64
}

func (f *fakeAuditRepo) Append(_ context.Context, _ auditmodel.Entry) error {
	f.appendCalled = true
	return f.appendErr
}
func (f *fakeAuditRepo) List(_ context.Context, _ auditsvc.ListFilter) ([]auditmodel.Entry, int64, error) {
	f.listCalled = true
	return f.listEntries, f.listTotal, nil
}

func TestAuditImpl_AppendDelegates(t *testing.T) {
	t.Parallel()
	repo := &fakeAuditRepo{}
	s := auditsvc.NewImplService(repo)
	if err := s.Append(context.Background(), auditmodel.Entry{Action: "user.delete"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if !repo.appendCalled {
		t.Fatal("expected delegation")
	}
}

func TestAuditImpl_AppendPropagatesError(t *testing.T) {
	t.Parallel()
	want := errors.New("boom")
	repo := &fakeAuditRepo{appendErr: want}
	s := auditsvc.NewImplService(repo)
	if err := s.Append(context.Background(), auditmodel.Entry{Action: "x"}); !errors.Is(err, want) {
		t.Errorf("err = %v, want wrapped %v", err, want)
	}
}

func TestAuditImpl_ListDelegates(t *testing.T) {
	t.Parallel()
	repo := &fakeAuditRepo{
		listEntries: []auditmodel.Entry{{Action: "x"}},
		listTotal:   1,
	}
	s := auditsvc.NewImplService(repo)
	rows, total, err := s.List(context.Background(), auditsvc.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Errorf("rows=%v total=%d", rows, total)
	}
	if !repo.listCalled {
		t.Fatal("expected delegation")
	}
}
