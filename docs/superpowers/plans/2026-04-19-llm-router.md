# LLM Router — Multi-slot rate limiting + cooldown

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route every Gemini call through a slot-aware router that enforces per-slot RPM / RPD / TPMIn via Redis (atomic Lua reserve), applies exponential cooldown on provider 429, and falls back across configured (key, model) slots before returning `ErrLLMAllExhausted`.

**Architecture:** Sits inside `service/llm/`. Existing `Scorer` / `JobsRelevanceFilter` / `JobPostEnricher` contracts stay unchanged. Each configured `(keyID, model)` pair becomes a **slot** backed by its own `geminiScorer` instance wrapped in `LLMRateLimitService`; all slots are collected behind `LLMRouterService` which implements `Scorer`. A `CooldownStore` tracks failure state per slot. Both stores are Redis-backed with an in-memory impl for tests.

**Tech Stack:** Go 1.25, `google/genai`, `redis/go-redis/v9` (new dep), `alicebob/miniredis/v2` (test dep), existing `common/logger` + `common/errors` + `common/metrics`.

**No git commits** — the user commits explicitly. Tasks end with staging only.

---

## File structure

**New:**
- `backend/common/redis/client.go` — Redis client bootstrap
- `backend/common/redis/CLAUDE.md` — package rules
- `backend/service/llm/dto/SlotStatus.go` — per-slot exhaustion detail
- `backend/service/llm/dto/CompleteOutcome.go` — outcome enum used by metrics
- `backend/service/llm/common/slots.go` — slot ID format, outcome constants, reserve-denial reason constants
- `backend/service/llm/store/LLMRateLimitStore.go` — interface + `ReserveResult` type
- `backend/service/llm/store/LLMRateLimitRedisStore.go` — Redis impl w/ Lua
- `backend/service/llm/store/LLMRateLimitMemoryStore.go` — in-memory impl (for tests)
- `backend/service/llm/store/reserve.lua` — atomic RPM+RPD+TPM Lua script (embedded via `//go:embed`)
- `backend/service/llm/store/LLMCooldownStore.go` — interface
- `backend/service/llm/store/LLMCooldownRedisStore.go` — Redis impl
- `backend/service/llm/store/LLMCooldownMemoryStore.go` — in-memory impl
- `backend/service/llm/services/LLMRateLimitService.go` — per-slot decorator (replaces old `LLMImplService`)
- `backend/service/llm/services/LLMRouterService.go` — multi-slot router
- `backend/service/llm/services/tests/LLMRateLimitService_test.go`
- `backend/service/llm/services/tests/LLMRouterService_test.go`
- `backend/service/llm/store/tests/LLMRateLimitStore_test.go`
- `backend/service/llm/store/tests/LLMCooldownStore_test.go`

**Modified:**
- `backend/common/errors/errors.go` — add 4 LLM sentinels + `AllExhaustedError`
- `backend/common/config/config.go` — multi-slot LLM config (`LLMSlotConfig`, `CooldownConfig`, `RedisConfig`)
- `backend/common/config/CLAUDE.md` — doc new env vars
- `backend/service/llm/services/LLMGeminiService.go` — map Gemini 429 → `ErrLLMProviderRateLimited`
- `backend/service/llm/services/LLMFactory.go` — build N slots, wrap, collect into router
- `backend/service/llm/common/constants.go` — add `ProviderGemini` if not present; reserve-denial codes
- `backend/service/llm/CLAUDE.md` — full design doc rewrite
- `backend/apps/filter/main.go` (or its wiring file) — use new factory signature (no API change expected; internal change)
- `backend/apps/analyzer/main.go` (or its wiring file) — same
- `docker-compose.yml` — add `redis:7-alpine` service

**Deleted:**
- `backend/service/llm/services/LLMImplService.go` — superseded by per-slot `LLMRateLimitService`

---

## Task 1 — Add Redis client + dependency

**Files:**
- Create: `backend/common/redis/client.go`
- Create: `backend/common/redis/CLAUDE.md`
- Modify: `backend/go.mod`, `backend/go.sum`

- [ ] **Step 1: Add dependency**

```bash
cd backend && go get github.com/redis/go-redis/v9@latest && go get github.com/alicebob/miniredis/v2@latest
```

- [ ] **Step 2: Create `backend/common/redis/client.go`**

```go
package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

type Client = goredis.Client

// New builds a Redis client and pings it so startup fails fast on misconfig.
func New(ctx context.Context, addr string, db int) (*Client, error) {
	c := goredis.NewClient(&goredis.Options{Addr: addr, DB: db})
	if err := c.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis: ping %s: %w", addr, err)
	}
	return c, nil
}
```

- [ ] **Step 3: Create `backend/common/redis/CLAUDE.md`**

```markdown
# redis/ — Redis client bootstrap

Single-purpose package: wraps `go-redis/v9` with a ping-on-connect `New(ctx, addr, db)`.

Used by `service/llm/store/` for per-slot rate-limit + cooldown state. No other package in the codebase should construct its own Redis client.
```

- [ ] **Step 4: Verify build**

```bash
cd backend && go build ./...
```

Expected: success.

---

## Task 2 — Add Redis config

**Files:**
- Modify: `backend/common/config/config.go`
- Modify: `backend/common/config/CLAUDE.md`

- [ ] **Step 1: Add `RedisConfig` + `CooldownConfig` types to `config.go`**

Insert after `type ObservabilityConfig` declaration:

```go
type RedisConfig struct {
	Addr string
	DB   int
}

type CooldownConfig struct {
	BaseSeconds int
	MaxSeconds  int
	Jitter      float64 // 0.0 – 1.0
}
```

Add `Redis RedisConfig` and `Cooldown CooldownConfig` fields to the top-level `Config` struct.

- [ ] **Step 2: Load from env in `Load()`**

Add inside the returned `&Config{...}`:

```go
Redis: RedisConfig{
	Addr: getEnv("REDIS_ADDR", "localhost:6379"),
	DB:   getEnvInt("REDIS_DB", 0),
},
Cooldown: CooldownConfig{
	BaseSeconds: getEnvInt("LLM_COOLDOWN_BASE_SECONDS", 60),
	MaxSeconds:  getEnvInt("LLM_COOLDOWN_MAX_SECONDS", 3600),
	Jitter:      getEnvFloat("LLM_COOLDOWN_JITTER", 0.2),
},
```

- [ ] **Step 3: Add `getEnvFloat` helper**

```go
func getEnvFloat(key string, fallback float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return fallback
}
```

- [ ] **Step 4: Update `common/config/CLAUDE.md`**

Add sections:

```markdown
### Redis (LLM rate limiting + cooldown)

| Env var     | Default          | Notes                             |
| ----------- | ---------------- | --------------------------------- |
| `REDIS_ADDR`| `localhost:6379` | host:port                          |
| `REDIS_DB`  | `0`              | logical database number            |

### LLM cooldown

| Env var                     | Default | Notes                                |
| --------------------------- | ------- | ------------------------------------ |
| `LLM_COOLDOWN_BASE_SECONDS` | `60`    | Base backoff duration                 |
| `LLM_COOLDOWN_MAX_SECONDS`  | `3600`  | Cap on backoff                        |
| `LLM_COOLDOWN_JITTER`       | `0.2`   | ± fraction, must be in [0, 1]         |
```

- [ ] **Step 5: Verify build** — `cd backend && go build ./...`

---

## Task 3 — Add multi-slot LLM config

**Files:**
- Modify: `backend/common/config/config.go`
- Modify: `backend/common/config/CLAUDE.md`

Behaviour: keep the existing `FilterConfig.Model` / `EnricherConfig.Model` fields for logging but stop using their single-provider `RateLimits`. Slots are declared via one JSON env var per worker so rollout is non-breaking when the var is empty (factory falls back to single-slot built from legacy fields).

- [ ] **Step 1: Add slot types**

```go
type LLMSlotConfig struct {
	KeyID  string `json:"key_id"`
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
	RPM    int    `json:"rpm"`
	RPD    int    `json:"rpd"`
	TPMIn  int    `json:"tpm_in"`
}

// ParseLLMSlots parses a JSON array of slot configs from a single env var.
// Returns nil on empty input (not an error — enables legacy single-slot mode).
func ParseLLMSlots(raw string) ([]LLMSlotConfig, error) {
	if raw == "" {
		return nil, nil
	}
	var slots []LLMSlotConfig
	if err := json.Unmarshal([]byte(raw), &slots); err != nil {
		return nil, fmt.Errorf("config: parse LLM slots: %w", err)
	}
	return slots, nil
}
```

Add `encoding/json` to imports.

- [ ] **Step 2: Add `Slots []LLMSlotConfig` to `FilterConfig` and `EnricherConfig`**

- [ ] **Step 3: Load from env in `Load()`**

For `FilterConfig`:

```go
slotsRaw := getEnv("FILTER_LLM_SLOTS", "")
filterSlots, err := ParseLLMSlots(slotsRaw)
if err != nil {
	fmt.Fprintf(os.Stderr, "config: %v — falling back to single slot\n", err)
}
// ... populate FilterConfig.Slots: filterSlots
```

Same for enricher using `ENRICHER_LLM_SLOTS`. Refactor `Load()` to compute these before constructing the `Config` literal.

- [ ] **Step 4: Update CLAUDE.md with the new env var**

```markdown
| `FILTER_LLM_SLOTS`   | `""`  | JSON array of `{key_id, api_key, model, rpm, rpd, tpm_in}`. If set, overrides single-slot config. |
| `ENRICHER_LLM_SLOTS` | `""`  | Same shape as `FILTER_LLM_SLOTS`.                                                                 |
```

- [ ] **Step 5: Verify build**

---

## Task 4 — LLM sentinel errors

**Files:**
- Modify: `backend/common/errors/errors.go`

- [ ] **Step 1: Append the new sentinels + exhaustion error**

```go
var (
	ErrLLMValidation         = &ServiceError{Code: "LLM_VALIDATION", Status: 400}
	ErrLLMLocalQuotaExceeded = &ServiceError{Code: "LLM_LOCAL_QUOTA", Status: 429}
	ErrLLMProviderRateLimited = &ServiceError{Code: "LLM_PROVIDER_429", Status: 429}
	ErrLLMAllExhausted       = &ServiceError{Code: "LLM_EXHAUSTED", Status: 503}
)

// LLMAllExhaustedError carries per-slot exhaustion detail. Unwraps to ErrLLMAllExhausted.
type LLMAllExhaustedError struct {
	Slots      []LLMSlotStatus
	RetryAfter time.Time
}

func (e *LLMAllExhaustedError) Error() string { return ErrLLMAllExhausted.Message }
func (e *LLMAllExhaustedError) Unwrap() error { return ErrLLMAllExhausted }

// LLMSlotStatus is the public shape of slot state at exhaustion time.
type LLMSlotStatus struct {
	SlotID        string
	Model         string
	CooldownUntil time.Time
	FailureCount  int
	LastDenial    string // "rpm" | "rpd" | "tpm" | "cooldown" | "provider_429"
}
```

Add `time` to imports. Set `.Message` on each sentinel to a short human string (`"llm: validation failed"`, etc.) so `.Error()` returns something non-empty.

- [ ] **Step 2: Verify build** — `cd backend && go build ./...`

---

## Task 5 — LLM common constants

**Files:**
- Modify: `backend/service/llm/common/constants.go`
- Create: `backend/service/llm/common/slots.go`

- [ ] **Step 1: Create `slots.go`**

```go
package common

import "fmt"

const (
	ProviderGemini = "gemini"
)

// Outcome labels used by metrics and logging.
const (
	OutcomeOK              = "ok"
	OutcomeLocalQuotaRPM   = "local_quota_rpm"
	OutcomeLocalQuotaRPD   = "local_quota_rpd"
	OutcomeLocalQuotaTPM   = "local_quota_tpm"
	OutcomeProvider429     = "provider_429"
	OutcomeOtherError      = "other_error"
	OutcomeCooldownSkipped = "cooldown_skipped"
)

// SlotID formats a stable ID used across Redis keys, logs, metrics.
func SlotID(provider, keyID, model string) string {
	return fmt.Sprintf("%s:%s:%s", provider, keyID, model)
}
```

- [ ] **Step 2: Verify build**

---

## Task 6 — DTOs

**Files:**
- Create: `backend/service/llm/dto/SlotStatus.go`

- [ ] **Step 1: Create `SlotStatus.go`**

```go
package dto

// SlotStatus is the snapshot router emits when recording an attempt outcome.
// Exposed to tests and to the exhaustion error translation layer.
type SlotStatus struct {
	SlotID        string
	Provider      string
	KeyID         string
	Model         string
	RPMUsage      int
	RPMLimit      int
	RPDUsage      int
	RPDLimit      int
	TPMUsage      int
	TPMLimit      int
	FailureCount  int
	CooldownUntil int64 // unix millis; 0 = not in cooldown
	LastDenial    string
}
```

- [ ] **Step 2: Verify build**

---

## Task 7 — RateLimit store interface + memory impl

**Files:**
- Create: `backend/service/llm/store/LLMRateLimitStore.go`
- Create: `backend/service/llm/store/LLMRateLimitMemoryStore.go`
- Create: `backend/service/llm/store/tests/LLMRateLimitStore_test.go`

- [ ] **Step 1: Write failing tests in `tests/LLMRateLimitStore_test.go`**

```go
package tests

import (
	"context"
	"testing"

	"github.com/trushilpatel/jobradar/service/llm/store"
)

func TestMemoryStore_RPMBoundary(t *testing.T) {
	s := store.NewLLMRateLimitMemoryStore()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 3, RPD: 100, TPMIn: 0, ReqID: id(i), Tokens: 0})
		mustOK(t, r, err)
	}
	r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 3, RPD: 100, ReqID: "x"})
	if err != nil { t.Fatal(err) }
	if r != store.ReserveDeniedRPM {
		t.Fatalf("want ReserveDeniedRPM, got %v", r)
	}
}

func TestMemoryStore_RPDBoundary(t *testing.T) { /* similar — RPD=2, RPM=100 */ }

func TestMemoryStore_TPMBoundary(t *testing.T) {
	s := store.NewLLMRateLimitMemoryStore()
	ctx := context.Background()
	r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 100, RPD: 100, TPMIn: 1000, ReqID: "a", Tokens: 600})
	mustOK(t, r, err)
	r, err = s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 100, RPD: 100, TPMIn: 1000, ReqID: "b", Tokens: 500})
	if r != store.ReserveDeniedTPM { t.Fatalf("want TPM denial, got %v", r) }
}

func TestMemoryStore_Release(t *testing.T) {
	s := store.NewLLMRateLimitMemoryStore()
	ctx := context.Background()
	_, _ = s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 1, RPD: 100, ReqID: "a"})
	if err := s.Release(ctx, "s1", "a"); err != nil { t.Fatal(err) }
	r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 1, RPD: 100, ReqID: "b"})
	mustOK(t, r, err)
}

func mustOK(t *testing.T, r store.ReserveResult, err error) {
	t.Helper()
	if err != nil { t.Fatal(err) }
	if r != store.ReserveOK { t.Fatalf("want ReserveOK, got %v", r) }
}

func id(i int) string { return []string{"a","b","c","d","e"}[i] }
```

- [ ] **Step 2: Run tests — expect FAIL (store not defined)**

```bash
cd backend && go test ./service/llm/store/tests/...
```

- [ ] **Step 3: Create `LLMRateLimitStore.go` with interface**

```go
package store

import "context"

type ReserveResult int

const (
	ReserveOK ReserveResult = iota
	ReserveDeniedRPM
	ReserveDeniedRPD
	ReserveDeniedTPM
)

type ReserveReq struct {
	SlotID string
	ReqID  string
	RPM    int
	RPD    int
	TPMIn  int
	Tokens int // estimated input tokens
}

type Usage struct {
	RPM   int
	RPD   int
	TPM   int
}

type LLMRateLimitStore interface {
	Reserve(ctx context.Context, req ReserveReq) (ReserveResult, error)
	Release(ctx context.Context, slotID, reqID string) error
	Usage(ctx context.Context, slotID string) (Usage, error)
}
```

- [ ] **Step 4: Create `LLMRateLimitMemoryStore.go`**

```go
package store

import (
	"context"
	"sync"
	"time"
)

type memEntry struct {
	ts     time.Time
	tokens int
}

type memSlot struct {
	minute []memEntry          // sliding 60s
	tpm    []memEntry          // sliding 60s
	day    map[string]int       // utcDate → count
	byReq  map[string]memEntry  // reqID → (ts, tokens) — for Release
}

type memoryStore struct {
	mu    sync.Mutex
	slots map[string]*memSlot
}

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

func (s *memoryStore) Reserve(ctx context.Context, r ReserveReq) (ReserveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	m := s.get(r.SlotID)
	s.evict(m, now)

	if r.RPM > 0 && len(m.minute) >= r.RPM { return ReserveDeniedRPM, nil }
	day := now.Format("2006-01-02")
	if r.RPD > 0 && m.day[day] >= r.RPD { return ReserveDeniedRPD, nil }
	if r.TPMIn > 0 {
		sum := 0
		for _, e := range m.tpm { sum += e.tokens }
		if sum+r.Tokens > r.TPMIn { return ReserveDeniedTPM, nil }
	}

	m.minute = append(m.minute, memEntry{ts: now})
	if r.Tokens > 0 { m.tpm = append(m.tpm, memEntry{ts: now, tokens: r.Tokens}) }
	m.day[day]++
	m.byReq[r.ReqID] = memEntry{ts: now, tokens: r.Tokens}
	return ReserveOK, nil
}

func (s *memoryStore) Release(ctx context.Context, slotID, reqID string) error {
	s.mu.Lock(); defer s.mu.Unlock()
	m, ok := s.slots[slotID]
	if !ok { return nil }
	e, ok := m.byReq[reqID]
	if !ok { return nil }
	delete(m.byReq, reqID)

	// Drop one matching minute entry + tpm entry.
	for i, x := range m.minute {
		if x.ts.Equal(e.ts) { m.minute = append(m.minute[:i], m.minute[i+1:]...); break }
	}
	if e.tokens > 0 {
		for i, x := range m.tpm {
			if x.ts.Equal(e.ts) && x.tokens == e.tokens {
				m.tpm = append(m.tpm[:i], m.tpm[i+1:]...); break
			}
		}
	}
	day := e.ts.Format("2006-01-02")
	if m.day[day] > 0 { m.day[day]-- }
	return nil
}

func (s *memoryStore) Usage(ctx context.Context, slotID string) (Usage, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	m, ok := s.slots[slotID]
	if !ok { return Usage{}, nil }
	s.evict(m, time.Now().UTC())
	sum := 0
	for _, e := range m.tpm { sum += e.tokens }
	return Usage{RPM: len(m.minute), RPD: m.day[time.Now().UTC().Format("2006-01-02")], TPM: sum}, nil
}
```

- [ ] **Step 5: Run tests — expect PASS**

```bash
cd backend && go test ./service/llm/store/tests/... -run TestMemoryStore -v
```

---

## Task 8 — Redis rate-limit store with Lua

**Files:**
- Create: `backend/service/llm/store/reserve.lua`
- Create: `backend/service/llm/store/LLMRateLimitRedisStore.go`
- Modify: `backend/service/llm/store/tests/LLMRateLimitStore_test.go` — add Redis subtests

- [ ] **Step 1: Create `reserve.lua`**

```lua
-- KEYS[1] = ratelimit:{slotID}:minute          (ZSET)
-- KEYS[2] = ratelimit:{slotID}:day:{UTCDATE}   (INT)
-- KEYS[3] = ratelimit:{slotID}:tpm:window      (ZSET)
-- KEYS[4] = ratelimit:{slotID}:tpm:tokens      (HASH)
-- KEYS[5] = ratelimit:{slotID}:tpm:sum         (INT)
-- ARGV[1] = now_millis
-- ARGV[2] = rpm_limit        (0 = unlimited)
-- ARGV[3] = rpd_limit        (0 = unlimited)
-- ARGV[4] = tpm_limit        (0 = unlimited)
-- ARGV[5] = tokens           (estimated input tokens for this call)
-- ARGV[6] = reqID
-- ARGV[7] = day_expire_millis (end-of-UTC-day + grace)
-- Returns: 0 OK | 1 RPM | 2 RPD | 3 TPM

local now    = tonumber(ARGV[1])
local rpm    = tonumber(ARGV[2])
local rpd    = tonumber(ARGV[3])
local tpm    = tonumber(ARGV[4])
local tokens = tonumber(ARGV[5])
local req    = ARGV[6]
local exp    = tonumber(ARGV[7])

-- Evict minute + tpm entries older than 60s.
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - 60000)
local evicted = redis.call('ZRANGEBYSCORE', KEYS[3], '-inf', now - 60000)
if #evicted > 0 then
    for _, rid in ipairs(evicted) do
        local t = redis.call('HGET', KEYS[4], rid)
        if t then
            redis.call('DECRBY', KEYS[5], tonumber(t))
            redis.call('HDEL', KEYS[4], rid)
        end
    end
    redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', now - 60000)
end

-- Ensure sum never goes negative (defensive for crash recovery).
local sum = tonumber(redis.call('GET', KEYS[5]) or '0')
if sum < 0 then
    redis.call('SET', KEYS[5], 0)
    sum = 0
end

if rpm > 0 then
    local n = redis.call('ZCARD', KEYS[1])
    if n >= rpm then return 1 end
end

if rpd > 0 then
    local d = tonumber(redis.call('GET', KEYS[2]) or '0')
    if d >= rpd then return 2 end
end

if tpm > 0 and (sum + tokens) > tpm then
    return 3
end

redis.call('ZADD', KEYS[1], now, req)
redis.call('EXPIRE', KEYS[1], 120)

if tokens > 0 then
    redis.call('ZADD', KEYS[3], now, req)
    redis.call('HSET', KEYS[4], req, tokens)
    redis.call('INCRBY', KEYS[5], tokens)
    redis.call('EXPIRE', KEYS[3], 120)
    redis.call('EXPIRE', KEYS[4], 120)
    redis.call('EXPIRE', KEYS[5], 120)
end

redis.call('INCR', KEYS[2])
redis.call('PEXPIREAT', KEYS[2], exp)
return 0
```

- [ ] **Step 2: Create `LLMRateLimitRedisStore.go`**

```go
package store

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

//go:embed reserve.lua
var reserveLua string

var reserveScript = goredis.NewScript(reserveLua)

type redisStore struct {
	rdb *goredis.Client
}

func NewLLMRateLimitRedisStore(rdb *goredis.Client) LLMRateLimitStore {
	return &redisStore{rdb: rdb}
}

func (s *redisStore) keys(slotID string, day string) []string {
	return []string{
		fmt.Sprintf("ratelimit:%s:minute", slotID),
		fmt.Sprintf("ratelimit:%s:day:%s", slotID, day),
		fmt.Sprintf("ratelimit:%s:tpm:window", slotID),
		fmt.Sprintf("ratelimit:%s:tpm:tokens", slotID),
		fmt.Sprintf("ratelimit:%s:tpm:sum", slotID),
	}
}

func endOfUTCDayMillis(now time.Time) int64 {
	y, m, d := now.UTC().Date()
	end := time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC).Add(time.Hour)
	return end.UnixMilli()
}

func (s *redisStore) Reserve(ctx context.Context, r ReserveReq) (ReserveResult, error) {
	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	keys := s.keys(r.SlotID, day)
	args := []any{
		now.UnixMilli(), r.RPM, r.RPD, r.TPMIn, r.Tokens, r.ReqID, endOfUTCDayMillis(now),
	}
	raw, err := reserveScript.Run(ctx, s.rdb, keys, args...).Int()
	if err != nil {
		return 0, fmt.Errorf("ratelimit redis reserve: %w", err)
	}
	return ReserveResult(raw), nil
}

func (s *redisStore) Release(ctx context.Context, slotID, reqID string) error {
	day := time.Now().UTC().Format("2006-01-02")
	keys := s.keys(slotID, day)

	pipe := s.rdb.Pipeline()
	pipe.ZRem(ctx, keys[0], reqID)
	tokenCmd := pipe.HGet(ctx, keys[3], reqID)
	if _, err := pipe.Exec(ctx); err != nil && err != goredis.Nil {
		return fmt.Errorf("ratelimit redis release lookup: %w", err)
	}
	tokensStr, _ := tokenCmd.Result()

	pipe = s.rdb.Pipeline()
	pipe.ZRem(ctx, keys[2], reqID)
	pipe.HDel(ctx, keys[3], reqID)
	if tokensStr != "" {
		var n int
		fmt.Sscanf(tokensStr, "%d", &n)
		if n > 0 { pipe.DecrBy(ctx, keys[4], int64(n)) }
	}
	pipe.Decr(ctx, keys[1])
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("ratelimit redis release apply: %w", err)
	}
	return nil
}

func (s *redisStore) Usage(ctx context.Context, slotID string) (Usage, error) {
	day := time.Now().UTC().Format("2006-01-02")
	keys := s.keys(slotID, day)

	pipe := s.rdb.Pipeline()
	rpm := pipe.ZCard(ctx, keys[0])
	rpd := pipe.Get(ctx, keys[1])
	tpm := pipe.Get(ctx, keys[4])
	if _, err := pipe.Exec(ctx); err != nil && err != goredis.Nil {
		return Usage{}, fmt.Errorf("ratelimit redis usage: %w", err)
	}
	u := Usage{}
	u.RPM = int(rpm.Val())
	if v, err := rpd.Int(); err == nil { u.RPD = v }
	if v, err := tpm.Int(); err == nil { u.TPM = v }
	return u, nil
}
```

- [ ] **Step 3: Add miniredis-backed tests to `tests/LLMRateLimitStore_test.go`**

```go
import (
	"testing"
	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func newRedisStore(t *testing.T) (store.LLMRateLimitStore, func()) {
	mr, err := miniredis.Run()
	if err != nil { t.Fatal(err) }
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return store.NewLLMRateLimitRedisStore(rdb), func() { _ = rdb.Close(); mr.Close() }
}

func TestRedisStore_RPMBoundary(t *testing.T) {
	s, cleanup := newRedisStore(t); defer cleanup()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		r, err := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 3, RPD: 100, ReqID: fmt.Sprintf("r%d", i)})
		mustOK(t, r, err)
	}
	r, _ := s.Reserve(ctx, store.ReserveReq{SlotID: "s1", RPM: 3, RPD: 100, ReqID: "x"})
	if r != store.ReserveDeniedRPM { t.Fatalf("want RPM denial, got %v", r) }
}

// + TestRedisStore_RPDBoundary, TestRedisStore_TPMBoundary, TestRedisStore_Release — mirror memory tests
```

- [ ] **Step 4: Run tests — `cd backend && go test ./service/llm/store/tests/... -v`** — expect PASS

---

## Task 9 — Cooldown store (interface + memory + Redis)

**Files:**
- Create: `backend/service/llm/store/LLMCooldownStore.go`
- Create: `backend/service/llm/store/LLMCooldownMemoryStore.go`
- Create: `backend/service/llm/store/LLMCooldownRedisStore.go`
- Create: `backend/service/llm/store/tests/LLMCooldownStore_test.go`

- [ ] **Step 1: Write failing tests**

```go
package tests

import (
	"context"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/service/llm/store"
)

func newCooldown() store.LLMCooldownStore {
	return store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: 60 * time.Second, Max: 3600 * time.Second, Jitter: 0})
}

func TestCooldown_InitiallyAvailable(t *testing.T) {
	s := newCooldown()
	ok, _ := s.IsAvailable(context.Background(), "s1")
	if !ok { t.Fatal("should be available initially") }
}

func TestCooldown_RecordFailureProgression(t *testing.T) {
	s := newCooldown()
	ctx := context.Background()
	_ = s.RecordFailure(ctx, "s1") // cooldown 60s
	ok, _ := s.IsAvailable(ctx, "s1")
	if ok { t.Fatal("should be in cooldown after first failure") }
	st, _ := s.Status(ctx, "s1")
	if st.FailureCount != 1 { t.Fatalf("want count=1, got %d", st.FailureCount) }
}

func TestCooldown_RecordSuccessResets(t *testing.T) {
	s := newCooldown()
	ctx := context.Background()
	_ = s.RecordFailure(ctx, "s1")
	_ = s.RecordSuccess(ctx, "s1")
	st, _ := s.Status(ctx, "s1")
	if st.FailureCount != 0 { t.Fatalf("want reset, got count=%d", st.FailureCount) }
	ok, _ := s.IsAvailable(ctx, "s1")
	if !ok { t.Fatal("should be available after success") }
}
```

- [ ] **Step 2: Create interface**

```go
package store

import (
	"context"
	"time"
)

type CooldownStatus struct {
	FailureCount  int
	CooldownUntil time.Time
}

type CooldownPolicy struct {
	Base   time.Duration
	Max    time.Duration
	Jitter float64 // [-Jitter, +Jitter]
}

type LLMCooldownStore interface {
	IsAvailable(ctx context.Context, slotID string) (bool, error)
	RecordSuccess(ctx context.Context, slotID string) error
	RecordFailure(ctx context.Context, slotID string) error
	Status(ctx context.Context, slotID string) (CooldownStatus, error)
}
```

- [ ] **Step 3: Create memory impl**

```go
type cooldownMem struct {
	mu     sync.Mutex
	state  map[string]CooldownStatus
	policy CooldownPolicy
	rand   *rand.Rand
}

func NewLLMCooldownMemoryStore(p CooldownPolicy) LLMCooldownStore {
	return &cooldownMem{state: map[string]CooldownStatus{}, policy: p, rand: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

func (c *cooldownMem) IsAvailable(_ context.Context, id string) (bool, error) {
	c.mu.Lock(); defer c.mu.Unlock()
	s, ok := c.state[id]
	if !ok { return true, nil }
	return time.Now().After(s.CooldownUntil), nil
}

func (c *cooldownMem) RecordSuccess(_ context.Context, id string) error {
	c.mu.Lock(); defer c.mu.Unlock()
	delete(c.state, id); return nil
}

func (c *cooldownMem) RecordFailure(_ context.Context, id string) error {
	c.mu.Lock(); defer c.mu.Unlock()
	s := c.state[id]
	s.FailureCount++
	dur := c.policy.Base * time.Duration(1<<uint(s.FailureCount-1))
	if dur > c.policy.Max { dur = c.policy.Max }
	if c.policy.Jitter > 0 {
		j := (c.rand.Float64()*2 - 1) * c.policy.Jitter
		dur = time.Duration(float64(dur) * (1 + j))
	}
	s.CooldownUntil = time.Now().Add(dur)
	c.state[id] = s
	return nil
}

func (c *cooldownMem) Status(_ context.Context, id string) (CooldownStatus, error) {
	c.mu.Lock(); defer c.mu.Unlock()
	return c.state[id], nil
}
```

Add imports: `context`, `math/rand`, `sync`, `time`.

- [ ] **Step 4: Create Redis impl**

Keys: `cooldown:{slotID}:failure_count`, `cooldown:{slotID}:cooldown_until` (unix millis).

```go
type cooldownRedis struct {
	rdb    *goredis.Client
	policy CooldownPolicy
}

func NewLLMCooldownRedisStore(rdb *goredis.Client, p CooldownPolicy) LLMCooldownStore {
	return &cooldownRedis{rdb: rdb, policy: p}
}

func (c *cooldownRedis) IsAvailable(ctx context.Context, id string) (bool, error) {
	until, err := c.rdb.Get(ctx, fmt.Sprintf("cooldown:%s:cooldown_until", id)).Int64()
	if err == goredis.Nil { return true, nil }
	if err != nil { return false, fmt.Errorf("cooldown get: %w", err) }
	return time.Now().UnixMilli() > until, nil
}

func (c *cooldownRedis) RecordSuccess(ctx context.Context, id string) error {
	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, fmt.Sprintf("cooldown:%s:cooldown_until", id))
	pipe.Set(ctx, fmt.Sprintf("cooldown:%s:failure_count", id), 0, time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *cooldownRedis) RecordFailure(ctx context.Context, id string) error {
	countKey := fmt.Sprintf("cooldown:%s:failure_count", id)
	untilKey := fmt.Sprintf("cooldown:%s:cooldown_until", id)
	n, err := c.rdb.Incr(ctx, countKey).Result()
	if err != nil { return fmt.Errorf("cooldown incr: %w", err) }

	dur := c.policy.Base * time.Duration(1<<uint(n-1))
	if dur > c.policy.Max || dur <= 0 { dur = c.policy.Max }
	if c.policy.Jitter > 0 {
		j := (rand.Float64()*2 - 1) * c.policy.Jitter
		dur = time.Duration(float64(dur) * (1 + j))
	}
	until := time.Now().Add(dur)

	pipe := c.rdb.Pipeline()
	pipe.Set(ctx, untilKey, until.UnixMilli(), dur+time.Hour)
	pipe.ExpireAt(ctx, countKey, until.Add(time.Hour))
	_, err = pipe.Exec(ctx)
	return err
}

func (c *cooldownRedis) Status(ctx context.Context, id string) (CooldownStatus, error) {
	pipe := c.rdb.Pipeline()
	countCmd := pipe.Get(ctx, fmt.Sprintf("cooldown:%s:failure_count", id))
	untilCmd := pipe.Get(ctx, fmt.Sprintf("cooldown:%s:cooldown_until", id))
	if _, err := pipe.Exec(ctx); err != nil && err != goredis.Nil {
		return CooldownStatus{}, err
	}
	s := CooldownStatus{}
	if v, err := countCmd.Int(); err == nil { s.FailureCount = v }
	if v, err := untilCmd.Int64(); err == nil { s.CooldownUntil = time.UnixMilli(v) }
	return s, nil
}
```

- [ ] **Step 5: Run tests** — expect PASS for memory; add miniredis Redis variant mirroring memory tests.

---

## Task 10 — RateLimit decorator

**Files:**
- Create: `backend/service/llm/services/LLMRateLimitService.go`
- Create: `backend/service/llm/services/tests/LLMRateLimitService_test.go`

Contract: implements `Scorer`. Wraps one `Scorer` (the per-slot `geminiScorer`). Before `ScoreBatch`/`Enrich`: reserve; map reserve result to `ErrLLMLocalQuotaExceeded` (with denial kind in error message); on success, call inner; on inner returning `ErrLLMProviderRateLimited` — keep reservation, bubble; any other inner error — release, bubble.

- [ ] **Step 1: Write failing test**

Use a fake `Scorer` that can be scripted to return errors; use memory store.

```go
package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/common/errors" // alias below
	llmerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/llm/services"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

type fakeScorer struct {
	scoreErr error
	scoreOut []string
	enrichErr error
}
func (f *fakeScorer) ScoreBatch(ctx context.Context, jobs []dto.JobsRelevanceFilterInput) ([]string, error) {
	return f.scoreOut, f.scoreErr
}
func (f *fakeScorer) Enrich(ctx context.Context, in dto.JobPostEnrichInput) (*dto.JobPostEnrichResult, error) {
	return nil, f.enrichErr
}

func TestRateLimit_DenyReturnsLocalQuota(t *testing.T) {
	inner := &fakeScorer{}
	rs := store.NewLLMRateLimitMemoryStore()
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{
		SlotID: "s1", RPM: 1, RPD: 100,
	}, inner, rs)
	_, err := rl.ScoreBatch(context.Background(), nil)
	if err != nil { t.Fatal(err) }
	_, err = rl.ScoreBatch(context.Background(), nil)
	if !errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) { t.Fatalf("want local quota, got %v", err) }
}

func TestRateLimit_ReleaseOnInnerError(t *testing.T) {
	inner := &fakeScorer{scoreErr: errors.New("network boom")}
	rs := store.NewLLMRateLimitMemoryStore()
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{SlotID: "s1", RPM: 1, RPD: 100}, inner, rs)
	_, _ = rl.ScoreBatch(context.Background(), nil)
	// inner failed → reservation released → next call should succeed.
	inner.scoreErr = nil
	if _, err := rl.ScoreBatch(context.Background(), nil); err != nil { t.Fatalf("expected success, got %v", err) }
}

func TestRateLimit_Keep_Reservation_On_Provider_429(t *testing.T) {
	inner := &fakeScorer{scoreErr: llmerr.ErrLLMProviderRateLimited}
	rs := store.NewLLMRateLimitMemoryStore()
	rl := services.NewLLMRateLimitService(services.RateLimitSlot{SlotID: "s1", RPM: 1, RPD: 100}, inner, rs)
	_, err := rl.ScoreBatch(context.Background(), nil)
	if !errors.Is(err, llmerr.ErrLLMProviderRateLimited) { t.Fatalf("want provider 429, got %v", err) }
	// Reservation kept → next call should get local quota.
	inner.scoreErr = nil
	_, err = rl.ScoreBatch(context.Background(), nil)
	if !errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) { t.Fatalf("want local quota, got %v", err) }
}
```

- [ ] **Step 2: Run tests** — expect FAIL (service not defined).

- [ ] **Step 3: Implement `LLMRateLimitService.go`**

```go
package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	llmerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	llmcommon "github.com/trushilpatel/jobradar/service/llm/common"
	"github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

type RateLimitSlot struct {
	SlotID   string
	Provider string
	KeyID    string
	Model    string
	RPM      int
	RPD      int
	TPMIn    int
}

type rateLimitService struct {
	slot  RateLimitSlot
	inner Scorer
	store store.LLMRateLimitStore
	log   logger.Logger
}

func NewLLMRateLimitService(slot RateLimitSlot, inner Scorer, s store.LLMRateLimitStore) Scorer {
	return &rateLimitService{slot: slot, inner: inner, store: s}
}

func (s *rateLimitService) WithLogger(l logger.Logger) Scorer { s.log = l; return s }

func (s *rateLimitService) ScoreBatch(ctx context.Context, jobs []dto.JobsRelevanceFilterInput) ([]string, error) {
	reqID := uuid.NewString()
	tokens := estimateScoreBatchTokens(jobs)
	if denied, err := s.reserve(ctx, reqID, tokens); denied != nil || err != nil {
		if err != nil { return nil, err }
		return nil, denied
	}
	out, err := s.inner.ScoreBatch(ctx, jobs)
	return out, s.aftercall(ctx, reqID, err, len(out))
}

func (s *rateLimitService) Enrich(ctx context.Context, in dto.JobPostEnrichInput) (*dto.JobPostEnrichResult, error) {
	reqID := uuid.NewString()
	tokens := estimateEnrichTokens(in)
	if denied, err := s.reserve(ctx, reqID, tokens); denied != nil || err != nil {
		if err != nil { return nil, err }
		return nil, denied
	}
	out, err := s.inner.Enrich(ctx, in)
	return out, s.aftercall(ctx, reqID, err, 0)
}

func (s *rateLimitService) reserve(ctx context.Context, reqID string, tokens int) (error, error) {
	r, err := s.store.Reserve(ctx, store.ReserveReq{
		SlotID: s.slot.SlotID, ReqID: reqID,
		RPM: s.slot.RPM, RPD: s.slot.RPD, TPMIn: s.slot.TPMIn, Tokens: tokens,
	})
	if err != nil { return nil, fmt.Errorf("ratelimit reserve slot=%s: %w", s.slot.SlotID, err) }

	switch r {
	case store.ReserveOK:
		return nil, nil
	case store.ReserveDeniedRPM:
		return llmerr.ErrLLMLocalQuotaExceeded.WithMessage(llmcommon.OutcomeLocalQuotaRPM), nil
	case store.ReserveDeniedRPD:
		return llmerr.ErrLLMLocalQuotaExceeded.WithMessage(llmcommon.OutcomeLocalQuotaRPD), nil
	case store.ReserveDeniedTPM:
		return llmerr.ErrLLMLocalQuotaExceeded.WithMessage(llmcommon.OutcomeLocalQuotaTPM), nil
	default:
		return nil, fmt.Errorf("ratelimit reserve unexpected code %d", r)
	}
}

func (s *rateLimitService) aftercall(ctx context.Context, reqID string, innerErr error, _ int) error {
	if innerErr == nil { return nil }
	if errors.Is(innerErr, llmerr.ErrLLMProviderRateLimited) { return innerErr }
	if relErr := s.store.Release(ctx, s.slot.SlotID, reqID); relErr != nil {
		if s.log != nil { s.log.Warn("ratelimit release failed", logger.Str("slot", s.slot.SlotID), logger.Err(relErr)) }
	}
	return innerErr
}

// token estimators (carried over from old LLMImplService).
func estimateScoreBatchTokens(jobs []dto.JobsRelevanceFilterInput) int {
	return 150 + len(jobs)*20
}
func estimateEnrichTokens(in dto.JobPostEnrichInput) int {
	return 200 + len(in.RawText)/4
}
```

- [ ] **Step 4: Run tests** — expect PASS.

---

## Task 11 — Router service

**Files:**
- Create: `backend/service/llm/services/LLMRouterService.go`
- Create: `backend/service/llm/services/tests/LLMRouterService_test.go`

Contract: implements `Scorer`. Takes `[]RouterSlot{ SlotID, Provider, KeyID, Model, Scorer }`, a cooldown store, and a default model preference list (for logging/headroom tiebreaker). For each call: iterate preference list (or fall through all slots in config order if preference empty) → filter out slots in cooldown → invoke → on `ErrLLMLocalQuotaExceeded` try next (no cooldown) → on `ErrLLMProviderRateLimited` `RecordFailure` + try next → on success `RecordSuccess` + return → on other errors bubble immediately. Exhausted → return `*LLMAllExhaustedError`.

- [ ] **Step 1: Tests**

```go
package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	llmerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/llm/services"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

func TestRouter_FallsBackOnLocalQuota(t *testing.T) {
	quota := &fakeScorer{scoreErr: llmerr.ErrLLMLocalQuotaExceeded}
	ok    := &fakeScorer{scoreOut: []string{"id-1"}}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: quota},
		{SlotID: "s2", Model: "m1", Scorer: ok},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))
	out, err := r.ScoreBatch(context.Background(), nil)
	if err != nil { t.Fatal(err) }
	if len(out) != 1 || out[0] != "id-1" { t.Fatalf("unexpected %v", out) }
}

func TestRouter_RecordsCooldownOnProvider429(t *testing.T) {
	limited := &fakeScorer{scoreErr: llmerr.ErrLLMProviderRateLimited}
	ok := &fakeScorer{scoreOut: []string{"id-1"}}
	cd := store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: 10 * time.Second, Max: time.Minute})
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: limited},
		{SlotID: "s2", Model: "m1", Scorer: ok},
	}, cd)
	if _, err := r.ScoreBatch(context.Background(), nil); err != nil { t.Fatal(err) }
	st, _ := cd.Status(context.Background(), "s1")
	if st.FailureCount != 1 { t.Fatalf("want cooldown recorded, got %+v", st) }
}

func TestRouter_BubblesOtherErrors(t *testing.T) {
	boom := &fakeScorer{scoreErr: errors.New("network boom")}
	ok   := &fakeScorer{scoreOut: []string{"x"}}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: boom},
		{SlotID: "s2", Model: "m1", Scorer: ok},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))
	_, err := r.ScoreBatch(context.Background(), nil)
	if err == nil || err.Error() != "network boom" {
		t.Fatalf("expected network boom to bubble, got %v", err)
	}
}

func TestRouter_ReturnsAllExhaustedError(t *testing.T) {
	q1 := &fakeScorer{scoreErr: llmerr.ErrLLMLocalQuotaExceeded}
	q2 := &fakeScorer{scoreErr: llmerr.ErrLLMLocalQuotaExceeded}
	r := services.NewLLMRouterService([]services.RouterSlot{
		{SlotID: "s1", Model: "m1", Scorer: q1},
		{SlotID: "s2", Model: "m1", Scorer: q2},
	}, store.NewLLMCooldownMemoryStore(store.CooldownPolicy{Base: time.Second, Max: time.Minute}))
	_, err := r.ScoreBatch(context.Background(), nil)
	var ae *llmerr.LLMAllExhaustedError
	if !errors.As(err, &ae) { t.Fatalf("want LLMAllExhaustedError, got %v", err) }
	if len(ae.Slots) != 2 { t.Fatalf("want 2 slot statuses, got %d", len(ae.Slots)) }
}
```

- [ ] **Step 2: Implement `LLMRouterService.go`**

```go
package services

import (
	"context"
	"errors"
	"time"

	llmerr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/llm/store"
)

type RouterSlot struct {
	SlotID   string
	Provider string
	KeyID    string
	Model    string
	Scorer   Scorer
}

type routerService struct {
	slots    []RouterSlot
	cooldown store.LLMCooldownStore
}

func NewLLMRouterService(slots []RouterSlot, cd store.LLMCooldownStore) Scorer {
	return &routerService{slots: slots, cooldown: cd}
}

func (r *routerService) ScoreBatch(ctx context.Context, jobs []dto.JobsRelevanceFilterInput) ([]string, error) {
	var statuses []llmerr.LLMSlotStatus
	for _, s := range r.slots {
		ok, _ := r.cooldown.IsAvailable(ctx, s.SlotID)
		if !ok {
			statuses = append(statuses, r.statusOf(ctx, s, "cooldown"))
			continue
		}
		out, err := s.Scorer.ScoreBatch(ctx, jobs)
		if err == nil {
			_ = r.cooldown.RecordSuccess(ctx, s.SlotID)
			return out, nil
		}
		if errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) {
			statuses = append(statuses, r.statusOf(ctx, s, "local_quota"))
			continue
		}
		if errors.Is(err, llmerr.ErrLLMProviderRateLimited) {
			_ = r.cooldown.RecordFailure(ctx, s.SlotID)
			statuses = append(statuses, r.statusOf(ctx, s, "provider_429"))
			continue
		}
		return nil, err
	}
	return nil, &llmerr.LLMAllExhaustedError{Slots: statuses, RetryAfter: r.earliest(statuses)}
}

func (r *routerService) Enrich(ctx context.Context, in dto.JobPostEnrichInput) (*dto.JobPostEnrichResult, error) {
	var statuses []llmerr.LLMSlotStatus
	for _, s := range r.slots {
		ok, _ := r.cooldown.IsAvailable(ctx, s.SlotID)
		if !ok {
			statuses = append(statuses, r.statusOf(ctx, s, "cooldown"))
			continue
		}
		out, err := s.Scorer.Enrich(ctx, in)
		if err == nil {
			_ = r.cooldown.RecordSuccess(ctx, s.SlotID)
			return out, nil
		}
		if errors.Is(err, llmerr.ErrLLMLocalQuotaExceeded) {
			statuses = append(statuses, r.statusOf(ctx, s, "local_quota"))
			continue
		}
		if errors.Is(err, llmerr.ErrLLMProviderRateLimited) {
			_ = r.cooldown.RecordFailure(ctx, s.SlotID)
			statuses = append(statuses, r.statusOf(ctx, s, "provider_429"))
			continue
		}
		return nil, err
	}
	return nil, &llmerr.LLMAllExhaustedError{Slots: statuses, RetryAfter: r.earliest(statuses)}
}

func (r *routerService) statusOf(ctx context.Context, s RouterSlot, reason string) llmerr.LLMSlotStatus {
	cst, _ := r.cooldown.Status(ctx, s.SlotID)
	return llmerr.LLMSlotStatus{
		SlotID: s.SlotID, Model: s.Model, FailureCount: cst.FailureCount,
		CooldownUntil: cst.CooldownUntil, LastDenial: reason,
	}
}

func (r *routerService) earliest(ss []llmerr.LLMSlotStatus) time.Time {
	var out time.Time
	for _, s := range ss {
		if s.CooldownUntil.IsZero() { continue }
		if out.IsZero() || s.CooldownUntil.Before(out) { out = s.CooldownUntil }
	}
	return out
}
```

- [ ] **Step 3: Run tests** — expect PASS.

---

## Task 12 — Gemini 429 mapping

**Files:**
- Modify: `backend/service/llm/services/LLMGeminiService.go`

- [ ] **Step 1: Map 429 errors**

At the bottom of `generate()` after `err != nil`:

```go
if err != nil {
    if isGemini429(err) {
        return "", llmerr.ErrLLMProviderRateLimited
    }
    return "", err
}
```

Add helper at bottom of file:

```go
func isGemini429(err error) bool {
    if err == nil { return false }
    msg := err.Error()
    return strings.Contains(msg, "429") ||
        strings.Contains(strings.ToLower(msg), "rate limit") ||
        strings.Contains(strings.ToLower(msg), "resource_exhausted") ||
        strings.Contains(strings.ToLower(msg), "quota")
}
```

Add `llmerr "github.com/trushilpatel/jobradar/common/errors"` to imports.

- [ ] **Step 2: Verify build** — `cd backend && go build ./...`

---

## Task 13 — Factory rewrite

**Files:**
- Modify: `backend/service/llm/services/LLMFactory.go`

Behaviour: if `opts.Slots` is non-empty, build one `geminiScorer` per slot, wrap each in `LLMRateLimitService`, collect into `LLMRouterService`. Legacy single-slot path (for back-compat while migrating callers): build one slot from legacy `APIKey` + `Model` + `Limits` when `Slots` is empty.

- [ ] **Step 1: Update `Options`**

```go
type Options struct {
    Provider    string
    Slots       []SlotOptions       // new
    TargetRoles []string
    YearsExp    int
    Log         logger.Logger
    RateLimitStore store.LLMRateLimitStore
    CooldownStore  store.LLMCooldownStore

    // Legacy (back-compat) — used only when Slots is empty.
    APIKey string
    Model  string
    Limits RateLimits
}

type SlotOptions struct {
    KeyID  string
    APIKey string
    Model  string
    RPM    int
    RPD    int
    TPMIn  int
}
```

- [ ] **Step 2: Rewrite `NewScorer`**

```go
func NewScorer(opts Options) (Scorer, error) {
    if opts.Provider != llmcommon.ProviderGemini {
        return nil, fmt.Errorf("llm: unknown provider %q (supported: gemini)", opts.Provider)
    }
    if opts.RateLimitStore == nil || opts.CooldownStore == nil {
        return nil, fmt.Errorf("llm: RateLimitStore and CooldownStore are required")
    }

    slots := opts.Slots
    if len(slots) == 0 {
        slots = []SlotOptions{{
            KeyID: "default", APIKey: opts.APIKey, Model: opts.Model,
            RPM: opts.Limits.RPM, RPD: opts.Limits.RPD, TPMIn: opts.Limits.TPMIn,
        }}
    }

    seen := map[string]bool{}
    routerSlots := make([]RouterSlot, 0, len(slots))
    for _, s := range slots {
        if s.APIKey == "" || s.Model == "" || s.KeyID == "" {
            return nil, fmt.Errorf("llm: slot missing fields (keyID=%q model=%q)", s.KeyID, s.Model)
        }
        slotID := llmcommon.SlotID(opts.Provider, s.KeyID, s.Model)
        if seen[slotID] { return nil, fmt.Errorf("llm: duplicate slot %s", slotID) }
        seen[slotID] = true

        inner, err := newGeminiScorer(context.Background(), s.APIKey, s.Model, opts.TargetRoles, opts.YearsExp)
        if err != nil {
            return nil, fmt.Errorf("llm: init gemini slot %s: %w", slotID, err)
        }
        rl := NewLLMRateLimitService(RateLimitSlot{
            SlotID: slotID, Provider: opts.Provider, KeyID: s.KeyID, Model: s.Model,
            RPM: s.RPM, RPD: s.RPD, TPMIn: s.TPMIn,
        }, inner, opts.RateLimitStore)
        routerSlots = append(routerSlots, RouterSlot{
            SlotID: slotID, Provider: opts.Provider, KeyID: s.KeyID, Model: s.Model, Scorer: rl,
        })
    }

    return NewLLMRouterService(routerSlots, opts.CooldownStore), nil
}
```

- [ ] **Step 3: Verify build** — `cd backend && go build ./...`

---

## Task 14 — Delete LLMImplService + update callers

**Files:**
- Delete: `backend/service/llm/services/LLMImplService.go`
- Modify: `backend/apps/filter/` — wiring updated
- Modify: `backend/apps/analyzer/` — wiring updated

- [ ] **Step 1: Find callers**

```bash
cd backend && grep -rn "NewScorer\b\|rateLimitedScorer\|newRateLimitedScorer\|llm.services.RateLimits\b" --include="*.go"
```

- [ ] **Step 2: Update filter + analyzer wiring**

At each worker startup site:

```go
rdb, err := redis.New(ctx, cfg.Redis.Addr, cfg.Redis.DB)
if err != nil { log.Fatal(err) }
rls := store.NewLLMRateLimitRedisStore(rdb)
cds := store.NewLLMCooldownRedisStore(rdb, store.CooldownPolicy{
    Base:   time.Duration(cfg.Cooldown.BaseSeconds) * time.Second,
    Max:    time.Duration(cfg.Cooldown.MaxSeconds) * time.Second,
    Jitter: cfg.Cooldown.Jitter,
})

slotOpts := buildSlotOpts(cfg.Filter) // or cfg.Enricher
scorer, err := services.NewScorer(services.Options{
    Provider: cfg.LLM.Provider,
    Slots: slotOpts,
    TargetRoles: cfg.LLM.TargetRoles,
    YearsExp: cfg.LLM.YearsExp,
    Log: log,
    RateLimitStore: rls, CooldownStore: cds,
    // legacy fallback:
    APIKey: cfg.LLM.APIKey,
    Model:  cfg.Filter.Model, // or cfg.Enricher.Model
    Limits: services.RateLimits{RPM: cfg.Filter.RateLimits.RPM, RPD: cfg.Filter.RateLimits.RPD, TPMIn: cfg.Filter.RateLimits.TPMIn},
})
```

Add `buildSlotOpts` helper per worker that maps `[]LLMSlotConfig` → `[]SlotOptions`.

- [ ] **Step 3: Delete `LLMImplService.go`**

```bash
rm backend/service/llm/services/LLMImplService.go
```

- [ ] **Step 4: Verify build + run existing LLM tests**

```bash
cd backend && go build ./... && go test ./service/llm/...
```

---

## Task 15 — docker-compose Redis service

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: Add service**

```yaml
  redis:
    image: redis:7-alpine
    container_name: jobradar_redis
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10
    restart: unless-stopped
```

Add `redis` under the `depends_on:` for filter + analyzer services if they use healthchecks.

- [ ] **Step 2: Verify compose**

```bash
docker compose config > /dev/null
```

---

## Task 16 — Design doc (CLAUDE.md) rewrite

**Files:**
- Modify: `backend/service/llm/CLAUDE.md`

- [ ] **Step 1: Replace file contents** with the content below.

```markdown
# service/llm/ — LLM routing + rate limiting + cooldown

## Interfaces (`services/LLMService.go`)

**`JobsRelevanceFilter`** — batch-scores job summaries, returns IDs of relevant jobs. Used by the filter worker.
**`JobPostEnricher`** — extracts structured data from a full job posting. Used by the analyzer worker.
**`Scorer`** — combines both. Returned by `NewScorer`; callers receive the narrower interface they need.

## Chain (decorator)

```
Router → per-slot RateLimit → GeminiService (per-slot)
          ↘ CooldownStore (injected)
          ↘ RateLimitStore (injected)
```

- Every configured `(provider, keyID, model)` becomes a **slot**. Each slot holds its own `geminiScorer` + `LLMRateLimitService`.
- The **router** picks the first non-cooled slot and invokes it. On `ErrLLMLocalQuotaExceeded` it skips without touching cooldown. On `ErrLLMProviderRateLimited` it records a cooldown failure and tries the next slot. Other errors bubble. When no slot succeeds, returns `*LLMAllExhaustedError`.

## Structure

```
service/llm/
  common/         ← provider + outcome constants, SlotID()
  dto/            ← JobsRelevanceFilterInput, JobPostEnrichInput, JobPostEnrichResult, SlotStatus
  prompts/        ← BuildScoreBatch, BuildEnrich
  services/
    LLMService.go          ← interfaces (contract)
    LLMGeminiService.go    ← per-slot Gemini client; maps 429 → ErrLLMProviderRateLimited
    LLMRateLimitService.go ← per-slot decorator over Scorer
    LLMRouterService.go    ← multi-slot router; terminal Scorer
    LLMFactory.go          ← Options → router
  store/
    LLMRateLimitStore.go         ← interface + ReserveResult
    LLMRateLimitRedisStore.go    ← Redis + Lua reserve.lua
    LLMRateLimitMemoryStore.go   ← tests
    LLMCooldownStore.go          ← interface
    LLMCooldownRedisStore.go     ← Redis impl
    LLMCooldownMemoryStore.go    ← tests
    reserve.lua                  ← atomic RPM + RPD + TPM reserve
```

## Rate limit store

Three dimensions per slot, all `0 = unlimited`:

| Dim    | Shape                                              |
| ------ | -------------------------------------------------- |
| RPM    | sliding 60s ZSET (member=reqID, score=now_millis)  |
| RPD    | fixed UTC-day INT counter (`…:day:{YYYY-MM-DD}`)   |
| TPMIn  | sliding 60s ZSET + HASH(reqID→tokens) + INT(sum)   |

Reservation is atomic via `reserve.lua`. Release rolls back a reservation (non-429 inner failures).

## Cooldown store

Per-slot exponential backoff with jitter. Keys:

```
cooldown:{slotID}:failure_count
cooldown:{slotID}:cooldown_until  (unix millis)
```

Policy: `Base * 2^(n-1)` capped at `Max`, then multiplied by `1 ± Jitter`. Success → count reset + until deleted.

## Errors (`common/errors`)

| Sentinel                     | Meaning                                                    | Router reaction     |
| ---------------------------- | ---------------------------------------------------------- | ------------------- |
| `ErrLLMValidation`           | bad input                                                  | caller bug (4xx)    |
| `ErrLLMLocalQuotaExceeded`   | our own RPM/RPD/TPM counter said no                        | skip, no cooldown   |
| `ErrLLMProviderRateLimited`  | provider 429                                               | record cooldown     |
| `ErrLLMAllExhausted`         | all slots cooled or over quota                             | fail-fast (503)     |

`*LLMAllExhaustedError` carries `[]LLMSlotStatus` + `RetryAfter` for the caller.

## Factory rules

- `NewScorer(Options)` **requires** `RateLimitStore` and `CooldownStore`.
- `Slots[]` preferred. Legacy `APIKey` + `Model` + `Limits` kept for back-compat — becomes a single `default` slot.
- Duplicate `(provider, keyID, model)` → error at startup.

## Worker integration

Workers still depend on `JobsRelevanceFilter` / `JobPostEnricher`. Only their wiring changes — see `apps/filter/` + `apps/analyzer/`.

## Rules

- Slot decisions belong in the router — never in workers
- `geminiScorer` stays focused: SDK call + JSON parse + 429 detection. No rate limiting inside
- All Redis access goes through `store/` — no `*redis.Client` outside that subpackage
- `ErrLLMLocalQuotaExceeded` **never** triggers cooldown — it's a routine skip
- Only real `ErrLLMProviderRateLimited` increments `failure_count`
```

- [ ] **Step 2: Verify file renders** — read it back to sanity-check.

---

## Task 17 — Final lint + tests

- [ ] **Step 1: Run lint**

```bash
cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m
```

Expected: clean.

- [ ] **Step 2: Run full test suite**

```bash
cd backend && go test ./... -count=1
```

Expected: all green.

---

## Task 18 — Write spec file

**Files:**
- Create: `docs/superpowers/specs/2026-04-19-llm-router.md`

- [ ] **Step 1: Short spec doc**

```markdown
# LLM Router Spec (2026-04-19)

- `service/llm/services/LLMRouterService.go` — multi-slot fallback, cooldown integration, exhaustion error
- `service/llm/services/LLMRateLimitService.go` — per-slot reserve + release around inner Scorer
- `service/llm/services/LLMGeminiService.go` — SDK call + 429 detection
- `service/llm/services/LLMFactory.go` — assembles slots → rate-limit → router
- `service/llm/store/LLMRateLimitRedisStore.go` + `reserve.lua` — atomic RPM/RPD/TPM reserve
- `service/llm/store/LLMCooldownRedisStore.go` — exponential backoff with jitter
- `common/redis/client.go` — Redis bootstrap
- `common/config/config.go` — REDIS_*, LLM_COOLDOWN_*, FILTER_LLM_SLOTS, ENRICHER_LLM_SLOTS
- `common/errors/errors.go` — ErrLLM* sentinels + LLMAllExhaustedError
- `docker-compose.yml` — redis service
- `service/llm/CLAUDE.md` — design doc
```

---

## Self-review checklist (run at end before hand-off)

1. **Spec coverage** — every section of the user's design is covered: Redis ✓ multi-slot ✓ RPM/RPD/TPM via Lua ✓ cooldown exp+jitter ✓ fail-fast ✓ provider 429 vs local quota distinction ✓ fallback order ✓ per-slot independence ✓.
2. **No placeholders** — all code blocks are concrete; no TODO / TBD.
3. **Type consistency** — `Scorer`, `RouterSlot`, `RateLimitSlot`, `SlotOptions`, `ReserveReq`, `ReserveResult` used consistently across tasks.
