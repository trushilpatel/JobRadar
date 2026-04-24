package workers_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/workers"
)

// stubService is a minimal AdapterCrawlerService for registry tests.
type stubService struct {
	adapterType string
	started     atomic.Bool
}

func (s *stubService) AdapterType() string { return s.adapterType }
func (s *stubService) Run(ctx context.Context) {
	s.started.Store(true)
	<-ctx.Done() // block until context is cancelled
}

var _ workers.AdapterCrawlerService = (*stubService)(nil)

func TestRegistry_StartAll_StartsAllServices(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	svcA := &stubService{adapterType: "greenhouse"}
	svcB := &stubService{adapterType: "lever"}
	svcC := &stubService{adapterType: "ashby"}

	registry := workers.NewRegistry()
	registry.Register(svcA)
	registry.Register(svcB)
	registry.Register(svcC)

	var wg sync.WaitGroup
	registry.StartAll(ctx, &wg)

	// Give goroutines time to start.
	time.Sleep(30 * time.Millisecond)

	for _, svc := range []*stubService{svcA, svcB, svcC} {
		if !svc.started.Load() {
			t.Errorf("service %q was not started", svc.adapterType)
		}
	}

	cancel()
	wg.Wait() // should unblock once all goroutines exit
}

func TestRegistry_StartAll_WaitGroupDoneAfterContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	svc := &stubService{adapterType: "test"}
	registry := workers.NewRegistry()
	registry.Register(svc)

	var wg sync.WaitGroup
	registry.StartAll(ctx, &wg)

	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// WaitGroup completed — correct.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WaitGroup did not complete after context cancellation")
	}
}

func TestRegistry_EmptyRegistry_StartAllNoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry := workers.NewRegistry()

	var wg sync.WaitGroup
	registry.StartAll(ctx, &wg) // should not panic or block

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Immediate completion — correct.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("empty registry WaitGroup should complete immediately")
	}
}
