package services

import (
	"context"
	stderrors "errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
)

// fakeGateAtsService is a minimal AtsService double for AdapterDelayGate
// tests. Only GetATS is exercised by TryReserve; the other two methods exist
// solely to satisfy the interface.
type fakeGateAtsService struct {
	delays map[string]int
	err    error
}

func (f *fakeGateAtsService) ListATS(context.Context) ([]atsdto.ATSDTO, error) { return nil, nil }

func (f *fakeGateAtsService) GetATS(_ context.Context, name string) (atsdto.ATSDTO, error) {
	if f.err != nil {
		return atsdto.ATSDTO{}, f.err
	}
	delay, ok := f.delays[name]
	if !ok {
		return atsdto.ATSDTO{}, stderrors.New("ats: unknown adapter " + name)
	}
	return atsdto.ATSDTO{Name: name, InterCompanyDelayS: delay}, nil
}

func (f *fakeGateAtsService) UpdateATS(_ context.Context, _ string, _ atsdto.UpdateATSInput) (atsdto.ATSDTO, error) {
	return atsdto.ATSDTO{}, nil
}

var _ atssvc.AtsService = (*fakeGateAtsService)(nil)

// A delay=0 adapter must never be blocked, including on a fresh/never-claimed
// adapter and across repeated reservations.
func TestAdapterDelayGate_ZeroDelay_AlwaysEligible(t *testing.T) {
	gate := NewAdapterDelayGate(&fakeGateAtsService{delays: map[string]int{"greenhouse": 0}})

	for i := 0; i < 3; i++ {
		ok, err := gate.TryReserve(context.Background(), "greenhouse")
		if err != nil {
			t.Fatalf("TryReserve() iteration %d error = %v", i, err)
		}
		if !ok {
			t.Fatalf("TryReserve() iteration %d = false, want true (delay=0 never blocks)", i)
		}
	}
}

// Two sequential reservations within the delay window: the first succeeds,
// the second (immediately after) fails; after advancing the clock past the
// delay, a third reservation succeeds again.
func TestAdapterDelayGate_SequentialReservations_RespectDelayWindow(t *testing.T) {
	gate := NewAdapterDelayGate(&fakeGateAtsService{delays: map[string]int{"lever": 5}})
	now := time.Now()
	gate.clock = func() time.Time { return now }

	ok1, err := gate.TryReserve(context.Background(), "lever")
	if err != nil || !ok1 {
		t.Fatalf("first TryReserve() = (%v, %v), want (true, nil)", ok1, err)
	}

	ok2, err := gate.TryReserve(context.Background(), "lever")
	if err != nil {
		t.Fatalf("second TryReserve() error = %v", err)
	}
	if ok2 {
		t.Fatal("second TryReserve() = true, want false (still within the delay window)")
	}

	gate.clock = func() time.Time { return now.Add(6 * time.Second) }
	ok3, err := gate.TryReserve(context.Background(), "lever")
	if err != nil || !ok3 {
		t.Fatalf("third TryReserve() (after delay elapsed) = (%v, %v), want (true, nil)", ok3, err)
	}
}

// Concurrent TryReserve calls for the same adapter must yield exactly one
// success within the delay window — proving the check-and-reserve is atomic
// under the mutex, not a check-then-act race.
func TestAdapterDelayGate_Concurrent_ExactlyOneSucceeds(t *testing.T) {
	gate := NewAdapterDelayGate(&fakeGateAtsService{delays: map[string]int{"workday": 60}})

	const goroutines = 50
	var successCount int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-start
			ok, err := gate.TryReserve(context.Background(), "workday")
			if err != nil {
				t.Errorf("TryReserve() error = %v", err)
				return
			}
			if ok {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if successCount != 1 {
		t.Fatalf("successCount = %d, want exactly 1", successCount)
	}
}

// An AtsService error (unknown/stale adapter type) must yield (false, nil) —
// fail-safe, never "always eligible" on a lookup failure, and never a panic
// or a propagated error.
func TestAdapterDelayGate_AtsServiceError_NotEligibleNoError(t *testing.T) {
	gate := NewAdapterDelayGate(&fakeGateAtsService{err: stderrors.New("ats lookup failed")})

	ok, err := gate.TryReserve(context.Background(), "unknown-adapter")
	if err != nil {
		t.Fatalf("TryReserve() error = %v, want nil", err)
	}
	if ok {
		t.Fatal("TryReserve() = true, want false on AtsService error")
	}
}

// Unknown adapter (not present in the fake's delay map — mirrors GetATS
// returning ErrNotFound) must also yield (false, nil).
func TestAdapterDelayGate_UnknownAdapter_NotEligibleNoError(t *testing.T) {
	gate := NewAdapterDelayGate(&fakeGateAtsService{delays: map[string]int{"greenhouse": 0}})

	ok, err := gate.TryReserve(context.Background(), "totally-unknown")
	if err != nil {
		t.Fatalf("TryReserve() error = %v, want nil", err)
	}
	if ok {
		t.Fatal("TryReserve() = true, want false for an adapter absent from the registry")
	}
}
