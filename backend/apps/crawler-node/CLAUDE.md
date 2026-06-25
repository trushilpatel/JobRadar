# apps/crawler-node/ — Stateless crawler worker

Polls the orchestrator for company scan assignments, runs crawler-v2 ATS adapters to fetch job listings, and submits results back to the orchestrator. Horizontally scalable — add replicas via the Helm `crawlerNode.replicas` field.

## Key files

| File            | Role                                                                                                        |
| --------------- | ----------------------------------------------------------------------------------------------------------- |
| `main.go`       | Wiring — builds config, HTTP client, adapter executor, logger, then starts the poll loop                    |
| `pollloop.go`   | Core poll loop: `Run()` calls `GetWork`, acquires the semaphore, launches goroutines per assignment         |
| `executor.go`   | `Execute()` — resolves the adapter from the registry and runs `FetchJobListing`/`FetchJobListingWithConfig` |
| `client.go`     | `OrchestratorClient` HTTP implementation — `GetWork`, `ReportResult`, `ReportError`                         |
| `config.go`     | Reads env vars into `Config` struct                                                                         |
| `interfaces.go` | `OrchestratorClient` and `AdapterExecutor` narrow interfaces the poll loop depends on                       |
| `types.go`      | `WorkUnit`, `WorkResult`, `WorkError` data transfer types                                                   |

## Lifecycle

```
Run()
  ↓
GetWork(ctx, holding[])          ← sends in-progress IDs for heartbeat
  ↓ assignment received
acquire semaphore slot
  ↓
goroutine:
  Execute(ctx, assignment)
    ↓ success → ReportResult(ctx, result)
    ↓ error   → ReportError(ctx, workErr)
  release semaphore slot
  ↓
loop
```

## Semaphore (MaxConcurrent)

`MaxConcurrent` controls how many assignments one crawler-node holds simultaneously. Set via `MAX_CONCURRENT` env var (Helm `crawlerNode.maxConcurrent`). Never hardcode a concurrency ceiling in application code — concurrency is a deployment-time decision.

## Heartbeat

The `holding[]` slice in `pollloop.go` tracks the assignment IDs currently in progress. Every `GetWork` call sends this slice to the orchestrator so it can reset the heartbeat clock. If the node stops sending (crash, OOM-kill), the orchestrator's `runResetStuck` ticker reclaims those rows after `HeartbeatStaleDuration`.

## Configuration env vars

| Env var                   | Default | Notes                                    |
| ------------------------- | ------- | ---------------------------------------- |
| `ORCHESTRATOR_URL`        | —       | Base URL of the orchestrator HTTP server |
| `MAX_CONCURRENT`          | `3`     | Max simultaneous assignments per node    |
| `POLL_INTERVAL_SECONDS`   | `5`     | Seconds between GetWork polls when idle  |
| `ADAPTER_TIMEOUT_SECONDS` | `30`    | Per-adapter HTTP fetch timeout           |

## Scaling

Crawler-node count is controlled by `crawlerNode.replicas` in `k8s/helm/jobradar/values.local.yaml`. Run `make k8s-up` after changing replicas — no image rebuild needed (replicas are a Deployment field, not compiled in).

## Logger injection

The logger is injected via `NewPollLoop(cfg, client, exec, log)`. Do not access the `log` field directly outside `pollloop.go`. All log calls must use field key constants from `platform/logger/fields.go`.
