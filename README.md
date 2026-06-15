# JobRadar

An automated job tracking system for software engineers. Monitors company careers pages continuously, filters listings via LLM, enriches them with structured data, and surfaces only the relevant ones in a clean feed.

Built for job seekers who need visa sponsorship — jobs that explicitly deny sponsorship, require citizenship, or require security clearance are automatically discarded before they ever reach your feed.

---

## How It Works

```
company_scan_scheduler
        ↓  (DB queue)
orchestrator + crawler-node(s)  ← orchestrator assigns work; crawler-nodes fetch careers pages via crawler-v2 ATS adapters
        ↓  (NATS JetStream: jobs.raw.<adapter>)
job_listing_relevance_filter  ← LLM batch-scores job titles
        ↓  (NATS JetStream: jobs.filtered.<adapter>)
analyzer                  ← fetches full page, LLM enriches, eligibility check
        ↓  (Supabase: jobs)
REST API + React UI
```

---

## Features

### Job Feed

- Auto-populated from monitored company careers pages
- LLM-filtered by title relevance against your target roles and experience level
- Each job enriched with: tech stack, salary range, remote status, location, sponsorship flag, experience requirement, role category, responsibilities
- Status tracking: **New → Saved → Applied** (or Dismissed)
- Notes per job

### Eligibility Filtering

Jobs are automatically discarded if the posting:

- Explicitly denies H-1B / visa sponsorship
- Requires US Citizenship or Green Card
- Requires or asks candidate to obtain any security clearance
- Requires more experience than your configured `DISCARD_JOB_WITH_EXP_GT` limit

### Applications Tracker

- Track every job through pipeline stages
- Stages: Phone Screen → Technical Interview → Final Round → Offer → Rejected

### Company Management

- Add companies to monitor by careers URL
- Manual scan trigger per company
- Automatic scan scheduling via `SCAN_INTERVAL_MINUTES` (default: 24h)

### Dashboard

Live stats: jobs scanned today, new/saved/applied counts, pipeline pending, pipeline errors.

### Insights

Analytics over all enriched jobs: funnel, role categories, company hiring velocity, location breakdown, sponsorship trend, tech stack trends.

---

## Supported ATS Platforms

| Platform   | Adapter                                                                      |
| ---------- | ---------------------------------------------------------------------------- |
| Greenhouse | `backend/apps/crawler-v2/adapters/ats/greenhouse`                            |
| Lever      | `backend/apps/crawler-v2/adapters/ats/lever`                                 |
| Ashby      | `backend/apps/crawler-v2/adapters/ats/ashby`                                 |
| Apple      | `backend/apps/crawler-v2/adapters/ats/apple`                                 |
| Deel       | `backend/apps/crawler-v2/adapters/ats/deel`                                  |
| Eightfold  | `backend/apps/crawler-v2/adapters/ats/eightfold`                             |
| Gem        | `backend/apps/crawler-v2/adapters/ats/gem`                                   |
| Google     | `backend/apps/crawler-v2/adapters/ats/google` (pure HTTP + RE2 — no browser) |
| Meta       | `backend/apps/crawler-v2/adapters/ats/meta`                                  |
| Microsoft  | `backend/apps/crawler-v2/adapters/ats/microsoft`                             |
| Netflix    | `backend/apps/crawler-v2/adapters/ats/netflix`                               |
| Rippling   | `backend/apps/crawler-v2/adapters/ats/rippling`                              |
| Uber       | `backend/apps/crawler-v2/adapters/ats/uber`                                  |
| Workday    | `backend/apps/crawler-v2/adapters/ats/workday`                               |

---

## Stack

| Layer                   | Technology                            |
| ----------------------- | ------------------------------------- |
| Backend                 | Go                                    |
| Frontend                | React + TypeScript + Tailwind         |
| Database                | Supabase (PostgreSQL)                 |
| Message queue           | NATS JetStream (Synadia Cloud)        |
| LLM                     | Gemini (via Google Generative AI SDK) |
| Auth                    | Supabase (magic link / email OTP)     |
| Migrations              | goose                                 |
| Container orchestration | Kubernetes (k3d local) via Helm       |
| Reverse proxy           | Caddy (prod)                          |

---

## Running — Kubernetes (primary)

Kubernetes with Helm is the primary way to run JobRadar locally. Everything runs in a k3d cluster.

### Prerequisites

- [k3d](https://k3d.io) + [kubectl](https://kubernetes.io/docs/tasks/tools/) + [Helm 3](https://helm.sh/docs/intro/install/)
- Docker Desktop running

### 1. One-shot bootstrap (new machine)

Before running anything, make sure these files are in place:

| File                                  | Description                                                                                     |
| ------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `.env`                                | All env vars — copy from `.env.example` and fill in                                             |
| `secrets/jobradar_NATS.creds`         | Synadia Cloud credentials — download from Account → User → Get Connected → Download Credentials |
| `k8s/helm/jobradar/values.local.yaml` | Local Helm overrides — must set `filter.natsCredsFile` and `enricher.natsCredsFile`             |

Then run:

```bash
make k8s-bootstrap
```

This validates all required files, env vars, and credential files; creates the k3d cluster if needed; and creates every k8s secret automatically. It will fail with a clear message for anything missing — fix and re-run.

After bootstrap succeeds:

```bash
make k8s-build   # build + import images
make k8s-up      # deploy via Helm
```

### 2. What bootstrap checks

Each service needs a Kubernetes Secret with its sensitive env vars (DB credentials, API keys, auth secrets). The required vars mirror those in `.env.example`. `make k8s-bootstrap` creates them all — but here is the full list for reference:

```bash
# Example for the API service — repeat for each: scheduler, orchestrator, crawler-node, filter, enricher, migrate
kubectl create secret generic jobradar-api-secret \
  --from-literal=DB_HOST="..." \
  --from-literal=DB_USER="..." \
  --from-literal=DB_PASSWORD="..." \
  --from-literal=SUPABASE_JWKS_URL="..." \
  --from-literal=SUPABASE_URL="..." \
  --from-literal=SUPABASE_SERVICE_ROLE_KEY="..." \
  -n jobradar-local
```

Non-sensitive config (ports, log levels, etc.) comes from `values.yaml` via ConfigMaps — only secrets need to be created manually. Check each service's `templates/<svc>/configmap.yaml` to see what is already covered.

---

**NATS credentials (Synadia Cloud)**

Download from: Synadia Cloud → Account → User → Get Connected → Download Credentials

```
# File goes here (gitignored — never commit):
secrets/jobradar_NATS.creds
```

```bash
# Create the k8s secret from that file:
kubectl create secret generic nats-creds \
  --from-file=jobradar.creds=./secrets/jobradar_NATS.creds \
  -n jobradar-local \
  --dry-run=client -o yaml | kubectl apply -f -
```

```
# File to edit (gitignored — create if it doesn't exist):
k8s/helm/jobradar/values.local.yaml
```

```yaml
# Add these lines to k8s/helm/jobradar/values.local.yaml:
filter:
  natsCredsFile: '/etc/nats/jobradar.creds'

enricher:
  natsCredsFile: '/etc/nats/jobradar.creds'
```

> `natsUrl` and `nats.external` are already set to Synadia Cloud defaults in `values.yaml` — no override needed.

> Grafana Cloud credentials and the supabase-audit-sync secret are covered in [Step 5 — Deploy observability](#5-deploy-observability--grafana-cloud-optional) below.

### 3. Build and import images

```bash
make k8s-build          # builds backend + orchestrator + crawler-node + console images and imports into k3d
```

For the Supabase audit sync image:

```bash
docker build -t jobradar-supabase-audit-sync:local ./infra/observability/supabase-audit-sync/
k3d image import jobradar-supabase-audit-sync:local -c jobradar
```

### 4. Deploy the app

```bash
make k8s-up             # helm install jobradar chart (namespace → migrate → workers → api → console)
```

### 5. Deploy observability — Grafana Cloud (optional)

JobRadar ships **metrics → Mimir**, **logs → Loki**, and **traces → Tempo** to a Grafana Cloud stack via an in-cluster **Grafana Alloy** DaemonSet (chart: [`k8s/helm/observability/`](k8s/helm/observability/)). No self-hosted Prometheus / Loki / Grafana / Jaeger exists — Grafana Cloud is the only destination.

```bash
make k8s-obs-up         # deploys: Alloy DaemonSet + supabase-audit-sync sidecar
```

#### Bootstrapping on a new machine

Everything except the credentials file is in git. To bring observability up on a fresh clone:

```bash
# 1. Verify kube context
kubectl config use-context k3d-jobradar

# 2. Create the gitignored credentials file
cat > k8s/helm/observability/values.local.yaml <<'EOF'
grafanaCloud:
  prom:
    url: "https://prometheus-prod-XX-prod-XX-XXXX.grafana.net/api/prom/push"
    user: "<prom_instance_id>"      # numeric, from Connections → Hosted Prometheus
    token: "glc_<your-token>"
  loki:
    url: "https://logs-prod-XXX.grafana.net/loki/api/v1/push"
    user: "<loki_instance_id>"      # numeric, from Connections → Hosted Loki
    token: "glc_<your-token>"
  tempo:
    url: "https://otlp-gateway-prod-XX.grafana.net/otlp"
    user: "<stack_id>"              # ⚠️ STACK ID — NOT the Tempo wizard's "User" field
    token: "glc_<your-token>"
EOF

# 3. Deploy
make k8s-obs-up
```

**Getting the values** — sign in to grafana.com → your stack → **Connections** → click each of "Hosted Prometheus metrics", "Hosted Loki logs", "OpenTelemetry / OTLP". Each page shows the URL and the numeric Instance ID / Username.

**Getting the token** — easiest path is a single token with all three write scopes:

1. Cloud portal → **Cloud access policies** → **Create access policy**
2. Name `jobradar-alloy`, realm = your stack, scopes: tick `metrics:write`, `logs:write`, `traces:write`
3. **Add token** → copy the `glc_...` string (shown once)
4. Use the same token in all three `token:` fields above

**⚠️ Tempo username gotcha** — the OTLP gateway authenticates with the **stack ID** (visible in the token's metadata as `stack-NNNNNNN-...`), NOT the "User" number the Tempo wizard shows in its query/Explore section. Get it wrong and you'll see `HTTP 401 Unauthenticated` in `kubectl logs -n jobradar-local -l app.kubernetes.io/name=alloy`. Sanity check from your laptop: `curl -u "<stack_id>:<token>" -X POST <tempo_url>/v1/traces` → expect `HTTP 400` (auth OK, body invalid), not `HTTP 401`.

#### Per-node identification

Alloy runs as a **DaemonSet** — one pod per node — and tails only the pods on its own node. Every log line is automatically labelled with the node it came from:

| Label       | Value                     |
| ----------- | ------------------------- |
| `cluster`   | `k3d-jobradar`            |
| `namespace` | `jobradar-local`          |
| `node`      | The Kubernetes node name  |
| `pod`       | The source pod name       |
| `container` | The source container name |

In Grafana Cloud → Explore (Loki datasource), filter by node directly:

```logql
{cluster="k3d-jobradar", node="k3d-jobradar-server-0"}        # one node only
{cluster="k3d-jobradar"} | json | node="k3d-jobradar-agent-1" # cross-cluster query
```

For multi-node clusters, this gives you instant per-node visibility without any per-app code changes. New nodes auto-join: Kubernetes schedules an Alloy pod onto each new node, which reads the same `grafana-cloud-credentials` Secret and starts shipping immediately.

> Per-node metrics: currently Alloy scrapes services via cluster DNS (single-replica setup). If you scale a worker to multiple replicas across nodes, switch the scrape targets in [`alloy-config.yaml`](k8s/helm/observability/templates/alloy-config.yaml) from Service DNS to Kubernetes pod discovery with a `spec.nodeName = $HOSTNAME` filter — the logs section already uses that pattern; copy it.

#### Sharing credentials across team / machines

`values.local.yaml` is gitignored. Recommended ways to share:

| Approach                                                                                  | When                                         |
| ----------------------------------------------------------------------------------------- | -------------------------------------------- |
| Paste the 12-line YAML into 1Password / Bitwarden as a secure note                        | Small team sharing a vault                   |
| `sealed-secrets` (encrypted Secret committed to git, only cluster controller can decrypt) | Small team without external secret stores    |
| External Secrets Operator + AWS / GCP Secret Manager                                      | Multiple environments (dev / staging / prod) |

#### Rotating tokens

If a token leaks (e.g., pasted into chat / Slack), rotate via Cloud portal → **Cloud access policies** → `jobradar-alloy` → **Tokens** → **Rotate**. Update the three `token:` fields in `values.local.yaml`, then:

```bash
make k8s-obs-up                                              # re-applies the secret
kubectl rollout restart daemonset/obs-alloy -n jobradar-local
```

#### Supabase audit sync (optional sidecar)

The chart also deploys `supabase-audit-sync` — a tiny Go daemon that polls the Supabase auth audit table and pushes rows into Loki. It needs a separate image build + a DB-URL secret:

```bash
docker build -t jobradar-supabase-audit-sync:local ./infra/observability/supabase-audit-sync/
k3d image import jobradar-supabase-audit-sync:local -c jobradar
kubectl create secret generic supabase-audit-sync-secret \
  --from-literal=SUPABASE_DB_URL="postgres://postgres:PASSWORD@HOST.supabase.co:5432/postgres" \
  -n jobradar-local
kubectl rollout restart deployment/supabase-audit-sync -n jobradar-local
```

This is independent of the three observability pillars above — skip it if you don't need auth-audit-log shipping.

### Access

| Service      | URL                    | Notes               |
| ------------ | ---------------------- | ------------------- |
| Console (UI) | http://localhost:30300 | Direct via NodePort |
| API          | http://localhost:30080 | Direct via NodePort |

> If the cluster was created **without** the `--port` flags above, use `make k8s-forward` instead.

### Day-to-day commands

```bash
make k8s-status              # show all pod statuses
make k8s-logs                # tail logs for all app services
make k8s-logs-api            # tail a specific service (also: -crawler-node, -orchestrator, -filter, -enricher, -scheduler, -console)
make k8s-restart             # rolling restart all app deployments
make k8s-restart-api         # rolling restart one service
make k8s-migrate             # re-run DB migrations
make k8s-forward             # port-forward console → :3000, api → :8080 (if no NodePort mappings)
make k8s-down                # uninstall app chart (secrets and PVCs preserved)
make k8s-nuke                # fully wipe namespace + PVCs (use when storage class / StatefulSet spec changed)
make k8s-obs-down            # uninstall observability chart
```

### After a code change

```bash
make k8s-build-backend       # rebuild + reimport backend image (api/scheduler/filter/analyzer/migrate)
make k8s-build-orchestrator  # rebuild + reimport orchestrator image
make k8s-build-crawler-node  # rebuild + reimport crawler-node image
make k8s-build-console       # rebuild + reimport console image
make k8s-restart             # roll all deployments onto the new image
```

---

## Running — Active Development (hot reload)

For iterating quickly on the API or frontend without rebuilding containers:

```bash
make dev          # NATS JetStream in Docker; API on host :8080, Vite dev server :5173
```

---

## Configuration

One env file at the repo root covers everything — backend services, Vite dev server, Docker Compose, and infra:

```bash
cp .env.example .env
# edit .env with your Supabase project ref, API keys, etc.
```

### Key variables

| Variable                                                                | Description                                                    |
| ----------------------------------------------------------------------- | -------------------------------------------------------------- |
| `DB_HOST`                                                               | Supabase pooler host                                           |
| `DB_USER`                                                               | Supabase pooler user (`postgres.<project-ref>`)                |
| `DB_PASSWORD`                                                           | Supabase DB password                                           |
| `DB_SSLMODE`                                                            | Always `require`                                               |
| `SUPABASE_PROJECT_URL`                                                  | `https://<project-ref>.supabase.co`                            |
| `SUPABASE_URL` / `SUPABASE_SERVICE_ROLE_KEY`                            | Used by `backend/platform/supabaseadmin` for `DELETE /api/me`. |
| `RATE_LIMIT_READ_RPM` / `RATE_LIMIT_WRITE_RPM` / `RATE_LIMIT_ADMIN_RPM` | Per-user token-bucket rate limits (defaults: 30 / 20 / 120).   |
| `CACHE_TTL_USER_SECONDS`                                                | TTL for the user-cache decorator (default 60).                 |
| `TARGET_ROLES`                                                          | Comma-separated target role titles                             |
| `YEARS_EXP`                                                             | Your years of experience                                       |
| `DISCARD_JOB_WITH_EXP_GT`                                               | Drop jobs requiring more than N years (0 = disabled)           |
| `LLM_SLOTS_CONFIG`                                                      | Path to `llm-slots.yaml`                                       |
| `SCAN_INTERVAL_MINUTES`                                                 | How often each company is re-scanned                           |

### LLM slots (`k8s/helm/jobradar/files/llm-slots.yaml`)

Single source of truth — Helm renders this file directly into the `jobradar-llm-slots` ConfigMap via `.Files.Get`. Each slot entry is `{key_id, api_key (env-ref), model, rpm, rpd, tpm_in}`. Slots are grouped by worker (`filter:` / `enricher:`). To add a slot, edit the file; no values.yaml changes needed.

---

## Kubernetes Structure

```
k8s/helm/
  jobradar/             ← app chart (api, console, workers, redis, migrate)
    Chart.yaml
    values.yaml         ← all config defaults
    templates/          ← one directory per component
    examples/secrets/   ← secret templates (fill in and apply manually)
  observability/        ← observability chart (Grafana Alloy, ships to Grafana Cloud + audit-sync)
    Chart.yaml          ← depends on grafana/alloy
    values.yaml
    values.local.yaml.example  ← Grafana Cloud credentials template
    templates/
```

Helm releases in namespace `jobradar-local`:

| Release    | Chart                    | Manages                                                                                   |
| ---------- | ------------------------ | ----------------------------------------------------------------------------------------- |
| `jobradar` | `k8s/helm/jobradar`      | api, console, scheduler, orchestrator, crawler-node, filter, enricher (analyzer), migrate |
| `obs`      | `k8s/helm/observability` | Grafana Alloy (ships metrics/logs/traces to Grafana Cloud), supabase-audit-sync           |

---

## Auth

Login uses **Supabase magic link (email OTP)**. No password required.

Add allowed redirect URLs in Supabase → Authentication → URL Configuration:

- Local k8s: `http://localhost:30300/auth/callback`
- Local dev (Vite): `http://localhost:5173/auth/callback`
- Production: `https://<your-domain>/auth/callback`

### Multi-user

All three phases of the multi-user rollout have shipped — see [`Designs/multi-user-rollout-plan.md`](Designs/multi-user-rollout-plan.md).

- Tables: `users`, `roles`, `user_roles`, `admin_audit_log`. RBAC0 — roles `user` and `admin` seeded by migration `043`; no permissions table.
- Middleware (`backend/apps/api/middleware/`): `RequireIdentity` (JWT verify + DB-backed enrichment, cache-aside, lazy provisioning, suspended-user check), `RequireRole(role)`, `RateLimit(limiter)` (per-user token bucket, fail-open on Redis outage, 429 + `Retry-After`), `AuditAdminMutation(svc)` (appends 2xx admin mutations to `admin_audit_log`).
- Route split: `/api/*` for authenticated users, `/admin/*` for admins only.
- Identity flows via `context.Context`; services pull `userID` via `identity.FromContext` / `UserIDFrom` / `IsAdmin`. Postgres `user_roles` is the source of truth — never JWT custom claims.
- Per-user overlay (`user_job_state`) for status, notes, dismissals; the global `jobs` feed joins per-user state on every request.
- Self-service endpoints: `GET /api/me`, `PATCH /api/me` (display_name only), `DELETE /api/me` (Supabase admin REST → DB cascade).
- Frontend: identity-aware Zustand auth store, role-gated admin nav, 429 toast handling.

---

## Git workflow

- **Never commit to `main` directly.** `.husky/pre-commit` blocks it — branch off (`git checkout -b <feature>`) before you start.
- **Never bypass the pre-commit hook with `git commit --no-verify`** (or any equivalent: `--no-gpg-sign`, custom `core.hooksPath`, etc.). This applies to every commit, including doc-only changes and hotfixes.
- If the hook fails, fix the failing check (formatter, lint, tests) and recommit. Bypassing is not an option.
- Server-side branch protection on `main` is the real gate — the local hook just catches mistakes earlier.

## Useful commands

```bash
make migrate          # run pending DB migrations (local Go binary)
make db-truncate      # truncate all data except companies (requires DB_URL env var)
make test             # run all Go tests
make lint             # ESLint on console
make lint-go          # golangci-lint on backend
make fmt              # gofmt all Go files
```
