package store

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"
)

type cooldownMemoryStore struct {
	mu     sync.Mutex
	state  map[string]CooldownStatus
	policy CooldownPolicy
	rand   *rand.Rand
}

// NewLLMCooldownMemoryStore returns an in-memory LLMCooldownStore suitable
// for tests and single-process deployments that can tolerate state loss.
func NewLLMCooldownMemoryStore(p CooldownPolicy) LLMCooldownStore {
	return &cooldownMemoryStore{
		state:  map[string]CooldownStatus{},
		policy: p,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (c *cooldownMemoryStore) IsAvailable(_ context.Context, slotID string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.state[slotID]
	if !ok {
		return true, nil
	}
	return time.Now().After(s.CooldownUntil), nil
}

func (c *cooldownMemoryStore) RecordSuccess(_ context.Context, slotID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.state, slotID)
	return nil
}

func (c *cooldownMemoryStore) RecordFailure(_ context.Context, slotID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	s := c.state[slotID]
	s.FailureCount++
	s.CooldownUntil = time.Now().Add(c.backoff(s.FailureCount))
	c.state[slotID] = s
	return nil
}

func (c *cooldownMemoryStore) Status(_ context.Context, slotID string) (CooldownStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state[slotID], nil
}

func (c *cooldownMemoryStore) backoff(n int) time.Duration {
	return computeBackoff(c.policy, n, c.rand.Float64())
}

// computeBackoff is exposed as a free function so the Redis store can share
// the exact same math without duplicating edge cases.
func computeBackoff(p CooldownPolicy, n int, jitterRoll float64) time.Duration {
	if n < 1 {
		n = 1
	}
	// Guard against overflow: 1<<62 * base overflows; cap shift at 30.
	shift := n - 1
	if shift > 30 {
		shift = 30
	}
	dur := time.Duration(float64(p.Base) * math.Pow(2, float64(shift)))
	if dur <= 0 {
		dur = p.Max
	}
	dur = min(dur, p.Max)
	if p.Jitter > 0 {
		j := (jitterRoll*2 - 1) * p.Jitter
		dur = time.Duration(float64(dur) * (1 + j))
	}
	if dur <= 0 {
		dur = p.Base
	}
	return dur
}
