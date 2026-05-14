package tests

import (
	"context"
	"errors"
	"testing"

	auditmodel "github.com/trushilpatel/jobradar/service/audit/models"
	auditsvc "github.com/trushilpatel/jobradar/service/audit/services"
)

// TestNewBaseService_PanicsOnNilNext locks down the BaseService invariant —
// every decorator layer must have a downstream service; a nil next means a
// broken wire setup, and we want the panic to fire at construction time.
func TestNewBaseService_PanicsOnNilNext(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = auditsvc.NewBaseService(nil)
}

// TestBaseService_AppendAndListDelegate verifies the auto-delegation methods
// route to next unchanged. NewImplService produces a base-only chain so this
// is the natural place to exercise both methods.
func TestBaseService_AppendAndListDelegate(t *testing.T) {
	t.Parallel()
	repo := &fakeAuditRepo{
		listEntries: []auditmodel.Entry{{Action: "z"}, {Action: "y"}},
		listTotal:   2,
	}
	base := auditsvc.NewBaseService(repo)

	if err := base.Append(context.Background(), auditmodel.Entry{Action: "z"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if !repo.appendCalled {
		t.Fatal("base.Append did not delegate")
	}

	rows, total, err := base.List(context.Background(), auditsvc.ListFilter{Limit: 7})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !repo.listCalled {
		t.Fatal("base.List did not delegate")
	}
	if total != 2 || len(rows) != 2 {
		t.Errorf("rows=%d total=%d, want 2/2", len(rows), total)
	}
	if repo.listFilter.Limit != 7 {
		t.Errorf("filter not forwarded: %+v", repo.listFilter)
	}
}

// TestBaseService_PropagatesAppendError ensures the delegating wrapper does
// not swallow next-layer errors. errors.Is must continue to match.
func TestBaseService_PropagatesAppendError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("nope")
	base := auditsvc.NewBaseService(&fakeAuditRepo{appendErr: sentinel})
	if err := base.Append(context.Background(), auditmodel.Entry{Action: "x"}); !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want %v", err, sentinel)
	}
}
