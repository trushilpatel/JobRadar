package scan

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/service/scan/dto"
	svc "github.com/trushilpatel/jobradar/service/scan/services"
)

// crawlerQueueFakeStore is a focused ScanQueueStore double for the crawler
// queue worker tests. Each crawler-facing method records its argument so the
// tests can verify the service passes the value through unchanged.
type crawlerQueueFakeStore struct {
	claimItem        *dto.ScanQueueItem
	err              error
	claimAdapterArg  string
	completeIDArg    string
	resetStuckArg    time.Duration
	resetStuckCount  int
	heartbeatIDArg   string
	heartbeatCallCnt int
}

func (f *crawlerQueueFakeStore) Enqueue(context.Context, string, string) error { return nil }
func (f *crawlerQueueFakeStore) EnqueueDue(context.Context, int, string) (svc.EnqueueDueResult, error) {
	return svc.EnqueueDueResult{}, nil
}
func (f *crawlerQueueFakeStore) EnqueueAll(context.Context, string) (int, error) { return 0, nil }
func (f *crawlerQueueFakeStore) ClaimNext(_ context.Context, adapter string) (*dto.ScanQueueItem, error) {
	f.claimAdapterArg = adapter
	return f.claimItem, f.err
}
func (f *crawlerQueueFakeStore) Complete(_ context.Context, id string) error {
	f.completeIDArg = id
	return f.err
}
func (f *crawlerQueueFakeStore) Remove(context.Context, string) error { return nil }
func (f *crawlerQueueFakeStore) ResetStuck(_ context.Context, d time.Duration) (int, error) {
	f.resetStuckArg = d
	return f.resetStuckCount, f.err
}
func (f *crawlerQueueFakeStore) PurgeInactive(context.Context) (int, error) { return 0, nil }
func (f *crawlerQueueFakeStore) AutoPauseChronicFailures(context.Context, int) ([]string, error) {
	return nil, nil
}
func (f *crawlerQueueFakeStore) UpdateHeartbeat(_ context.Context, id string) error {
	f.heartbeatIDArg = id
	f.heartbeatCallCnt++
	return f.err
}
func (f *crawlerQueueFakeStore) ListQueue(context.Context, dto.QueueFilter) ([]dto.ScanQueueItem, error) {
	return nil, nil
}
func (f *crawlerQueueFakeStore) CountQueued(context.Context) (int, error) { return 0, nil }
func (f *crawlerQueueFakeStore) OldestQueuedAgeSeconds(context.Context) (float64, error) {
	return 0, nil
}

var _ svc.ScanQueueStore = (*crawlerQueueFakeStore)(nil)

// TestCrawlerQueueService_ClaimNext_OK asserts the adapter type is passed
// through and the claimed item is returned unchanged.
func TestCrawlerQueueService_ClaimNext_OK(t *testing.T) {
	want := &dto.ScanQueueItem{ID: "q1", CompanyID: "stripe", AdapterType: "greenhouse"}
	store := &crawlerQueueFakeStore{claimItem: want}
	s := svc.NewCrawlerQueueService(store)

	got, err := s.ClaimNext(context.Background(), "greenhouse")
	if err != nil {
		t.Fatalf("ClaimNext() error = %v", err)
	}
	if got != want {
		t.Errorf("expected %+v, got %+v", want, got)
	}
	if store.claimAdapterArg != "greenhouse" {
		t.Errorf("expected adapter 'greenhouse', got %q", store.claimAdapterArg)
	}
}

// TestCrawlerQueueService_ClaimNext_EmptyQueue asserts a nil-nil return is
// preserved (worker sleeps and retries; no error surfaced).
func TestCrawlerQueueService_ClaimNext_EmptyQueue(t *testing.T) {
	store := &crawlerQueueFakeStore{}
	s := svc.NewCrawlerQueueService(store)

	got, err := s.ClaimNext(context.Background(), "lever")
	if err != nil {
		t.Fatalf("ClaimNext() error = %v", err)
	}
	if got != nil {
		t.Errorf("expected nil item, got %+v", got)
	}
}

// TestCrawlerQueueService_ClaimNext_StoreError asserts the service wraps the
// store error with a contextual fmt.Errorf — preserving the original via %w.
func TestCrawlerQueueService_ClaimNext_StoreError(t *testing.T) {
	underlying := errors.New("db down")
	store := &crawlerQueueFakeStore{err: underlying}
	s := svc.NewCrawlerQueueService(store)

	_, err := s.ClaimNext(context.Background(), "greenhouse")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped underlying, got %v", err)
	}
	if !strings.Contains(err.Error(), "claim queue item") {
		t.Errorf("expected context 'claim queue item', got %q", err.Error())
	}
}

func TestCrawlerQueueService_Complete_OK(t *testing.T) {
	store := &crawlerQueueFakeStore{}
	s := svc.NewCrawlerQueueService(store)

	if err := s.Complete(context.Background(), "entry-1"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if store.completeIDArg != "entry-1" {
		t.Errorf("expected id 'entry-1', got %q", store.completeIDArg)
	}
}

func TestCrawlerQueueService_Complete_StoreError(t *testing.T) {
	underlying := errors.New("db down")
	store := &crawlerQueueFakeStore{err: underlying}
	s := svc.NewCrawlerQueueService(store)

	err := s.Complete(context.Background(), "entry-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped underlying, got %v", err)
	}
	if !strings.Contains(err.Error(), "complete queue item") {
		t.Errorf("missing wrap context: %v", err)
	}
}

func TestCrawlerQueueService_ResetStuck_OK(t *testing.T) {
	store := &crawlerQueueFakeStore{resetStuckCount: 3}
	s := svc.NewCrawlerQueueService(store)

	n, err := s.ResetStuck(context.Background(), 2*time.Minute)
	if err != nil {
		t.Fatalf("ResetStuck() error = %v", err)
	}
	if n != 3 {
		t.Errorf("expected count 3 passed through, got %d", n)
	}
	if store.resetStuckArg != 2*time.Minute {
		t.Errorf("expected 2m, got %v", store.resetStuckArg)
	}
}

func TestCrawlerQueueService_ResetStuck_StoreError(t *testing.T) {
	underlying := errors.New("db down")
	store := &crawlerQueueFakeStore{err: underlying}
	s := svc.NewCrawlerQueueService(store)

	_, err := s.ResetStuck(context.Background(), time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped underlying, got %v", err)
	}
	if !strings.Contains(err.Error(), "reset stuck queue items") {
		t.Errorf("missing wrap context: %v", err)
	}
}

func TestCrawlerQueueService_UpdateHeartbeat_OK(t *testing.T) {
	store := &crawlerQueueFakeStore{}
	s := svc.NewCrawlerQueueService(store)

	if err := s.UpdateHeartbeat(context.Background(), "entry-2"); err != nil {
		t.Fatalf("UpdateHeartbeat() error = %v", err)
	}
	if store.heartbeatIDArg != "entry-2" {
		t.Errorf("expected id 'entry-2', got %q", store.heartbeatIDArg)
	}
	if store.heartbeatCallCnt != 1 {
		t.Errorf("expected exactly one call, got %d", store.heartbeatCallCnt)
	}
}

func TestCrawlerQueueService_UpdateHeartbeat_StoreError(t *testing.T) {
	underlying := errors.New("db down")
	store := &crawlerQueueFakeStore{err: underlying}
	s := svc.NewCrawlerQueueService(store)

	err := s.UpdateHeartbeat(context.Background(), "entry-2")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped underlying, got %v", err)
	}
	if !strings.Contains(err.Error(), "update heartbeat") {
		t.Errorf("missing wrap context: %v", err)
	}
}
