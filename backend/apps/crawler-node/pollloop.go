package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
)

// PollLoop is the main polling loop that pulls work from the orchestrator and
// dispatches it to worker goroutines bounded by a semaphore.
type PollLoop struct {
	cfg    *Config
	client OrchestratorClient
	exec   AdapterExecutor
	log    logger.Logger

	mu      sync.Mutex
	holding []string // assignment IDs currently in progress
}

// NewPollLoop constructs a PollLoop with the given dependencies.
func NewPollLoop(cfg *Config, client OrchestratorClient, exec AdapterExecutor, log logger.Logger) *PollLoop {
	return &PollLoop{
		cfg:    cfg,
		client: client,
		exec:   exec,
		log:    log,
	}
}

// Run polls for work until ctx is cancelled. It blocks until all in-flight
// worker goroutines complete before returning, ensuring clean shutdown.
func (l *PollLoop) Run(ctx context.Context) {
	sem := make(chan struct{}, l.cfg.MaxConcurrent)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		default:
		}

		holding := l.snapshot()
		result, err := l.client.GetWork(ctx, holding)
		if err != nil {
			// Transient error — log and continue; don't exit the loop.
			l.log.Error("GetWork failed", err)
			l.sleep(ctx)
			continue
		}

		// Remove any assignments the orchestrator no longer tracks.
		if len(result.HeartbeatMissed) > 0 {
			l.removeMissed(result.HeartbeatMissed)
		}

		if result.Assignment == nil {
			// Queue empty — sleep before next poll.
			l.sleep(ctx)
			continue
		}

		unit := *result.Assignment

		// Block here if already at MAX_CONCURRENT; unblocks when a worker finishes.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}

		l.addHolding(unit.AssignmentID)
		wg.Add(1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					l.log.Error("worker panic", fmt.Errorf("panic: %v", r))
					l.reportError(unit, fmt.Sprintf("panic: %v", r))
				}
				l.removeHolding(unit.AssignmentID)
				<-sem
				wg.Done()
			}()
			// Use WithoutCancel so in-progress scans survive graceful shutdown.
			workerCtx := context.WithoutCancel(ctx)
			if execErr := l.exec.Execute(workerCtx, unit); execErr != nil {
				l.log.Error("execute failed", execErr)
				l.reportError(unit, execErr.Error())
			}
		}()
	}
}

// sleep pauses for PollIntervalSeconds unless ctx is done first.
func (l *PollLoop) sleep(ctx context.Context) {
	if l.cfg.PollIntervalSeconds <= 0 {
		return
	}
	select {
	case <-time.After(time.Duration(l.cfg.PollIntervalSeconds) * time.Second):
	case <-ctx.Done():
	}
}

func (l *PollLoop) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.holding))
	copy(out, l.holding)
	return out
}

func (l *PollLoop) addHolding(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.holding = append(l.holding, id)
}

func (l *PollLoop) removeHolding(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, v := range l.holding {
		if v == id {
			l.holding = append(l.holding[:i], l.holding[i+1:]...)
			return
		}
	}
}

func (l *PollLoop) removeMissed(missed []string) {
	missedSet := make(map[string]struct{}, len(missed))
	for _, id := range missed {
		missedSet[id] = struct{}{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	filtered := l.holding[:0]
	for _, id := range l.holding {
		if _, ok := missedSet[id]; !ok {
			filtered = append(filtered, id)
		}
	}
	l.holding = filtered
}

func (l *PollLoop) reportError(unit WorkUnit, msg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := l.client.ReportError(ctx, WorkError{
		AssignmentID: unit.AssignmentID,
		CompanyID:    unit.CompanyID,
		AdapterType:  unit.AdapterType,
		ErrorMessage: msg,
	})
	if err != nil {
		l.log.Error("ReportError failed", err)
	}
}
