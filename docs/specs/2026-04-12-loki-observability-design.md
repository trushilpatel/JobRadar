# Loki Observability Design

**Date:** 2026-04-12
**Scope:** Add centralised log aggregation to JobRadar using Loki + Promtail + Grafana via Docker Compose. No Go code changes required.

---

## Goal

Provide a single UI (Grafana) where logs from all 5 JobRadar services (api, scheduler, crawler, filter, enricher) can be searched, filtered, and correlated in real time.

---

## Architecture

```
Docker containers (api, crawler, filter, scheduler, enricher)
    │  stdout/stderr (structured JSON via zerolog)
    ▼
Promtail  ←── discovers containers via /var/run/docker.sock
    │  ships log lines + labels over HTTP
    ▼
Loki  (port 3100)  ←── stores + indexes logs
    ▲
Grafana  (port 3001)  ←── queries Loki, displays logs in UI
```

No changes to Go code. All existing zerolog JSON output is the data source.

---

## New Containers

| Container | Image | Port | Role |
|---|---|---|---|
| `loki` | `grafana/loki:2.9.0` | 3100 (internal) | Log storage and query engine |
| `promtail` | `grafana/promtail:2.9.0` | none | Log collector, ships to Loki |
| `grafana` | `grafana/grafana:10.4.0` | 3001 → 3000 | Visualisation UI |

Grafana uses port 3001 on the host because 3000 is taken by the JobRadar frontend.

---

## New Files

| File | Purpose |
|---|---|
| `deploy/loki/loki-config.yml` | Loki server config: storage path, retention, compactor |
| `deploy/loki/promtail-config.yml` | Promtail config: Docker socket discovery, label extraction, Loki push target |

---

## Promtail Config Detail

Promtail is configured to:

1. **Discover containers** via `docker_sd_configs` using `/var/run/docker.sock`
2. **Attach labels** from Docker metadata:
   - `container_name` — e.g. `jobradar_crawler`
   - `compose_service` — e.g. `crawler`
   - `image` — e.g. `jobradar_backend`
3. **Drop internal containers** — filter out `loki`, `promtail`, `grafana` themselves
4. **Unwrap Docker log format** — extract the inner `log` field from Docker's JSON wrapper
5. **Parse log level** — extract the `level` field from zerolog JSON and promote it to a Loki label, enabling `{level="error"}` queries

---

## Loki Config Detail

Loki is configured with:

- **Filesystem storage** — logs stored in a Docker volume (`lokidata`)
- **Single-binary mode** (`target: all`) — appropriate for single-node dev/personal use
- **7-day retention** — compactor deletes logs older than 7 days
- **No authentication** — single-user setup, internal Docker network only

---

## Grafana Config Detail

- Loki added as a provisioned data source (auto-configured on first start via a provisioning config file)
- No manual setup required — open Grafana, Loki is already connected
- Access at `http://localhost:3001` (default credentials: admin / admin)

---

## Docker Compose Changes

Three new service blocks added to `docker-compose.yml`:

- `loki` — depends on nothing, always starts
- `promtail` — depends on `loki`, mounts Docker socket read-only
- `grafana` — depends on `loki`, provisioned with Loki data source

One new volume: `lokidata` for Loki's log storage.

---

## Querying in Grafana

After setup, basic queries in Grafana → Explore:

| What you want | LogQL query |
|---|---|
| All crawler logs | `{compose_service="crawler"}` |
| All error logs across services | `{compose_service=~".+"} \| json \| level="error"` |
| Crawler errors only | `{compose_service="crawler"} \| json \| level="error"` |
| Logs for a specific job | `{compose_service=~".+"} \| json \| job_id="abc-123"` |

The last query works because your zerolog output already includes `job_id` as a structured field — no code changes needed.

---

## What is NOT in scope

- Prometheus metrics (next phase)
- Alerting rules in Grafana
- Log-based derived metrics
- TLS or authentication on Grafana/Loki
- Tracing (OpenTelemetry)

---

## Implementation Steps (high level)

1. Create `deploy/loki/loki-config.yml`
2. Create `deploy/loki/promtail-config.yml`
3. Create `deploy/loki/grafana-datasources.yml` (Grafana provisioning)
4. Add `loki`, `promtail`, `grafana` services to `docker-compose.yml`
5. Add `lokidata` volume to `docker-compose.yml`
6. Start the stack and verify logs appear in Grafana
