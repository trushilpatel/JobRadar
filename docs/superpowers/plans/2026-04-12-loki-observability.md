# Loki Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add centralised log aggregation to JobRadar so all 5 service logs are searchable in Grafana via Loki + Promtail — no Go code changes required.

**Architecture:** Promtail connects to the Docker socket, discovers all running JobRadar containers, and ships their stdout logs (already structured zerolog JSON) to Loki. Grafana is auto-provisioned with Loki as a data source so no manual setup is needed after `docker-compose up`.

**Tech Stack:** Loki 2.9.0, Promtail 2.9.0, Grafana 10.4.0 — all run as Docker Compose services alongside the existing stack.

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| Create | `backend/deploy/loki/loki-config.yml` | Loki server settings: storage path, retention (7 days), compactor |
| Create | `backend/deploy/loki/promtail-config.yml` | Promtail: Docker socket discovery, label extraction, push to Loki |
| Create | `backend/deploy/loki/grafana-datasources.yml` | Grafana provisioning: auto-registers Loki as default data source |
| Modify | `docker-compose.yml` | Add `loki`, `promtail`, `grafana` services + `lokidata`, `grafanadata` volumes |

---

## Task 1: Create the Loki config file

**Why:** Loki needs to know where to store log data, how long to keep it, and what port to listen on. We use filesystem storage (simplest for local/personal use) and 7-day retention.

**Files:**
- Create: `backend/deploy/loki/loki-config.yml`

- [ ] **Step 1: Create the config file**

Create `backend/deploy/loki/loki-config.yml` with this exact content:

```yaml
auth_enabled: false

server:
  http_listen_port: 3100
  grpc_listen_port: 9096

common:
  instance_addr: 127.0.0.1
  path_prefix: /loki
  storage:
    filesystem:
      chunks_directory: /loki/chunks
      rules_directory: /loki/rules
  replication_factor: 1
  ring:
    kvstore:
      store: inmemory

schema_config:
  configs:
    - from: 2020-10-24
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h

limits_config:
  retention_period: 168h

compactor:
  working_directory: /loki/compactor
  retention_enabled: true
  retention_delete_delay: 2h
```

**Key fields explained:**
- `auth_enabled: false` — no authentication needed (single-user local setup)
- `http_listen_port: 3100` — port Loki listens on inside the container
- `path_prefix: /loki` — root directory for all Loki data (maps to the `lokidata` Docker volume)
- `schema_config` — tells Loki how to structure its internal index (v13 is the current stable schema)
- `retention_period: 168h` — 7 days × 24 hours; older logs are deleted automatically

- [ ] **Step 2: Verify the file exists**

```bash
ls backend/deploy/loki/
```

Expected output: `loki-config.yml`

- [ ] **Step 3: Commit**

```bash
git add backend/deploy/loki/loki-config.yml
git commit -m "feat(observability): add Loki server config"
```

---

## Task 2: Create the Promtail config file

**Why:** Promtail needs to know (a) where to find logs — Docker socket, (b) what labels to attach — container name, compose service, log level, and (c) where to send them — Loki's HTTP push endpoint.

**Files:**
- Create: `backend/deploy/loki/promtail-config.yml`

- [ ] **Step 1: Create the config file**

Create `backend/deploy/loki/promtail-config.yml` with this exact content:

```yaml
server:
  http_listen_port: 9080
  grpc_listen_port: 0

positions:
  filename: /tmp/positions.yaml

clients:
  - url: http://loki:3100/loki/api/v1/push

scrape_configs:
  - job_name: docker
    docker_sd_configs:
      - host: unix:///var/run/docker.sock
        refresh_interval: 5s
    relabel_configs:
      - source_labels: ['__meta_docker_container_name']
        regex: '/(.*)'
        target_label: 'container_name'
      - source_labels: ['__meta_docker_container_label_com_docker_compose_service']
        target_label: 'compose_service'
      - source_labels: ['container_name']
        regex: 'jobradar_loki|jobradar_promtail|jobradar_grafana'
        action: drop
    pipeline_stages:
      - docker: {}
      - json:
          expressions:
            level: level
            service: service
      - labels:
          level:
          service:
```

**Key sections explained:**

`positions.filename` — Promtail saves its read position in each log file here. If Promtail restarts it resumes from where it left off, not from the beginning.

`clients` — where to push logs. `http://loki:3100` uses Docker's internal DNS — containers talk to each other by service name.

`docker_sd_configs` — "service discovery" via Docker socket. `refresh_interval: 5s` means Promtail checks for new/removed containers every 5 seconds.

`relabel_configs` — transforms raw Docker metadata into Loki labels:
- First rule: strips the leading `/` from container names (Docker names them `/jobradar_crawler`, we want `jobradar_crawler`)
- Second rule: copies the Docker Compose service label (e.g. `crawler`) into a `compose_service` label
- Third rule: drops logs from the observability containers themselves — you don't need to see Loki's own logs in Loki

`pipeline_stages`:
- `docker: {}` — unwraps Docker's JSON wrapper (`{"log":"...","stream":"stdout","time":"..."}`) and extracts just the inner log line
- `json` — parses your zerolog JSON output and pulls out the `level` and `service` fields
- `labels` — promotes `level` and `service` to Loki labels, enabling queries like `{level="error"}` or `{service="crawler"}`

- [ ] **Step 2: Verify the file exists**

```bash
ls backend/deploy/loki/
```

Expected output: `loki-config.yml  promtail-config.yml`

- [ ] **Step 3: Commit**

```bash
git add backend/deploy/loki/promtail-config.yml
git commit -m "feat(observability): add Promtail Docker socket config"
```

---

## Task 3: Create the Grafana datasource provisioning file

**Why:** Without provisioning, you'd have to manually add Loki as a data source in the Grafana UI every time you recreate the container. This file tells Grafana to auto-register Loki on startup so it's ready immediately.

**Files:**
- Create: `backend/deploy/loki/grafana-datasources.yml`

- [ ] **Step 1: Create the provisioning file**

Create `backend/deploy/loki/grafana-datasources.yml` with this exact content:

```yaml
apiVersion: 1

datasources:
  - name: Loki
    type: loki
    access: proxy
    url: http://loki:3100
    isDefault: true
    editable: true
```

**Key fields explained:**
- `access: proxy` — Grafana talks to Loki server-side (not from your browser directly). This is always what you want when both are in the same Docker network.
- `url: http://loki:3100` — again using Docker internal DNS
- `isDefault: true` — when you open Grafana Explore, Loki is pre-selected
- `editable: true` — you can still modify it in the UI if needed

- [ ] **Step 2: Verify all three config files exist**

```bash
ls backend/deploy/loki/
```

Expected output: `grafana-datasources.yml  loki-config.yml  promtail-config.yml`

- [ ] **Step 3: Commit**

```bash
git add backend/deploy/loki/grafana-datasources.yml
git commit -m "feat(observability): add Grafana Loki datasource provisioning"
```

---

## Task 4: Add observability services to docker-compose.yml

**Why:** This wires the three new containers into the existing stack — same Docker network, same lifecycle management via `docker-compose up/down`.

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: Add the three new services**

Open `docker-compose.yml`. Find the `# ── Frontend` comment block. Add the following **before** the frontend service:

```yaml
  # ── Observability ─────────────────────────────────────────────────────────────

  loki:
    image: grafana/loki:2.9.0
    container_name: jobradar_loki
    ports:
      - "3100:3100"
    volumes:
      - ./backend/deploy/loki/loki-config.yml:/etc/loki/local-config.yaml
      - lokidata:/loki
    command: -config.file=/etc/loki/local-config.yaml
    restart: unless-stopped

  promtail:
    image: grafana/promtail:2.9.0
    container_name: jobradar_promtail
    volumes:
      - ./backend/deploy/loki/promtail-config.yml:/etc/promtail/config.yml
      - /var/run/docker.sock:/var/run/docker.sock:ro
    command: -config.file=/etc/promtail/config.yml
    depends_on:
      - loki
    restart: unless-stopped

  grafana:
    image: grafana/grafana:10.4.0
    container_name: jobradar_grafana
    ports:
      - "3001:3000"
    volumes:
      - ./backend/deploy/loki/grafana-datasources.yml:/etc/grafana/provisioning/datasources/loki.yml
      - grafanadata:/var/lib/grafana
    environment:
      - GF_AUTH_ANONYMOUS_ENABLED=true
      - GF_AUTH_ANONYMOUS_ORG_ROLE=Admin
    depends_on:
      - loki
    restart: unless-stopped

```

**Key points:**
- `loki` exposes port 3100 so you can also hit it directly for debugging (`curl localhost:3100/ready`)
- `promtail` mounts the Docker socket read-only (`:ro`) — it only needs to read, never write
- `grafana` maps container port 3000 to host port 3001 (3000 is taken by your frontend)
- `GF_AUTH_ANONYMOUS_ENABLED=true` skips the login screen for local dev — open `localhost:3001` and you're in immediately
- Both config files are mounted as volumes so changes to the YAML take effect on container restart without rebuilding images

- [ ] **Step 2: Add the two new volumes**

Find the `volumes:` block at the bottom of `docker-compose.yml` (currently has `pgdata` and `kafkadata`). Add the two new volumes:

```yaml
volumes:
  pgdata:
  kafkadata:
  lokidata:
  grafanadata:
```

- [ ] **Step 3: Verify docker-compose syntax is valid**

```bash
docker compose config --quiet
```

Expected: no output (silent = valid). Any output means there's a syntax error — check indentation.

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yml
git commit -m "feat(observability): add Loki, Promtail, Grafana to docker-compose"
```

---

## Task 5: Start the stack and verify logs appear in Grafana

**Why:** Confirm the full pipeline works end-to-end: containers start → Promtail discovers them → logs flow to Loki → Grafana shows them.

- [ ] **Step 1: Pull the new images**

```bash
docker compose pull loki promtail grafana
```

Expected: Docker downloads the three images. Takes 1-2 minutes on first run.

- [ ] **Step 2: Start the observability services**

```bash
docker compose up -d loki promtail grafana
```

Expected output: three lines saying `Container jobradar_loki started`, `jobradar_promtail started`, `jobradar_grafana started`.

- [ ] **Step 3: Verify Loki is healthy**

```bash
curl -s localhost:3100/ready
```

Expected output: `ready`

If you see `connection refused`, wait 10 seconds and retry — Loki takes a moment to initialise.

- [ ] **Step 4: Verify Promtail is running and connected**

```bash
docker compose logs promtail | tail -20
```

Expected: log lines showing Promtail discovered containers, e.g.:
```
level=info msg="added entry to target" container_name=jobradar_api
level=info msg="added entry to target" container_name=jobradar_crawler
```

If you see `connection refused` errors pointing at Loki, wait a few more seconds — Promtail retries automatically.

- [ ] **Step 5: Open Grafana**

Open `http://localhost:3001` in your browser.

You should land directly on the Grafana home page without a login prompt (anonymous auth is enabled).

- [ ] **Step 6: Query logs in Grafana Explore**

1. Click the compass icon in the left sidebar (**Explore**)
2. The data source dropdown at the top should already show **Loki**
3. In the query box, type:

```
{compose_service="api"}
```

4. Click **Run query** (or press Shift+Enter)

Expected: log lines from your API service appear in the panel below, with timestamps.

Try a few more queries to see the labelling in action:

```logql
# All logs across all services
{container_name=~"jobradar_.+"}

# Only error-level logs
{container_name=~"jobradar_.+"} | json | level="error"

# Crawler logs containing a specific job_id (replace with a real ID from your logs)
{compose_service="crawler"} | json | job_id="some-uuid"
```

- [ ] **Step 7: Confirm labels are visible**

In the Grafana Explore view, click **Label browser** (next to the query box). You should see:
- `compose_service` with values: `api`, `crawler`, `filter`, `enricher`, `scheduler`
- `level` with values: `debug`, `info`, `warn`, `error`
- `container_name` with all `jobradar_*` values

If `level` and `service` labels are missing, check the Promtail logs:
```bash
docker compose logs promtail | grep -i "pipeline\|error"
```

---

## Cheat Sheet: Useful LogQL Queries

Once everything is running, bookmark these:

| Goal | Query |
|---|---|
| All logs, all services | `{container_name=~"jobradar_.+"}` |
| One service | `{compose_service="crawler"}` |
| Errors only | `{container_name=~"jobradar_.+"} \| json \| level="error"` |
| Specific job trace | `{container_name=~"jobradar_.+"} \| json \| job_id="<uuid>"` |
| Count errors per service (rate) | `sum by (compose_service) (count_over_time({container_name=~"jobradar_.+"} \| json \| level="error" [5m]))` |

---

## Troubleshooting

**Promtail can't connect to Docker socket:**
```bash
docker compose logs promtail | grep "error"
```
On macOS, the Docker socket path might differ. If you see permission errors, check:
```bash
ls -la /var/run/docker.sock
```

**No logs appearing in Grafana:**
- Make sure the other services are running: `docker compose ps`
- Check Promtail discovered them: `docker compose logs promtail | grep "added entry"`
- Wait 30 seconds after starting — Promtail buffers before first push

**Grafana shows "Data source connected but no labels received":**
- Loki may not have received any logs yet. Generate some by hitting the API: `curl localhost:8080/health`
- Then re-query in Grafana
