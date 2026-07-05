package services

import (
	"context"
	"sync"
	"time"

	atssvc "github.com/trushilpatel/jobradar/service/ats/services"
)

// AdapterDelayGate enforces the per-adapter-type inter-company politeness
// cooldown (ats.inter_company_delay_s) across every claim attempt handled by
// this orchestrator process, regardless of which crawler-node is polling.
//
// It is intentionally in-process rather than Postgres/Redis-backed: the
// orchestrator runs as a single replica (see apps/orchestrator/CLAUDE.md), so
// a process-local map guarded by a mutex is sufficient and avoids a second
// round-trip on every poll. AtsService already carries a 5-minute in-process
// MemoryCache (service/ats/services/AtsCacheService.go) for the
// inter_company_delay_s lookup itself — this gate does not duplicate that
// cache, it only tracks per-adapter last-claimed-at timestamps.
type AdapterDelayGate struct {
	mu        sync.Mutex
	lastClaim map[string]time.Time
	ats       atssvc.AtsService
	clock     func() time.Time
}

// NewAdapterDelayGate constructs a gate backed by ats for delay lookups.
func NewAdapterDelayGate(ats atssvc.AtsService) *AdapterDelayGate {
	return &AdapterDelayGate{
		lastClaim: make(map[string]time.Time),
		ats:       ats,
		clock:     time.Now,
	}
}

// TryReserve reports whether adapterType has cleared its inter-company delay
// window. When eligible — the adapter has never been claimed, its
// inter_company_delay_s is 0/unset, or the window has elapsed — it
// atomically marks the adapter as just-claimed under the same mutex critical
// section before returning, so two concurrent callers can never both observe
// eligible for the same adapter in the same window.
//
// A lookup failure (AtsService error, or an adapter type absent from the
// registry) is treated as NOT eligible — fail safe, never "always eligible"
// on a lookup failure — and reported as (false, nil): the anomaly is not
// surfaced as an error, so a single unknown/stale adapter_type never fails
// the caller's whole poll request.
func (g *AdapterDelayGate) TryReserve(ctx context.Context, adapterType string) (bool, error) {
	info, err := g.ats.GetATS(ctx, adapterType)
	if err != nil {
		return false, nil
	}

	delay := time.Duration(info.InterCompanyDelayS) * time.Second

	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.clock()
	if delay > 0 {
		if last, ok := g.lastClaim[adapterType]; ok && now.Sub(last) < delay {
			return false, nil
		}
	}
	g.lastClaim[adapterType] = now
	return true, nil
}
