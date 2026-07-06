package tests

import (
	"context"
	"errors"
	"testing"

	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
)

// TestAtsImplService_PassesThroughListATS verifies ImplService delegates ListATS
// to the underlying chain without modification.
func TestAtsImplService_PassesThroughListATS(t *testing.T) {
	t.Parallel()
	want := []atsdto.ATSDTO{{Name: "greenhouse"}}
	next := &recordingAtsNext{listResult: want}
	svc := atssvc.NewImplService(next)

	got, err := svc.ListATS(context.Background())
	if err != nil {
		t.Fatalf("ListATS: %v", err)
	}
	if !next.listCalled {
		t.Error("next.ListATS not invoked")
	}
	if len(got) != 1 || got[0].Name != "greenhouse" {
		t.Errorf("got %v, want [greenhouse]", got)
	}
}

// TestAtsImplService_PassesThroughGetATS verifies ImplService delegates GetATS.
func TestAtsImplService_PassesThroughGetATS(t *testing.T) {
	t.Parallel()
	want := &atsdto.ATSDTO{Name: "lever"}
	next := &recordingAtsNext{getResult: want}
	svc := atssvc.NewImplService(next)

	got, err := svc.GetATS(context.Background(), "lever")
	if err != nil {
		t.Fatalf("GetATS: %v", err)
	}
	if !next.getCalled {
		t.Error("next.GetATS not invoked")
	}
	if got.Name != "lever" {
		t.Errorf("name = %q, want lever", got.Name)
	}
}

// TestAtsImplService_PassesThroughUpdateATS verifies ImplService delegates UpdateATS.
func TestAtsImplService_PassesThroughUpdateATS(t *testing.T) {
	t.Parallel()
	want := &atsdto.ATSDTO{Name: "ashby"}
	next := &recordingAtsNext{updateResult: want}
	svc := atssvc.NewImplService(next)

	interval := 720
	got, err := svc.UpdateATS(context.Background(), "ashby", atsdto.UpdateATSInput{ScanIntervalMin: &interval})
	if err != nil {
		t.Fatalf("UpdateATS: %v", err)
	}
	if !next.updateCalled {
		t.Error("next.UpdateATS not invoked")
	}
	if got.Name != "ashby" {
		t.Errorf("name = %q, want ashby", got.Name)
	}
}

// TestAtsImplService_PropagatesErrors verifies that errors from next pass through.
func TestAtsImplService_PropagatesErrors(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("next error")
	next := &recordingAtsNext{listErr: wantErr, getErr: wantErr, updateErr: wantErr}
	svc := atssvc.NewImplService(next)
	ctx := context.Background()

	if _, err := svc.ListATS(ctx); !errors.Is(err, wantErr) {
		t.Errorf("ListATS err = %v, want %v", err, wantErr)
	}
	if _, err := svc.GetATS(ctx, "x"); !errors.Is(err, wantErr) {
		t.Errorf("GetATS err = %v, want %v", err, wantErr)
	}
	if _, err := svc.UpdateATS(ctx, "x", atsdto.UpdateATSInput{}); !errors.Is(err, wantErr) {
		t.Errorf("UpdateATS err = %v, want %v", err, wantErr)
	}
}
