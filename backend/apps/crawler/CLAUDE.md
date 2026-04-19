# apps/crawler/ — Careers page crawler

Polls `company_scan_queue` (DB), fetches careers pages via ATS adapters, publishes raw jobs to `job_listings.raw` (Kafka).

## Key files

| File             | Role                                                                 |
| ---------------- | -------------------------------------------------------------------- |
| `main.go`        | Wiring + poll loop — claims queue items, runs engine, ticks heartbeat |
| `engine.go`      | `Engine` — orchestrates one company scan end-to-end                  |
| `interfaces.go`  | Narrow interfaces the engine depends on                              |

## ATS adapters (`ats/`)

One sub-package per platform. Each implements the `Adapter` interface (list jobs) and optionally `ContentFetcher` (fetch full job detail via native API).

| Adapter      | Method          |
| ------------ | --------------- |
| `greenhouse` | JSON API        |
| `lever`      | JSON API        |
| `ashby`      | GraphQL API     |
| `google`     | Playwright scraper (see `ats/google/scraper/`) |
| `netflix`    | HTML scrape     |
| `uber`       | JSON API        |
| others       | HTML scrape     |

`ats/registry.go` maps adapter name strings → adapter constructors. Adding a new ATS = new sub-package + one line in the registry.

## Queue mechanics

- `ClaimNext` uses `FOR UPDATE SKIP LOCKED` — safe for concurrent crawler instances
- `UpdateHeartbeat` ticks every `HEARTBEAT_INTERVAL_SECONDS` to keep the row alive
- `ResetStuck` reclaims rows whose heartbeat expired (previous crash recovery)
- `Complete` marks the row done after a successful scan

## Rules

- Never fetch job content in the crawler — publish `RawJob` to Kafka and let the analyzer fetch it
- One company per queue claim — no bulk processing in a single goroutine
- `companyDelay` paces claims to avoid hammering ATS APIs back-to-back
