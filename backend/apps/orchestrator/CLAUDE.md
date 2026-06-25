# apps/orchestrator/ — Crawler orchestrator HTTP server

HTTP server on `:8082`. Assigns `company_scan_queue` rows to crawler-nodes, receives crawl results, and publishes raw job listings to NATS JetStream.

## Key files

| File            | Role                                                                                                |
| --------------- | --------------------------------------------------------------------------------------------------- |
| `main.go`       | Wiring + HTTP server startup + `runResetStuck` background ticker + graceful shutdown                |
| `middleware/`   | Node token authentication — verifies bearer tokens against `crawler_nodes.token_hash` (HMAC-SHA256) |
| `nodeidentity/` | Context helpers — injects and reads the authenticated crawler-node identity from `context.Context`  |

## Route structure

| Group                    | Purpose                                                                                   |
| ------------------------ | ----------------------------------------------------------------------------------------- |
| `/node/*`                | Crawler-node work polling (`GET /node/work`) and result submission (`POST /node/results`) |
| `/admin/crawler-nodes/*` | Node management — register, list, and decommission crawler-node records                   |

## Background ticker — runResetStuck

Runs every **5 minutes**. Calls `ResetStuckForNodes(ctx, HeartbeatStaleDuration)` to reclaim `company_scan_queue` rows whose heartbeat has expired (i.e., the crawler-node that claimed them crashed or went offline). Reclaimed rows return to `pending` state and are reassigned on the next poll.

## NATS publishing

After receiving a successful crawl result via `POST /node/results`, the orchestrator publishes each raw job listing to `jobs.raw.<adapter>` on the `JOBS_RAW` JetStream stream. The filter worker consumes from this stream.

## Graceful shutdown

Listens for SIGINT/SIGTERM. On signal: stops accepting new requests and waits up to **10 seconds** for in-flight requests to complete before exiting.

## How to add a new route

1. Add handler logic in `service/crawlernode/` (domain logic) or inline in a new handler file under `apps/orchestrator/` (wiring only — no business logic in `main.go`)
2. Wire the new handler into `main.go` (inject dependencies via constructor)
3. Register the route on the chi router in `main.go` — use `/node/` for crawler-node routes, `/admin/` for admin routes
4. Add node-auth middleware (`NodeAuth`) to any `/node/*` route; add admin-auth to any `/admin/*` route
5. Add handler tests covering the new route's success and error paths
