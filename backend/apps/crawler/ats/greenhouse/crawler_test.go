package greenhouse_test

import (
	"context"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/greenhouse"
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

func TestCrawlerService_AdapterType(t *testing.T) {
	svc := greenhouse.NewCrawlerService(&noopQueue{}, &noopScanner{}, logger.NewNoop())
	if got := svc.AdapterType(); got != "greenhouse" {
		t.Errorf("expected AdapterType() == %q, got %q", "greenhouse", got)
	}
}

func TestCrawlerService_Run_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	svc := greenhouse.NewCrawlerService(&noopQueue{}, &noopScanner{}, logger.NewNoop())

	done := make(chan struct{})
	go func() {
		svc.Run(ctx)
		close(done)
	}()

	// Give the goroutine time to enter its sleep loop.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Correct — Run returned after cancel.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestCrawlerService_ImplementsAdapterCrawlerService(t *testing.T) {
	var _ workers.AdapterCrawlerService = greenhouse.NewCrawlerService(&noopQueue{}, &noopScanner{}, logger.NewNoop())
}
