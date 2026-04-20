package store

import (
	"context"
	"sync"
	"time"
)

type memEntry struct {
	reqID  string
	ts     time.Time
	tokens int
}

type memSlot struct {
	minute []memEntry          // sliding 60s window of reservations
	tpm    []memEntry          // sliding 60s window of token charges
	day    map[string]int      // utcDate -> count
	byReq  map[string]memEntry // reqID -> (ts, tokens) for Release
}

type memoryStore struct {
	mu    sync.Mutex
	slots map[string]*memSlot
}

// NewLLMRateLimitMemoryStore returns an in-memory rate-limit store suitable
// for single-process deployments and tests.
func NewLLMRateLimitMemoryStore() LLMRateLimitStore {
	return &memoryStore{slots: map[string]*memSlot{}}
}

func (s *memoryStore) get(slotID string) *memSlot {
	m, ok := s.slots[slotID]
	if !ok {
		m = &memSlot{day: map[string]int{}, byReq: map[string]memEntry{}}
		s.slots[slotID] = m
	}
	return m
}

func (s *memoryStore) evict(m *memSlot, now time.Time) {
	cutoff := now.Add(-60 * time.Second)
	m.minute = prune(m.minute, cutoff)
	m.tpm = prune(m.tpm, cutoff)
}

func prune(entries []memEntry, cutoff time.Time) []memEntry {
	out := entries[:0]
	for _, e := range entries {
		if e.ts.After(cutoff) {
			out = append(out, e)
		}
	}
	return out
}

func (s *memoryStore) Reserve(_ context.Context, r ReserveReq) (ReserveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	m := s.get(r.SlotID)
	s.evict(m, now)

	if r.RPM > 0 && len(m.minute) >= r.RPM {
		return ReserveDeniedRPM, nil
	}
	day := now.Format("2006-01-02")
	if r.RPD > 0 && m.day[day] >= r.RPD {
		return ReserveDeniedRPD, nil
	}
	if r.TPMIn > 0 {
		sum := 0
		for _, e := range m.tpm {
			sum += e.tokens
		}
		if sum+r.Tokens > r.TPMIn {
			return ReserveDeniedTPM, nil
		}
	}

	m.minute = append(m.minute, memEntry{reqID: r.ReqID, ts: now})
	if r.Tokens > 0 {
		m.tpm = append(m.tpm, memEntry{reqID: r.ReqID, ts: now, tokens: r.Tokens})
	}
	m.day[day]++
	m.byReq[r.ReqID] = memEntry{reqID: r.ReqID, ts: now, tokens: r.Tokens}
	return ReserveOK, nil
}

func (s *memoryStore) Release(_ context.Context, slotID, reqID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.slots[slotID]
	if !ok {
		return nil
	}
	e, ok := m.byReq[reqID]
	if !ok {
		return nil
	}
	delete(m.byReq, reqID)

	// Drop the minute entry and (if tokens > 0) the tpm entry that match this reqID.
	for i, x := range m.minute {
		if x.reqID == reqID {
			m.minute = append(m.minute[:i], m.minute[i+1:]...)
			break
		}
	}
	if e.tokens > 0 {
		for i, x := range m.tpm {
			if x.reqID == reqID {
				m.tpm = append(m.tpm[:i], m.tpm[i+1:]...)
				break
			}
		}
	}
	day := e.ts.Format("2006-01-02")
	if m.day[day] > 0 {
		m.day[day]--
	}
	return nil
}

func (s *memoryStore) Usage(_ context.Context, slotID string) (Usage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.slots[slotID]
	if !ok {
		return Usage{}, nil
	}
	now := time.Now().UTC()
	s.evict(m, now)
	sum := 0
	for _, e := range m.tpm {
		sum += e.tokens
	}
	return Usage{RPM: len(m.minute), RPD: m.day[now.Format("2006-01-02")], TPM: sum}, nil
}
