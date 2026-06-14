package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
)

// lazyPublisher implements SweeperPublisher with lazy NATS connection.
// It dials NATS on the first publish call and caches the connection.
// If the connection fails or the publish itself fails, the cached publisher
// is evicted so the next call re-dials — NATS outages are transparent to
// the sweeper (jobs stay at reason='retry' until the next cycle succeeds).
//
// This ensures the scheduler always constructs a lazyPublisher rather than
// branching on NATS availability at startup: the sweeper is never "disabled"
// due to a transient NATS outage (AC-2 / RC-2).
type lazyPublisher struct {
	mu        sync.Mutex
	inner     scansvc.SweeperPublisher
	connectFn func(context.Context) (scansvc.SweeperPublisher, error)
	log       logger.Logger
}

// newLazyPublisher constructs a lazyPublisher. connectFn is called on first
// use (and on every reconnect after a failure); it should return a ready
// SweeperPublisher (e.g. backed by js.NewPublisher).
func newLazyPublisher(connectFn func(context.Context) (scansvc.SweeperPublisher, error), log logger.Logger) *lazyPublisher {
	return &lazyPublisher{connectFn: connectFn, log: log}
}

// acquire returns the cached publisher, dialling NATS if needed.
// Returns (nil, err) when NATS is unavailable; increments
// SweeperNATSUnavailableTotal so the scheduler's metric surface reflects
// the outage without any additional caller instrumentation.
func (p *lazyPublisher) acquire(ctx context.Context) (scansvc.SweeperPublisher, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inner != nil {
		return p.inner, nil
	}
	pub, err := p.connectFn(ctx)
	if err != nil {
		metrics.SweeperNATSUnavailableTotal.Inc()
		p.log.Warn("lazy publisher: NATS unavailable; sweeper will retry next cycle",
			logger.Err(err),
		)
		return nil, fmt.Errorf("lazy publisher: connect: %w", err)
	}
	p.inner = pub
	return pub, nil
}

// evict clears the cached publisher so the next call re-dials.
func (p *lazyPublisher) evict() {
	p.mu.Lock()
	p.inner = nil
	p.mu.Unlock()
}

// Publish satisfies SweeperPublisher. It is kept for callers that use the
// plain publish path (e.g. future non-sweeper uses). The sweeper itself uses
// PublishWithHeaders (AC-1).
func (p *lazyPublisher) Publish(ctx context.Context, subject string, payload []byte) error {
	pub, err := p.acquire(ctx)
	if err != nil {
		return err
	}
	if perr := pub.Publish(ctx, subject, payload); perr != nil {
		p.evict()
		return perr
	}
	return nil
}

// PublishWithHeaders satisfies SweeperPublisher and is the primary path used
// by the sweeper (AC-1 / RC-1). On connection failure it returns an error;
// on publish failure it evicts the cached publisher so the next cycle re-dials.
func (p *lazyPublisher) PublishWithHeaders(ctx context.Context, subject string, payload []byte, headers map[string]string) error {
	pub, err := p.acquire(ctx)
	if err != nil {
		return err
	}
	if perr := pub.PublishWithHeaders(ctx, subject, payload, headers); perr != nil {
		p.evict()
		return perr
	}
	return nil
}

var _ scansvc.SweeperPublisher = (*lazyPublisher)(nil)
