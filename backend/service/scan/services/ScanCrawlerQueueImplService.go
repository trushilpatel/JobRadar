package services

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/common/domain"
	"github.com/trushilpatel/jobradar/platform/tracing"
	"github.com/trushilpatel/jobradar/service/scan/dto"
)

// CrawlerQueueService is the interface the crawler worker uses for queue operations.
type CrawlerQueueService interface {
	ClaimNext(ctx context.Context) (*dto.ScanQueueItem, error)
	Complete(ctx context.Context, id string) error
	ResetStuck(ctx context.Context, staleDur time.Duration) error
	UpdateHeartbeat(ctx context.Context, id string) error
}

type crawlerQueueService struct {
	store ScanQueueStore
}

// NewCrawlerQueueService returns a CrawlerQueueService backed by the scan queue store.
func NewCrawlerQueueService(store ScanQueueStore) CrawlerQueueService {
	return &crawlerQueueService{store: store}
}

func (s *crawlerQueueService) ClaimNext(ctx context.Context) (_ *dto.ScanQueueItem, err error) {
	ctx, span := tracing.Start(ctx, "scan.crawlerqueue.ClaimNext")
	defer tracing.End(span, &err)
	item, err := s.store.ClaimNext(ctx)
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to claim queue item")
	}
	return item, nil
}

func (s *crawlerQueueService) Complete(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.crawlerqueue.Complete")
	defer tracing.End(span, &err)
	if err := s.store.Complete(ctx, id); err != nil {
		return domain.ErrInternal.WithMessage("failed to complete queue item")
	}
	return nil
}

func (s *crawlerQueueService) ResetStuck(ctx context.Context, staleDur time.Duration) (err error) {
	ctx, span := tracing.Start(ctx, "scan.crawlerqueue.ResetStuck")
	defer tracing.End(span, &err)
	if err := s.store.ResetStuck(ctx, staleDur); err != nil {
		return domain.ErrInternal.WithMessage("failed to reset stuck queue items")
	}
	return nil
}

func (s *crawlerQueueService) UpdateHeartbeat(ctx context.Context, id string) (err error) {
	ctx, span := tracing.Start(ctx, "scan.crawlerqueue.UpdateHeartbeat")
	defer tracing.End(span, &err)
	if err := s.store.UpdateHeartbeat(ctx, id); err != nil {
		return domain.ErrInternal.WithMessage("failed to update heartbeat")
	}
	return nil
}
