# LLM Rate Limiting — Multi-Instance Design

**Owner:** Trushil · **Status:** design · **Date:** 2026-04-19

---

## 1. Problem Statement

The current `LLMImplService.go` rate limiter uses an in-process `sync.Mutex` + `golang.org/x/time/rate` token bucket. This works for a single process but breaks completely under multi-instance deployment:

- N replicas of the filter worker and M replicas of the analyzer worker each hold their own independent token bucket
- Total quota consumed = N × RPM + M × RPM, which will breach the provider's account-level limits at any non-trivial replica count
- The daily counter (`dayCount`) resets independently in each replica — a day's entire quota can be consumed in the first hour if enough replicas run simultaneously
- Circuit breaker state is local — one instance sees 429s and opens its breaker; all other instances continue firing until they hit the same wall

The constraint is now: **N filter replicas + M analyzer replicas all share one LLM provider account.** The system must enforce quota at the account level, not the instance level.

---

## 2. Design Goals

1. Quota (RPM, RPD, TPM) is enforced account-wide across all instances of all worker types
2. Circuit breaker state is shared — when one instance sees the provider fail, all instances back off
3. Provider failover does not cause a thundering herd — instances do not all switch simultaneously
4. Redis failure degrades gracefully — workers do not deadlock waiting for Redis
5. Token accounting is resilient to instance death mid-request
6. The design adds no new required infrastructure for single-instance deployment (Redis is optional, falls back to local limiter)

---

## 3. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│  Worker Pod (filter or analyzer) — any of N+M replicas              │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │  SharedRateLimitedScorer  (wraps Scorer interface)           │   │
│  │                                                              │   │
│  │   1. AcquireSlot(ctx, estimatedTokens)                       │   │
│  │      ├─ CheckCircuitBreaker (shared CB state via Redis)      │   │
│  │      ├─ TryConsumeRPM (Redis sliding window, Lua script)     │   │
│  │      ├─ TryConsumeTPM (Redis sliding window, Lua script)     │   │
│  │      └─ TryConsumeRPD (Redis counter + TTL)                  │   │
│  │                                                              │   │
│  │   2. inner.ScoreBatch / inner.Enrich (actual LLM call)       │   │
│  │                                                              │   │
│  │   3. RecordActualTokens(actualIn, actualOut)                 │   │
│  │      └─ Adjust TPM bucket (actual − estimated delta)         │   │
│  │                                                              │   │
│  │   4. RecordOutcome(success | rate_limited | error)           │   │
│  │      └─ Drive circuit breaker transitions                    │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                         │                                            │
└─────────────────────────┼────────────────────────────────────────────┘
                          │ Redis commands (single round-trip per call)
                          ▼
              ┌─────────────────────┐
              │   Redis (shared)    │
              │                     │
              │  rpm:{provider}     │  Sorted set — sliding window
              │  tpm:{provider}     │  Sorted set — sliding window
              │  rpd:{provider}     │  Counter + TTL to next midnight
              │  cb:{provider}      │  Hash — CB state
              │  cb:half:{provider} │  String — half-open probe lock
              └─────────────────────┘
```

---

## 4. Redis Key Schema

All keys are namespaced by provider name (e.g. `gemini`, `openai`) so multiple providers coexist without collision.

### 4.1 RPM — Sliding Window

```
rpm:{provider}
  type: sorted set
  member: "{unix_nanoseconds}:{random_suffix}"  (unique per request)
  score: unix_nanoseconds (for range deletion)
  TTL: 70s (slightly > 60s to survive clock skew)
```

The Lua script atomically:
1. Removes members older than `now - 60s`
2. Counts remaining members
3. If count < RPM limit: adds new member, returns 1 (allowed)
4. Else: returns 0 + time until oldest member expires (for wait calculation)

Using nanoseconds + random suffix as the member key guarantees uniqueness even if two instances call at the exact same millisecond.

### 4.2 TPM — Sliding Window (Input Tokens)

```
tpm:{provider}
  type: sorted set
  member: "{unix_nanoseconds}:{random_suffix}"
  score: unix_nanoseconds
  weight field: stored in member value as "ns:random:tokens"
  TTL: 70s
```

The Lua script for TPM differs from RPM in one way: rather than counting members, it sums the token weights of members within the 60s window. Member value encodes `{timestamp}:{nonce}:{tokenCount}` delimited by `:`. The Lua script parses token count from the member name for the ZSCORE-less summation approach.

**Implementation note:** Redis sorted sets do not natively support weighted sums. Two options:
- Store token count in member value, iterate to sum (O(N) per call, acceptable at low RPM)
- Use a separate counter key that decrements on expiry via a Lua script that manages both the set and a shadow counter

Given typical batch sizes (50 jobs × ~170 tokens ≈ 8,500 tokens/batch) and RPM limits (15), the sorted set will have at most 15 members. O(N) iteration is fine.

### 4.3 RPD — Daily Counter

```
rpd:{provider}
  type: string (integer counter)
  TTL: seconds until next UTC midnight (set on creation, not updated on increment)
```

- `INCR rpd:{provider}` returns the new count atomically
- If the key did not exist before INCR, set TTL immediately using a Lua script (INCR + TTL in one round trip)
- If count > RPD limit: deny the request; also expose the TTL as wait duration

The TTL approach means the counter auto-resets at midnight without a separate reset job. The window is not perfectly aligned to a UTC day if the first request arrives at 23:59, but it self-corrects within 1 day and this is acceptable for a free-tier guard.

### 4.4 Circuit Breaker State

```
cb:{provider}
  type: hash
  fields:
    state          "closed" | "open" | "half_open"
    failure_count  integer
    last_failure   unix_seconds
    opened_at      unix_seconds
    version        integer (monotone increment, used for CAS)
  TTL: none (permanent until explicitly reset)
```

```
cb:half:{provider}
  type: string (value = instance_id)
  TTL: probe_timeout_seconds (e.g. 30s)
```

The `cb:half:{provider}` key acts as a mutex for the half-open probe: only the instance that successfully `SET cb:half:{provider} {instance_id} NX EX 30` gets to send the probe request. All other instances hitting the half-open state skip the probe and fail fast.

---

## 5. Lua Scripts

All quota checks are single round-trip atomic operations implemented as Lua scripts loaded at startup via `SCRIPT LOAD`. This eliminates TOCTOU races between check and decrement.

### 5.1 RPM Check + Acquire (`rpm_acquire.lua`)

```lua
local key    = KEYS[1]
local limit  = tonumber(ARGV[1])
local now_ns = ARGV[2]        -- nanoseconds as string
local member = ARGV[3]        -- "now_ns:nonce"
local window = tonumber(ARGV[4]) -- 60000000000 (60s in ns)
local ttl    = tonumber(ARGV[5]) -- 70 (seconds)

-- Evict expired members
redis.call('ZREMRANGEBYSCORE', key, '-inf', tostring(tonumber(now_ns) - window))

-- Count current window
local count = redis.call('ZCARD', key)

if count >= limit then
    -- Return remaining ns until oldest expires
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    if #oldest >= 2 then
        local wait_ns = tonumber(oldest[2]) + window - tonumber(now_ns)
        return {0, tostring(math.max(0, wait_ns))}
    end
    return {0, '0'}
end

redis.call('ZADD', key, now_ns, member)
redis.call('EXPIRE', key, ttl)
return {1, '0'}
```

### 5.2 TPM Check + Acquire (`tpm_acquire.lua`)

Same structure as RPM but sums token counts from member values instead of counting cardinality.

### 5.3 RPD Check + Acquire (`rpd_acquire.lua`)

```lua
local key     = KEYS[1]
local limit   = tonumber(ARGV[1])
local midnight = tonumber(ARGV[2])  -- unix seconds of next UTC midnight
local now     = tonumber(ARGV[3])

local count = redis.call('INCR', key)
if count == 1 then
    redis.call('EXPIREAT', key, midnight)
end
if count > limit then
    -- Undo the increment
    redis.call('DECR', key)
    local ttl = redis.call('TTL', key)
    return {0, tostring(ttl)}
end
return {1, '0'}
```

### 5.4 Circuit Breaker Transition (`cb_transition.lua`)

Takes current state, expected version, and new state. Uses optimistic concurrency: if the version has changed since the caller read it, the write is rejected (returns 0). The caller re-reads and re-evaluates.

```lua
local key          = KEYS[1]
local expected_ver = tonumber(ARGV[1])
local new_state    = ARGV[2]
local now          = ARGV[3]
local new_failure  = ARGV[4]  -- may be empty

local ver = tonumber(redis.call('HGET', key, 'version') or '0')
if ver ~= expected_ver then
    return 0  -- lost the race, caller should re-read
end

redis.call('HSET', key,
    'state', new_state,
    'version', tostring(ver + 1),
    'last_failure', now
)
if new_failure ~= '' then
    redis.call('HSET', key, 'failure_count', new_failure)
end
if new_state == 'open' then
    redis.call('HSET', key, 'opened_at', now)
end
return 1
```

---

## 6. Circuit Breaker — Shared State Protocol

### 6.1 State Machine

```
                    ┌─────────────────────────────────────┐
          failure   │                                     │
          count     │  CLOSED (normal operation)          │
          ≥         │  All instances: make requests       │
          threshold │  failures increment shared counter  │
                    └──────────────────┬──────────────────┘
                                       │ threshold crossed
                                       │ (cb_transition.lua CLOSED→OPEN, CAS)
                                       ▼
                    ┌─────────────────────────────────────┐
                    │  OPEN (tripped)                     │
                    │  All instances: fail fast           │
                    │  No LLM calls                       │
                    │  Wait for recovery_timeout (e.g. 60s)│
                    └──────────────────┬──────────────────┘
                                       │ recovery_timeout elapsed
                                       │ (instance reads opened_at + elapsed)
                                       ▼
                    ┌─────────────────────────────────────┐
                    │  HALF_OPEN (probe mode)              │
                    │  One instance wins cb:half:{p} NX   │
                    │  probe succeeds → CLOSED (CAS)      │
                    │  probe fails → back to OPEN (CAS)   │
                    └─────────────────────────────────────┘
```

### 6.2 Race Conditions and Their Resolution

**CLOSED → OPEN race:** Multiple instances can each increment the failure counter and each attempt the CAS transition. The Lua CAS script (`cb_transition.lua`) only allows one write per version increment. The first writer wins; all other writers get version mismatch and re-read the hash — they will see `state=open` and back off immediately. No correctness issue; the worst case is one extra transition attempt.

**OPEN → HALF_OPEN:** Any instance noticing `now - opened_at > recovery_timeout` can attempt the HALF_OPEN probe by doing `SET cb:half:{provider} {instance_id} NX EX 30`. Only one succeeds. All others continue to fail fast. This serializes probe attempts to one per 30s.

**HALF_OPEN → CLOSED race:** The probing instance is the only one making requests (the `NX` lock ensures this). No race.

**HALF_OPEN → OPEN race (probe failure):** The probing instance writes OPEN via CAS. Other instances are in fail-fast mode and will re-read the new OPEN state. No race.

### 6.3 Failure Threshold Configuration

```
CB_FAILURE_THRESHOLD  default: 5   (consecutive failures to trip)
CB_RECOVERY_TIMEOUT   default: 60  (seconds before half-open probe)
CB_SUCCESS_THRESHOLD  default: 2   (consecutive successes to reset from half-open)
```

"Consecutive failures" is defined as: failure_count incremented each time any instance gets a 429, 500, or timeout from the LLM provider. A success resets the per-instance success counter but does not reset the shared failure count until `success_threshold` is reached — at that point the transition script zeros `failure_count`.

---

## 7. Thundering Herd — Provider Failover

When a provider circuit breaker opens, instances that support fallback providers need to switch without all simultaneously hammering the fallback at full rate.

### 7.1 Jittered Failover

Each instance, on detecting `state=open` for the primary, waits a jittered delay before switching to the backup provider:

```
failover_wait = base_delay × (1 + rand(0, jitter_factor))
base_delay    = 100ms (configurable)
jitter_factor = 1.0   (so wait is between 100ms and 200ms)
```

With N=10 instances and 100ms base delay + full jitter, failover attempts spread over ~200ms instead of all hitting the backup simultaneously. The backup also has its own Redis rate limit keys, so even if all instances switch at once, the shared quota prevents a burst.

### 7.2 Failover Provider Chain

```
Options struct:
    Providers []ProviderConfig  // ordered: primary, fallback1, fallback2
```

```go
type ProviderConfig struct {
    Name   string
    APIKey string
    Model  string
    Limits RateLimits
}
```

`NewScorer` builds one `geminiScorer` (or equivalent) per provider. `SharedRateLimitedScorer` iterates the provider list in order. For each provider it checks the circuit breaker key in Redis. The first non-open provider with available quota wins.

If all providers are open: return `ErrAllProvidersUnavailable`. Workers handle this the same way as a ScoreBatch failure — mark jobs as `reason=error` in the ledger.

### 7.3 Sticky Failover Window

To prevent oscillation (instance A uses primary, instance B uses fallback, both succeed, both switch back, repeat), once an instance fails over it stays on the backup for a sticky window:

```
instance-local variable: currentProviderIdx int
                         stickyUntil        time.Time
sticky_window = 5 * time.Minute
```

After `stickyUntil`, the instance re-evaluates from the primary. This is instance-local (not in Redis) because stickiness is a performance optimization, not a correctness requirement — different instances can be on different providers simultaneously.

---

## 8. Charge-on-Response Token Accounting

### 8.1 The Problem

Token quotas (TPM) must deduct actual tokens consumed, not just estimates. The problem:

1. Instance acquires slot using estimated tokens (`estimated = 200 + len(rawText)/4`)
2. Instance calls LLM — the call may take 2–5 seconds
3. LLM returns actual token counts in the response metadata
4. Instance must correct the quota: add back `estimated - actual` (or deduct additional tokens if actual > estimated)
5. The instance may die between steps 3 and 4

### 8.2 Debit-Then-Adjust Protocol

The TPM sliding window uses the following protocol:

**On acquire:** debit estimated tokens at request time using a member value encoding `{now_ns}:{nonce}:{estimated_tokens}`.

**On response:** issue a correction. The correction does not modify the existing sorted set member (that would require a delete + re-add). Instead, it adds a second "adjustment" member with a negative token value:

```
member value: "{now_ns}:{nonce}:delta:{delta_tokens}"
```

Where `delta_tokens = actual_tokens - estimated_tokens`. If actual < estimated, `delta_tokens` is negative (correction reduces total usage). If actual > estimated, `delta_tokens` is positive (adds to usage).

The Lua TPM-sum script handles both positive and negative members when computing the window total.

**On instance death:** the estimated tokens remain debited. This is intentional — it is the conservative safe failure. The debit expires naturally when the sorted set member ages out of the 60s window. The worst case is a brief over-estimation of quota usage for one 60s window. The alternative (fail-open: not deducting on death) risks actual quota breach, which is worse.

### 8.3 Estimator Calibration

The estimator (`estimateScoreBatchTokens`, `estimateEnrichTokens`) should be tuned to over-estimate slightly:

- `ScoreBatch`: `200 + len(jobs) * 25` (current: `150 + len(jobs) * 20`)
- `Enrich`: `300 + len(rawText) / 3` (current: `200 + len(rawText) / 4`)

Over-estimating by ~20% means adjustments are mostly negative (giving quota back), which means TPM is slightly underutilized. This is the correct safety direction — quota overage causes 429s, quota underutilization only costs throughput.

### 8.4 Long-Running Request Timeout

If an instance holds a quota slot and the LLM call hangs indefinitely, the estimated tokens remain debited forever. To bound this:

- Every ScoreBatch and Enrich call carries a `context.WithTimeout` of `max_llm_call_timeout` (default 60s, configurable via `LLM_CALL_TIMEOUT_SECONDS`)
- The sorted set TTL (70s) bounds the worst-case debit duration to 70s regardless of instance behavior

---

## 9. Redis HA Story

### 9.1 Deployment Options

| Mode | Use case | Notes |
|------|----------|-------|
| Single node | Dev, single-replica deployment | No HA; acceptable if workers are also single-replica |
| Sentinel (3 nodes) | Multi-replica workers, production | Automatic failover; 1 primary + 2 replicas |
| Cluster (6 nodes) | Not needed at this scale | Over-engineered for a single-user job tracker |

**Recommendation:** Redis Sentinel with 3 nodes for any multi-replica deployment. The system has one logical "shard" of keys (one provider namespace) — no sharding benefit from Cluster. Sentinel provides failover in ~30s without application changes (the client reconnects to the new primary).

### 9.2 Connection

Use `go-redis/redis/v9` with `redis.NewFailoverClient` for Sentinel, `redis.NewClient` for single-node. The factory picks based on `REDIS_SENTINEL_ADDRS` being set:

```
REDIS_ADDR              single-node address (default: localhost:6379)
REDIS_SENTINEL_ADDRS    comma-separated sentinel addresses (e.g. sentinel1:26379,sentinel2:26379,sentinel3:26379)
REDIS_SENTINEL_MASTER   master name (default: mymaster)
REDIS_PASSWORD          optional
REDIS_DB                database index (default: 0)
```

### 9.3 Degraded Mode — Redis Down

This is the most important operational question. Two philosophies:

**Fail-closed:** if Redis is unreachable, reject all LLM calls with `ErrRateLimiterUnavailable`. Workers mark the entire batch as `error` in the ledger.

- Blast radius: zero LLM calls while Redis is down. Jobs accumulate in Kafka. When Redis recovers, normal processing resumes. No quota breach.
- Risk: pipeline stalls if Redis has a prolonged outage.

**Fail-open:** if Redis is unreachable, fall back to the per-instance local limiter.

- Blast radius: N × (per-instance RPM) during outage. With N=3 filter replicas and RPM=15 per replica, the account sees 45 RPM instead of 15. If the provider limit is 15 RPM at the account level, this causes 429s on ~2/3 of requests.
- Risk: quota breach during outage. 429s trigger circuit breaker → all instances stall anyway. The fail-open becomes fail-closed via the circuit breaker after a few seconds of 429s.

**Decision: Fail-open with circuit breaker as the backstop.** Rationale:

1. For a single-user job tracker, a brief quota overage during a Redis outage is preferable to a total pipeline stall. Jobs will receive 429s, the circuit breaker will trip, and the system will self-throttle — the end state is the same (pipeline paused) but fail-open gives the circuit breaker a chance to observe real provider behavior rather than making a pre-emptive shutdown call.
2. If Redis is degraded (connected but slow), fail-open prevents Redis latency from adding to every LLM call's latency.

**Fallback trigger:** if a Redis command returns an error that is not `redis.Nil` (key not found) and not a Lua script error, the `SharedRateLimitedScorer` notes the error and falls through to the local limiter. A counter `redis_unavailable_total` is incremented for alerting.

**Recovery:** when Redis becomes available again, the fallback path stops being used on the next successful Redis ping (checked every 5s in the background). There is no explicit state reset needed — the sorted set keys for RPM/TPM will be empty (they expired during the outage) and the counters start fresh.

---

## 10. Failure Modes and Blast Radius

### 10.1 Redis Node Failure (Sentinel Primary Fails)

| Phase | Behavior |
|-------|----------|
| 0–30s (failover in progress) | Redis commands fail → workers fall back to local limiter. Up to N × RPM effective request rate. Circuit breaker may trip if provider returns 429s. |
| 30s (Sentinel promotes replica) | Redis reconnects automatically. Workers detect Redis is healthy and resume shared limiting. Local limiter state is discarded. |
| After recovery | Quota keys are fresh (empty sorted sets). The first 60s after recovery the system gets a "free window" — no accumulated debt. Acceptable. |

**Blast radius:** brief quota overage + possible provider 429s. Self-heals within ~1 minute.

### 10.2 Redis Completely Down (All Nodes Dead)

| Duration | Behavior |
|----------|----------|
| 0–60s | Local limiters run. N×M effective quota. If provider 429s, circuit breaker trips. |
| 60s+ | Circuit breaker (now local per-instance) prevents further calls. Pipeline effectively stalled. |
| Redis returns | Circuit breaker resets via half-open probe. Normal processing resumes. |

**Blast radius:** provider quota breach possible for 60s, then self-throttled. No data loss (jobs remain in Kafka). Redis restart is the recovery action.

### 10.3 Single Worker Instance Dies Mid-Request

| Scenario | Behavior |
|----------|----------|
| Died before LLM response | Estimated tokens remain debited in Redis sorted set for up to 70s. No correctness issue. |
| Died after LLM response but before token adjustment | Over-estimated tokens remain debited for up to 70s. Slightly underutilizes quota window. No correctness issue. |
| Died after LLM response but before writing ledger | The Kafka consumer offset for that partition was not committed. Kafka redelivers the message to another instance. The ledger's `job_pipeline_ledger` unique index prevents double-writes. No data loss, no duplicate processing. |

**Blast radius:** at most one 60s window of slightly reduced throughput per dead instance.

### 10.4 Circuit Breaker Stuck in OPEN

If the LLM provider recovers but the circuit breaker cannot transition to HALF_OPEN (e.g. the half-open probe instance itself dies mid-probe and the `cb:half:{provider}` key was not set):

- `cb:half:{provider}` has a 30s TTL. After 30s, any instance can attempt a new probe.
- Worst case: one additional 30s wait before recovery.

If the circuit breaker is stuck open due to a Redis write failure (cb_transition.lua fails): the key remains at `open`. The instance logs a warning (`circuit_breaker_write_failed`). Next time recovery_timeout elapses, another instance will attempt the probe. Self-healing.

### 10.5 All LLM Providers Unavailable (Every Circuit Breaker Open)

| Duration | Behavior |
|----------|----------|
| 0–5min | All instances return `ErrAllProvidersUnavailable`. Workers mark all jobs as `reason=error` in ledger. Kafka consumer offset advances; jobs are not re-queued. |
| 5min+ | Operators should inspect provider status pages and reset circuit breakers manually if needed: `DEL cb:{provider}` in Redis. |

**Blast radius:** all new jobs from this window are lost (marked error). They can be reprocessed by deleting their ledger rows — a documented ops runbook. No other systems affected.

### 10.6 Lua Script Race on First-Time Key Creation

If two instances simultaneously check an empty RPM key and both see count=0, both will succeed in adding their members. The Lua script is atomic — this cannot happen. EVALSHA guarantees atomicity at the Redis keyspace level.

### 10.7 Clock Skew Between Instances

The sliding window scripts use the server-side clock in Redis (`redis.call('TIME')`) rather than the client-supplied timestamp for window boundary calculations. The `now_ns` argument is used as the sorted set score (for ordering and future eviction), but the eviction cutoff is computed inside the script as `redis.call('TIME')[1] * 1e9 - window_ns`. This eliminates clock skew between instances as a failure vector.

**Implementation note:** update the Lua scripts to call `redis.call('TIME')` internally rather than accepting `now_ns` as ARGV for the eviction calculation. The ARGV `now_ns` is kept only as the member identifier to avoid key collisions.

---

## 11. Go Interface Design

### 11.1 New Wrapper: `SharedRateLimitedScorer`

This replaces the existing `rateLimitedScorer` when Redis is configured.

```
service/llm/services/
  LLMSharedRateLimitedScorer.go   ← new file
  LLMImplService.go               ← existing; kept for single-instance use
  LLMFactory.go                   ← updated to pick based on Redis config
  LLMCircuitBreaker.go            ← new file; CB state + transition logic
  LLMRedisLimiter.go              ← new file; Lua scripts + Redis ops
```

`LLMFactory.go` picks the wrapper based on whether `REDIS_ADDR` or `REDIS_SENTINEL_ADDRS` is set:
- Redis configured → `SharedRateLimitedScorer` (shared limits, shared CB)
- Redis not configured → existing `rateLimitedScorer` (local limits, local CB)

This keeps single-instance and multi-instance deployments working with zero config change for existing setups.

### 11.2 Updated `Options` Struct

```go
type Options struct {
    // Existing fields unchanged
    Provider    string
    APIKey      string
    Model       string
    TargetRoles []string
    YearsExp    int
    Limits      RateLimits
    Log         logger.Logger

    // Multi-provider failover chain (optional; if empty, single-provider behavior)
    FallbackProviders []ProviderConfig

    // Redis config (optional; if empty, use local limiter)
    Redis RedisConfig

    // Circuit breaker config (optional; uses defaults if zero)
    CircuitBreaker CircuitBreakerConfig
}

type ProviderConfig struct {
    Name   string
    APIKey string
    Model  string
    Limits RateLimits
}

type RedisConfig struct {
    Addr          string   // single node
    SentinelAddrs []string // HA mode
    SentinelMaster string
    Password      string
    DB            int
}

type CircuitBreakerConfig struct {
    FailureThreshold int           // default 5
    RecoveryTimeout  time.Duration // default 60s
    SuccessThreshold int           // default 2
    ProbeTimeout     time.Duration // default 30s
}
```

### 11.3 New Config Env Vars

Added to `backend/common/config/config.go` under a new `RedisConfig` field on the top-level `Config`:

| Env var | Default | Notes |
|---------|---------|-------|
| `REDIS_ADDR` | `""` | Single-node address; empty = no Redis |
| `REDIS_SENTINEL_ADDRS` | `""` | Comma-separated; takes precedence over `REDIS_ADDR` |
| `REDIS_SENTINEL_MASTER` | `mymaster` | |
| `REDIS_PASSWORD` | `""` | |
| `REDIS_DB` | `0` | |
| `LLM_CB_FAILURE_THRESHOLD` | `5` | Failures before OPEN |
| `LLM_CB_RECOVERY_TIMEOUT` | `60` | Seconds before half-open probe |
| `LLM_CB_SUCCESS_THRESHOLD` | `2` | Successes to close from half-open |
| `LLM_CB_PROBE_TIMEOUT` | `30` | Seconds the half-open probe lock holds |
| `LLM_CALL_TIMEOUT_SECONDS` | `60` | Per-call deadline |
| `LLM_FAILOVER_BASE_DELAY_MS` | `100` | Jitter base for provider failover |

---

## 12. Observability

New Prometheus metrics (constants added to `backend/common/metrics/`):

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `llm_rate_limit_wait_seconds` | Histogram | `provider`, `limit_type` (rpm/rpd/tpm) | Time spent waiting for quota |
| `llm_rate_limit_denied_total` | Counter | `provider`, `limit_type` | Requests denied (quota exhausted) |
| `llm_circuit_breaker_state` | Gauge | `provider` | 0=closed 1=open 2=half_open |
| `llm_circuit_breaker_transitions_total` | Counter | `provider`, `transition` (closed_open, open_half, half_closed, half_open) | CB state changes |
| `llm_provider_failover_total` | Counter | `from_provider`, `to_provider` | Failover events |
| `llm_token_estimation_error` | Histogram | `provider`, `call_type` (score/enrich) | `actual - estimated` tokens; tracks estimator drift |
| `redis_unavailable_total` | Counter | `operation` | Redis errors causing fallback to local limiter |

Log fields to add to `backend/common/logger/fields.go`:

```go
FieldProvider         = "llm_provider"
FieldCBState          = "cb_state"
FieldRateLimitType    = "rate_limit_type"
FieldTokensEstimated  = "tokens_estimated"
FieldTokensActual     = "tokens_actual"
```

---

## 13. File-by-File Scope

| File | Action |
|------|--------|
| `backend/service/llm/services/LLMRedisLimiter.go` | New — Redis client setup, Lua script loading, `AcquireSlot` / `RecordActualTokens` |
| `backend/service/llm/services/LLMCircuitBreaker.go` | New — CB state read/write via Redis hash, CAS transitions, half-open probe lock |
| `backend/service/llm/services/LLMSharedRateLimitedScorer.go` | New — implements `Scorer`, wraps provider chain, drives CB and rate limiter |
| `backend/service/llm/services/LLMFactory.go` | Updated — picks shared vs local wrapper based on Redis config |
| `backend/service/llm/services/LLMImplService.go` | Unchanged — kept as local fallback |
| `backend/common/config/config.go` | Updated — add `RedisConfig`, new LLM CB + failover fields |
| `backend/common/config/CLAUDE.md` | Updated — document new env vars |
| `backend/common/metrics/metrics.go` | Updated — new LLM rate limit and CB metrics |
| `backend/common/logger/fields.go` | Updated — new field constants |
| `backend/service/llm/services/tests/shared_rate_limiter_test.go` | New — uses `miniredis` to test Lua scripts |
| `backend/service/llm/services/tests/circuit_breaker_test.go` | New — CB state machine transitions |

---

## 14. What Does Not Change

- `JobsRelevanceFilter` and `JobPostEnricher` interfaces — callers (filter worker, analyzer worker) are unchanged
- Kafka topics and message formats — unchanged
- `job_pipeline_ledger` error handling rules — failed batches still mark `reason=error`, never re-queue
- Single-instance deployments with no `REDIS_ADDR` set — fall through to existing `rateLimitedScorer` unchanged
- The filter simplification design (`filter-simplification.md`) — pre-filter stage, score shape, threshold logic all unchanged; this design only replaces the rate limiting layer underneath `ScoreBatch`

---

## 15. Open Questions (Intentionally Deferred)

- **Per-model vs per-account quota:** Gemini free tier quotas are per-model (flash-lite has different limits than gemini-pro). The design currently namespaces by provider only. If two models on the same provider have independent quotas, a second key namespace (`rpm:{provider}:{model}`) would be needed. Deferred until a second model is added.
- **Quota lease expiry and GC:** dead instances leave estimated token debits in Redis that expire naturally. For very long-running requests (> 70s), the debit expires before the response arrives, and the correction write silently fails (key not found). A `SETNX` variant that recreates the adjustment key and sets a fresh TTL could handle this. Deferred — the `LLM_CALL_TIMEOUT_SECONDS=60s` guard makes this edge case unreachable by design.
- **Manual CB reset API:** an admin endpoint (`POST /admin/circuit-breaker/{provider}/reset`) that does `DEL cb:{provider}` would allow faster recovery without Redis CLI access. Deferred until Kubernetes deployment is required.
