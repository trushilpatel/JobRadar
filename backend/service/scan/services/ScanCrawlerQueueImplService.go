package services

import (
	"context"
	"fmt"
	"time"

	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// CrawlerQueueService is the interface the crawler worker uses for queue operations.
type CrawlerQueueService interface {
	// ClaimNext atomically claims the next queued item for the given adapterType,
	// enforcing the inter-company delay from adapter_configs. Returns nil, nil if
	// the queue is empty or the delay has not yet elapsed.
	ClaimNext(ctx context.Context, adapterType string) (*dto.ScanQueueItem, error)
	// Complete returns ErrQueueRowMissing when the row was already removed/reassigned.
	Complete(ctx context.Context, id string) error
	// ResetStuck returns the number of rows reclaimed.
	ResetStuck(ctx context.Context, staleDur time.Duration) (int, error)
	// UpdateHeartbeat returns ErrQueueRowMissing when the row was reassigned by
	// ResetStuck — the crawler must abort the in-flight scan.
	UpdateHeartbeat(ctx context.Context, id string) error
}

type crawlerQueueService struct {
	store ScanQueueStore
}

// NewCrawlerQueueService returns a CrawlerQueueService backed by the scan queue store.
func NewCrawlerQueueService(store ScanQueueStore) CrawlerQueueService {
	return &crawlerQueueService{store: store}
}

func (s *crawlerQueueService) ClaimNext(ctx context.Context, adapterType string) (_ *dto.ScanQueueItem, err error) {
	ctx, span := tracing.Start(ctx, "scan.crawlerqueue.ClaimNext")
	defer tracing.End(span, &err)
	item, err := s.store.ClaimNext(ctx, adapterType)
	if err != nil {
		return nil, fmt.Errorf("claim queue item: %w", err)
	}
	return item, nil
}

func (s *crawlerQueueService) Complete(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.crawlerqueue.Complete")
	defer tracing.End(span, &err)
	if err := s.store.Complete(ctx, id); err != nil {
		return fmt.Errorf("complete queue item: %w", err)
	}
	return nil
}

func (s *crawlerQueueService) ResetStuck(ctx context.Context, staleDur time.Duration) (rowsReset int, err error) {
	ctx, span := tracing.Start(ctx, "scan.crawlerqueue.ResetStuck")
	defer tracing.End(span, &err)
	n, err := s.store.ResetStuck(ctx, staleDur)
	if err != nil {
		return 0, fmt.Errorf("reset stuck queue items: %w", err)
	}
	return n, nil
}

func (s *crawlerQueueService) UpdateHeartbeat(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.crawlerqueue.UpdateHeartbeat")
	defer tracing.End(span, &err)
	if err := s.store.UpdateHeartbeat(ctx, id); err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}
	return nil
}
