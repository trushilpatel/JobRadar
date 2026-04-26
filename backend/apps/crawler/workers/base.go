package workers

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
)

const defaultHeartbeatInterval = 30 * time.Second

// BaseWorker contains the common claim → heartbeat → scan → complete loop
// shared by all per-adapter crawler services. Embed this in your CrawlerService
// and call RunLoop from your Run method.
type BaseWorker struct {
	adapterType       string
	queue             CrawlerQueueService
	scanner           Scanner
	log               logger.Logger
	pollInterval      time.Duration
	heartbeatInterval time.Duration
}

// NewBaseWorker constructs a BaseWorker for the given adapter type.
// pollInterval controls how long the worker sleeps when the queue is empty or
// the inter-company delay has not elapsed.
func NewBaseWorker(adapterType string, queue CrawlerQueueService, scanner Scanner, log logger.Logger, pollInterval time.Duration) BaseWorker {
	return BaseWorker{
		adapterType:       adapterType,
		queue:             queue,
		scanner:           scanner,
		log:               log,
		pollInterval:      pollInterval,
		heartbeatInterval: defaultHeartbeatInterval,
	}
}

// NewBaseWorkerWithHeartbeat is like NewBaseWorker but allows overriding the
// heartbeat interval. Intended for testing.
func NewBaseWorkerWithHeartbeat(adapterType string, queue CrawlerQueueService, scanner Scanner, log logger.Logger, pollInterval, heartbeatInterval time.Duration) BaseWorker {
	return BaseWorker{
		adapterType:       adapterType,
		queue:             queue,
		scanner:           scanner,
		log:               log,
		pollInterval:      pollInterval,
		heartbeatInterval: heartbeatInterval,
	}
}

// StartHeartbeatForTest exposes startHeartbeat for use in tests.
// Do not call this from production code — use RunLoop instead.
func (w *BaseWorker) StartHeartbeatForTest(ctx context.Context, id string) func() {
	return w.startHeartbeat(ctx, id)
}

// RunLoop runs the claim → heartbeat → scan → complete loop until ctx is cancelled.
// The inter-company delay is enforced at ClaimNext (DB level) — no sleep after scan.
func (w *BaseWorker) RunLoop(ctx context.Context) {
	w.log.Info("adapter worker started", logger.Str("adapter_type", w.adapterType))
	for {
		if ctx.Err() != nil {
			w.log.Info("adapter worker stopping", logger.Str("adapter_type", w.adapterType))
			return
		}

		item, err := w.queue.ClaimNext(ctx, w.adapterType)
		if err != nil {
			w.log.Error("claim failed", err, logger.Str("adapter_type", w.adapterType))
			if err := w.Sleep(ctx, w.pollInterval); err != nil {
				return
			}
			continue
		}
		if item == nil {
			// Queue empty or inter-company delay not yet elapsed — sleep and retry.
			if err := w.Sleep(ctx, w.pollInterval); err != nil {
				return
			}
			continue
		}

		w.log.Info("processing company",
			logger.Str("adapter_type", w.adapterType),
			logger.Str(logger.FieldCompany, item.CompanyID),
			logger.Str("source", item.Source),
		)

		stop := w.startHeartbeat(ctx, item.ID)
		func() {
			defer func() {
				if r := recover(); r != nil {
					w.log.Error("panic in RunForCompany, recovering",
						fmt.Errorf("panic: %v", r),
						logger.Str("adapter_type", w.adapterType),
						logger.Str(logger.FieldCompany, item.CompanyID),
						logger.Str("stack", string(debug.Stack())),
					)
				}
			}()
			w.scanner.RunForCompany(ctx, item.CompanyID)
		}()
		if ctx.Err() != nil {
			stop()
			w.log.Info("adapter worker stopping after panic recovery", logger.Str("adapter_type", w.adapterType))
			return
		}
		if err := w.queue.Complete(ctx, item.ID); err != nil {
			w.log.Error("complete failed", err,
				logger.Str("adapter_type", w.adapterType),
				logger.Str(logger.FieldQueueID, item.ID),
			)
		}
		stop()
		// No sleep here — the delay is enforced at ClaimNext via adapter_configs.
	}
}

// startHeartbeat ticks UpdateHeartbeat every heartbeatInterval while a scan is active.
// Returns a stop func — call it after the scan completes.
func (w *BaseWorker) startHeartbeat(ctx context.Context, id string) func() {
	hbCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(w.heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := w.queue.UpdateHeartbeat(hbCtx, id); err != nil {
					w.log.Warn("heartbeat update failed",
						logger.Str(logger.FieldQueueID, id),
						logger.Err(err),
					)
				}
			case <-hbCtx.Done():
				return
			}
		}
	}()
	return cancel
}

// Sleep pauses for d or until ctx is cancelled.
// Returns ctx.Err() if cancelled, nil otherwise.
func (w *BaseWorker) Sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
