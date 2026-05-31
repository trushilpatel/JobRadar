# apps/crawler/ — Careers page crawler

Polls `company_scan_queue` (DB), fetches careers pages via ATS adapters, publishes raw jobs to `jobs.raw.<adapter>` (NATS JetStream).

## Key files

| File            | Role                                                                  |
| --------------- | --------------------------------------------------------------------- |
| `main.go`       | Wiring + poll loop — claims queue items, runs engine, ticks heartbeat |
| `engine.go`     | `Engine` — orchestrates one company scan end-to-end                   |
| `interfaces.go` | Narrow interfaces the engine depends on                               |

## ATS adapters (`ats/`)

One sub-package per platform. Each implements the `Adapter` interface (list jobs) and optionally `ContentFetcher` (fetch full job detail via native API).

| Adapter      | Method                                                    |
| ------------ | --------------------------------------------------------- |
| `greenhouse` | JSON API                                                  |
| `lever`      | JSON API                                                  |
| `ashby`      | GraphQL API                                               |
| `google`     | Pure HTTP + RE2 regex (`ats/google/http.go`) — no browser |
| `netflix`    | HTML scrape                                               |
| `uber`       | JSON API                                                  |
| others       | HTML scrape                                               |

`ats/registry.go` maps adapter name strings → adapter constructors. Adding a new ATS = new sub-package + one line in the registry.

## Queue mechanics

- `ClaimNext` uses `FOR UPDATE SKIP LOCKED` — safe for concurrent crawler instances
- `UpdateHeartbeat` ticks every `HEARTBEAT_INTERVAL_SECONDS` to keep the row alive. It returns `ErrQueueRowMissing` if the row is gone (a concurrent `ResetStuck` reassigned it) — the crawler treats that as a **reassignment signal, aborts the current scan, skips `Complete`, and increments `crawler_scan_aborted_reassigned_total`**. This prevents two workers double-scanning the same company.
- `ResetStuck` reclaims rows whose heartbeat expired (previous crash recovery); returns the reset count for observability
- `Complete`/`Remove` mark/delete the row; both return `ErrQueueRowMissing` on 0 rows rather than silently no-op'ing

## Publish ordering (closes the filter phantom-row race)

`publishNewJobs` writes the `reason='fetch'` ledger rows via `MarkSeenBatch` **BEFORE** publishing to `jobs.raw.*`, so the ledger row always exists before the filter can consume the message. (The reverse order let a fast filter UPDATE-against-a-missing-row, silently no-op, and Ack — leaving a phantom `fetch` row.) If an individual publish fails, the row stays at `reason='fetch'`, `crawler_publish_failures_total{adapter}` increments, and the next scan re-discovers it via `HasSeenBatch`. Do not move `MarkSeenBatch` back after the publish loop.

## Rules

- Never fetch job content in the crawler — publish `RawJob` to NATS and let the analyzer fetch it
- One company per queue claim — no bulk processing in a single goroutine
- Inter-company delay is enforced at the DB level — `ClaimNext` checks `adapter_configs.inter_company_delay_s` before claiming the next row; no sleep loop is needed in the worker
