package tests

import (
	"context"
	"errors"
	"testing"

	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
)

// recordingAtsNext is a minimal AtsService double that records invocations
// and returns configured values. Used to verify BaseAtsService delegates to
// next without any transformation.
type recordingAtsNext struct {
	listCalled   bool
	getCalled    bool
	updateCalled bool

	listResult   []atsdto.ATSDTO
	listErr      error
	getResult    *atsdto.ATSDTO
	getErr       error
	updateResult *atsdto.ATSDTO
	updateErr    error
}

func (r *recordingAtsNext) ListATS(_ context.Context) ([]atsdto.ATSDTO, error) {
	r.listCalled = true
	return r.listResult, r.listErr
}

func (r *recordingAtsNext) GetATS(_ context.Context, _ string) (atsdto.ATSDTO, error) {
	r.getCalled = true
	if r.getResult != nil {
		return *r.getResult, r.getErr
	}
	return atsdto.ATSDTO{}, r.getErr
}

func (r *recordingAtsNext) UpdateATS(_ context.Context, _ string, _ atsdto.UpdateATSInput) (atsdto.ATSDTO, error) {
	r.updateCalled = true
	if r.updateResult != nil {
		return *r.updateResult, r.updateErr
	}
	return atsdto.ATSDTO{}, r.updateErr
}

// atsBaseHarness embeds BaseAtsService to exercise the pass-through path.
type atsBaseHarness struct {
	atssvc.BaseAtsService
}

func newAtsBaseHarness(next atssvc.AtsService) *atsBaseHarness {
	return &atsBaseHarness{BaseAtsService: atssvc.NewBaseAtsService(next)}
}

// TestBaseAtsService_NilNextPanics verifies that passing nil to
// NewBaseAtsService is caught as a programmer error at construction time.
func TestBaseAtsService_NilNextPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = atssvc.NewBaseAtsService(nil)
}

// TestBaseAtsService_DelegatesListATS verifies the pass-through path for ListATS.
func TestBaseAtsService_DelegatesListATS(t *testing.T) {
	t.Parallel()
	want := []atsdto.ATSDTO{{Name: "greenhouse"}}
	next := &recordingAtsNext{listResult: want}
	b := newAtsBaseHarness(next)

	got, err := b.ListATS(context.Background())
	if err != nil {
		t.Fatalf("ListATS: %v", err)
	}
	if !next.listCalled {
		t.Error("next.ListATS not invoked")
	}
	if len(got) != 1 || got[0].Name != "greenhouse" {
		t.Errorf("got %v, want greenhouse", got)
	}
}

// TestBaseAtsService_DelegatesGetATS verifies the pass-through path for GetATS.
func TestBaseAtsService_DelegatesGetATS(t *testing.T) {
	t.Parallel()
	want := &atsdto.ATSDTO{Name: "lever"}
	next := &recordingAtsNext{getResult: want}
	b := newAtsBaseHarness(next)

	got, err := b.GetATS(context.Background(), "lever")
	if err != nil {
		t.Fatalf("GetATS: %v", err)
	}
	if !next.getCalled {
		t.Error("next.GetATS not invoked")
	}
	if got.Name != "lever" {
		t.Errorf("got name %q, want lever", got.Name)
	}
}

// TestBaseAtsService_DelegatesUpdateATS verifies the pass-through path for UpdateATS.
func TestBaseAtsService_DelegatesUpdateATS(t *testing.T) {
	t.Parallel()
	want := &atsdto.ATSDTO{Name: "lever"}
	next := &recordingAtsNext{updateResult: want}
	b := newAtsBaseHarness(next)

	active := true
	got, err := b.UpdateATS(context.Background(), "lever", atsdto.UpdateATSInput{IsActive: &active})
	if err != nil {
		t.Fatalf("UpdateATS: %v", err)
	}
	if !next.updateCalled {
		t.Error("next.UpdateATS not invoked")
	}
	if got.Name != "lever" {
		t.Errorf("got name %q, want lever", got.Name)
	}
}

// TestBaseAtsService_PropagatesErrors verifies that errors from next are
// returned unmodified.
func TestBaseAtsService_PropagatesErrors(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("downstream failure")
	next := &recordingAtsNext{
		listErr:   wantErr,
		getErr:    wantErr,
		updateErr: wantErr,
	}
	b := newAtsBaseHarness(next)
	ctx := context.Background()

	if _, err := b.ListATS(ctx); !errors.Is(err, wantErr) {
		t.Errorf("ListATS err = %v, want %v", err, wantErr)
	}
	if _, err := b.GetATS(ctx, "x"); !errors.Is(err, wantErr) {
		t.Errorf("GetATS err = %v, want %v", err, wantErr)
	}
	if _, err := b.UpdateATS(ctx, "x", atsdto.UpdateATSInput{}); !errors.Is(err, wantErr) {
		t.Errorf("UpdateATS err = %v, want %v", err, wantErr)
	}
}
