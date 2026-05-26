package workers

import (
	"context"
	stderrors "errors"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
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
// Returns the stop func and the reassigned flag the heartbeat sets when the
// queue row goes missing (signalling a scheduler reassignment).
func (w *BaseWorker) StartHeartbeatForTest(ctx context.Context, cancelScan context.CancelFunc, id string) (func(), *atomic.Bool) {
	return w.startHeartbeat(ctx, cancelScan, id)
}

// RunLoop runs the claim → heartbeat → scan → complete loop until ctx is cancelled.
// The inter-company delay is enforced at ClaimNext (DB level) — no sleep after scan.
func (w *BaseWorker) RunLoop(ctx context.Context) {
	w.log.Info("adapter worker started", logger.Str(logger.FieldAdapterType, w.adapterType))
	for {
		if ctx.Err() != nil {
			w.log.Info("adapter worker stopping", logger.Str(logger.FieldAdapterType, w.adapterType))
			return
		}

		item, err := w.queue.ClaimNext(ctx, w.adapterType)
		if err != nil {
			w.log.Error("claim failed", err, logger.Str(logger.FieldAdapterType, w.adapterType))
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
			logger.Str(logger.FieldAdapterType, w.adapterType),
			logger.Str(logger.FieldCompany, item.CompanyID),
			logger.Str(logger.FieldSource, item.Source),
		)

		// Scan runs under a cancellable child context so the heartbeat can abort
		// it if the queue row gets reassigned by the scheduler's ResetStuck.
		scanCtx, cancelScan := context.WithCancel(ctx)
		stop, reassigned := w.startHeartbeat(scanCtx, cancelScan, item.ID)
		func() {
			defer func() {
				if r := recover(); r != nil {
					w.log.Error("panic in RunForCompany, recovering",
						fmt.Errorf("panic: %v", r),
						logger.Str(logger.FieldAdapterType, w.adapterType),
						logger.Str(logger.FieldCompany, item.CompanyID),
						logger.Str(logger.FieldStack, string(debug.Stack())),
					)
				}
			}()
			w.scanner.RunForCompany(scanCtx, item.CompanyID)
		}()
		stop()
		cancelScan()

		// Row reassigned mid-scan: another worker now drives it (ResetStuck
		// requeued it). Skip Complete — completing would delete the row the
		// other worker is processing and risk double-publishing.
		if reassigned.Load() {
			w.log.Warn("scan aborted — queue row reassigned, skipping Complete",
				logger.Str(logger.FieldAdapterType, w.adapterType),
				logger.Str(logger.FieldQueueID, item.ID),
			)
			continue
		}

		if ctx.Err() != nil {
			w.log.Info("adapter worker stopping after panic recovery", logger.Str(logger.FieldAdapterType, w.adapterType))
			return
		}
		if err := w.queue.Complete(ctx, item.ID); err != nil {
			w.log.Error("complete failed", err,
				logger.Str("adapter_type", w.adapterType),
				logger.Str(logger.FieldQueueID, item.ID),
			)
		}
		// No sleep here — the delay is enforced at ClaimNext via adapter_configs.
	}
}

// startHeartbeat ticks UpdateHeartbeat every heartbeatInterval while a scan is
// active. If a tick reports ErrQueueRowMissing the row has been reassigned by
// the scheduler's ResetStuck: the heartbeat sets the returned reassigned flag,
// cancels the scan via cancelScan, increments a metric, and stops. Returns a
// stop func (call after the scan completes) and the reassigned flag.
func (w *BaseWorker) startHeartbeat(ctx context.Context, cancelScan context.CancelFunc, id string) (func(), *atomic.Bool) {
	hbCtx, cancel := context.WithCancel(ctx)
	reassigned := &atomic.Bool{}
	go func() {
		ticker := time.NewTicker(w.heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := w.queue.UpdateHeartbeat(hbCtx, id); err != nil {
					if stderrors.Is(err, domainerrs.ErrQueueRowMissing) {
						reassigned.Store(true)
						metrics.CrawlerScanAbortedReassigned.Inc()
						w.log.Warn("heartbeat row missing — aborting scan (row reassigned)",
							logger.Str(logger.FieldQueueID, id),
						)
						cancelScan()
						return
					}
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
	return cancel, reassigned
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
