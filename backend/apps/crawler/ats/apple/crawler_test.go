package apple_test

import (
	"context"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/apple"
	"github.com/trushilpatel/jobradar/apps/crawler/workers"
	"github.com/trushilpatel/jobradar/common/logger"
)

// noopQueue is a workers.CrawlerQueueService that always returns nil.
type noopQueue struct{}

func (q *noopQueue) ClaimNext(_ context.Context, _ string) (*workers.ScanQueueItem, error) {
	return nil, nil
}
func (q *noopQueue) Complete(_ context.Context, _ string) error        { return nil }
func (q *noopQueue) UpdateHeartbeat(_ context.Context, _ string) error { return nil }

var _ workers.CrawlerQueueService = (*noopQueue)(nil)

// noopScanner is a workers.Scanner that does nothing.
type noopScanner struct{}

func (s *noopScanner) RunForCompany(_ context.Context, _ string) {}

var _ workers.Scanner = (*noopScanner)(nil)

// TestCrawlerService_AdapterType verifies the constant returned to the
// workers registry. The registry routes queue claims by this string — a
// typo here would silently starve the worker lane.
func TestCrawlerService_AdapterType(t *testing.T) {
	svc := apple.NewCrawlerService(&noopQueue{}, &noopScanner{}, logger.NewNoop())
	if got := svc.AdapterType(); got != "apple" {
		t.Errorf("AdapterType: got %q, want %q", got, "apple")
	}
}

// TestCrawlerService_Run_StopsOnContextCancel verifies the worker loop
// exits cleanly when the supplied context is cancelled — required for
// graceful shutdown.
func TestCrawlerService_Run_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	svc := apple.NewCrawlerService(&noopQueue{}, &noopScanner{}, logger.NewNoop())

	done := make(chan struct{})
	go func() {
		svc.Run(ctx)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return after context cancellation")
	}
}

// TestCrawlerService_ImplementsAdapterCrawlerService verifies the worker
// satisfies the registry contract.
func TestCrawlerService_ImplementsAdapterCrawlerService(t *testing.T) {
	var _ workers.AdapterCrawlerService = apple.NewCrawlerService(&noopQueue{}, &noopScanner{}, logger.NewNoop())
}
