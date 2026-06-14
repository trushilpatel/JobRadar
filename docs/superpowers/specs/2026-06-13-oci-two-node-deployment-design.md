# OCI Two-Node Production Deployment

**Date:** 2026-06-13
**Status:** Approved — pending implementation
**Supersedes:** `2026-05-23-ghcr-flux-cd-gitops-design.md` (Flux CD rejected; too heavy for 1 GB nodes)

---

## Goal

Deploy the full JobRadar stack across two Oracle Cloud Free Tier ARM A1 instances (1 GB RAM each),
with automated CI/CD on every push to `main` and full observability via the existing Grafana Cloud
stack. Optimize so only changed images are rebuilt, only affected nodes are deployed, and only pods
whose image tag changed are restarted.

---

## Instance Roles

All inter-service communication routes through external managed services (Supabase, Synadia Cloud
NATS, Upstash Redis). The only in-cluster dependency is `crawler-node → orchestrator` — both live on
Node 2. The two nodes therefore have zero cross-node traffic.

### Node 1 — Serving

Hosts user-facing services. Needs low latency and stable uptime.

| Service                 | Memory request | Memory limit |
| ----------------------- | -------------- | ------------ |
| API (Chi REST)          | 64 Mi          | 128 Mi       |
| Console (nginx + React) | 32 Mi          | 64 Mi        |
| Grafana Alloy           | ~150 MB        | —            |
| k3s + OS                | ~250 MB        | —            |
| **Estimated total**     | **~500 MB**    | —            |

### Node 2 — Pipeline

Hosts the background job engine. Downtime here does not affect the UI; NATS buffers messages and the
scheduler re-enqueues on next tick.

| Service             | Memory request | Memory limit |
| ------------------- | -------------- | ------------ |
| Scheduler           | 64 Mi          | 128 Mi       |
| Orchestrator        | 64 Mi          | 128 Mi       |
| CrawlerNode ×3      | 192 Mi         | 576 Mi       |
| Filter              | 32 Mi          | 128 Mi       |
| Enricher            | 32 Mi          | 128 Mi       |
| Grafana Alloy       | ~150 MB        | —            |
| k3s + OS            | ~250 MB        | —            |
| **Estimated total** | **~784 MB**    | —            |

Note: sum of pod limits (~1,088 Mi) exceeds the 1 GB ceiling. This is intentional — Go binaries are
lean and actual CrawlerNode RSS is ~20–30 MB each. If OOM events appear in Grafana, drop CrawlerNode
replicas from 3 → 2.

---

## CI/CD Pipeline

**Trigger:** push to `main`

**Principle:** build only what changed, deploy only nodes affected, restart only pods whose image tag
changed.

### Workflow job graph

```
[detect-changes]
      │
      ├─ backend_changed? ──► [build-backend]      ─┐
      ├─ crawler_changed? ──► [build-crawler]      ─┤ all parallel,
      ├─ orch_changed?    ──► [build-orchestrator] ─┤ each conditional
      ├─ node_changed?    ──► [build-crawler-node] ─┤ on its filter
      └─ console_changed? ──► [build-console]      ─┘
                                                    │
                     ┌──────────────────────────────┤
                     ▼ if node1_changed             ▼ if node2_changed
               [deploy-node1]               [deploy-node2]
               SSH → helm upgrade           SSH → helm upgrade
               (parallel, independent)
```

A push that only touches `frontend/` rebuilds only `jobradar-console`, deploys only Node 1, and
restarts only Console pods. Node 2 is never touched.

### Change detection paths

The `detect-changes` job uses `dorny/paths-filter` (already used in `backend.yml`) with these path
mappings:

| Filter output     | Paths that trigger it                                                                                                                                                                                                                                 |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `backend_changed` | `backend/apps/api/**`, `backend/apps/filter/**`, `backend/apps/enricher/**`, `backend/apps/scheduler/**`, `backend/apps/migrate/**`, `backend/service/**`, `backend/platform/**`, `backend/common/**`, `go.mod`, `go.sum`, `infra/Dockerfile.backend` |
| `crawler_changed` | `backend/apps/crawler/**`, `backend/service/**`, `backend/platform/**`, `backend/common/**`, `go.mod`, `go.sum`, `infra/Dockerfile.crawler`                                                                                                           |
| `orch_changed`    | `backend/apps/orchestrator/**`, `backend/service/**`, `backend/platform/**`, `backend/common/**`, `go.mod`, `go.sum`, `infra/Dockerfile.orchestrator`                                                                                                 |
| `node_changed`    | `backend/apps/crawler-node/**`, `backend/service/**`, `backend/platform/**`, `backend/common/**`, `go.mod`, `go.sum`, `infra/Dockerfile.crawler-node`                                                                                                 |
| `console_changed` | `frontend/**`, `frontend/Dockerfile`                                                                                                                                                                                                                  |

`backend/service/`, `backend/platform/`, `backend/common/`, and `go.mod` changes set **all four**
backend image filters to `true` — shared code changes trigger a full backend rebuild.

Derived node-level flags (computed as `detect-changes` job outputs):

```
node1_changed = backend_changed OR console_changed
node2_changed = backend_changed OR crawler_changed OR orch_changed OR node_changed
```

### Image registry and tags

Five independent images, dual-tagged on every build:

| `values.yaml` key     | GHCR path                                    | Dockerfile                | Contains                                  |
| --------------------- | -------------------------------------------- | ------------------------- | ----------------------------------------- |
| `images.backend`      | `ghcr.io/trushilpatel/jobradar-backend`      | `Dockerfile.backend`      | api, scheduler, filter, enricher, migrate |
| `images.crawler`      | `ghcr.io/trushilpatel/jobradar-crawler`      | `Dockerfile.crawler`      | crawler                                   |
| `images.orchestrator` | `ghcr.io/trushilpatel/jobradar-orchestrator` | `Dockerfile.orchestrator` | orchestrator                              |
| `images.crawlerNode`  | `ghcr.io/trushilpatel/jobradar-crawler-node` | `Dockerfile.crawler-node` | crawler-node                              |
| `images.console`      | `ghcr.io/trushilpatel/jobradar-console`      | `frontend/Dockerfile`     | React + nginx                             |

Every build pushes two tags:

- `:<sha>` (7-char git SHA) — immutable, used in `helm upgrade --set`, enables `helm rollback`
- `:latest` — convenience alias, always points to the newest build of that image

`pullPolicy` is `IfNotPresent` in prod — k3s only pulls when the SHA tag isn't locally cached.

### Build cache

Each build job uses `docker/build-push-action` with GHA layer cache:

```yaml
cache-from: type=gha,scope=<image-name>
cache-to: type=gha,scope=<image-name>,mode=max
```

Go's build cache is already in the Dockerfiles via `--mount=type=cache,target=/root/.cache/go-build`.
Each image has its own GHA cache scope so a console build never evicts backend cache. On cache hit,
Go compile time drops from ~3 min to ~20 sec.

### Partial deploy — reading current tag state

For images that did not change on a given push, the deploy job reads their current pinned SHA from the
live Helm release rather than hardcoding them:

```bash
CURRENT=$(helm get values jobradar --all -o json 2>/dev/null || echo '{}')
BACKEND_TAG=$(echo "$CURRENT" | jq -r '.images.backend.tag // "latest"')
# only override the tag if this image was rebuilt this run
helm upgrade --install jobradar k8s/helm/jobradar \
  -f k8s/helm/jobradar/values.prod-node1.yaml \
  --set images.backend.tag="${NEW_BACKEND_TAG:-$BACKEND_TAG}" \
  --set images.console.tag="${NEW_CONSOLE_TAG:-$CONSOLE_TAG}"
```

Unchanged images keep their existing pinned SHA. Only rebuilt images get a new tag. This prevents
spurious pod restarts.

### Pod restart precision — deploy annotation

Each Deployment template carries per-image deploy annotations in its pod spec:

```yaml
annotations:
  deploy/backend-sha: '{{ .Values.images.backend.tag }}'
  deploy/console-sha: '{{ .Values.images.console.tag }}'
```

When `helm upgrade` runs with a changed `images.backend.tag` but the same `images.console.tag`,
Kubernetes computes a new pod spec hash only for Deployments that reference the backend annotation —
only those pods roll. Console pods stay up with zero downtime.

### GitHub secrets required

| Secret           | Value                                          |
| ---------------- | ---------------------------------------------- |
| `OCI_NODE1_HOST` | Public IP of Node 1                            |
| `OCI_NODE2_HOST` | Public IP of Node 2                            |
| `OCI_SSH_USER`   | SSH username (`ubuntu` or `opc`)               |
| `OCI_SSH_KEY`    | Private key (Ed25519) — GitHub never logs this |

`GITHUB_TOKEN` for GHCR push is automatic — no extra secret needed.

---

## Helm Chart Changes

The existing chart deploys all services unconditionally and has no pod restart annotations. Two
changes are needed.

### 1. Per-service `enabled` flags

Add `enabled: true` default to each service in `values.yaml`:

```yaml
api:
  enabled: true
console:
  enabled: true
scheduler:
  enabled: true
crawler:
  enabled: true
orchestrator:
  enabled: true
crawlerNode:
  enabled: true
filter:
  enabled: true
enricher:
  enabled: true
```

Each Deployment template gains a top-level guard:

```yaml
{{- if .Values.<service>.enabled }}
apiVersion: apps/v1
kind: Deployment
...
{{- end }}
```

### 2. Deploy annotations on pod specs

Each Deployment's pod template gets annotations referencing the image tags it uses. This is what
makes Kubernetes restart only the pods that actually changed:

- API, Scheduler, Filter, Enricher → `deploy/backend-sha: "{{ .Values.images.backend.tag }}"`
- Crawler → `deploy/crawler-sha: "{{ .Values.images.crawler.tag }}"`
- Orchestrator → `deploy/orchestrator-sha: "{{ .Values.images.orchestrator.tag }}"`
- CrawlerNode → `deploy/crawler-node-sha: "{{ .Values.images.crawlerNode.tag }}"`
- Console → `deploy/console-sha: "{{ .Values.images.console.tag }}"`

### values.prod-node1.yaml (committed, tag-agnostic)

```yaml
# Node 1: Serving only — pipeline workers disabled
nats:
  external: true
scheduler:
  enabled: false
crawler:
  enabled: false
orchestrator:
  enabled: false
crawlerNode:
  enabled: false
filter:
  enabled: false
enricher:
  enabled: false

images:
  backend:
    repository: ghcr.io/trushilpatel/jobradar-backend
    pullPolicy: IfNotPresent
  console:
    repository: ghcr.io/trushilpatel/jobradar-console
    pullPolicy: IfNotPresent
```

### values.prod-node2.yaml (committed, tag-agnostic)

```yaml
# Node 2: Pipeline only — serving layer disabled
nats:
  external: true
api:
  enabled: false
console:
  enabled: false
crawlerNode:
  replicas: 3

images:
  backend:
    repository: ghcr.io/trushilpatel/jobradar-backend
    pullPolicy: IfNotPresent
  crawler:
    repository: ghcr.io/trushilpatel/jobradar-crawler
    pullPolicy: IfNotPresent
  orchestrator:
    repository: ghcr.io/trushilpatel/jobradar-orchestrator
    pullPolicy: IfNotPresent
  crawlerNode:
    repository: ghcr.io/trushilpatel/jobradar-crawler-node
    pullPolicy: IfNotPresent
```

Image tags are never committed to these files. They are passed at deploy time via `--set
images.<name>.tag=<sha>` (for rebuilt images) or carried over from the current release state (for
unchanged images).

---

## Observability

Both nodes run the existing `obs` Helm chart (Grafana Alloy). No chart changes needed — Alloy scrapes
pod metrics, tails logs, and forwards OTLP traces to Grafana Cloud exactly as today.

The OTLP in-cluster endpoint `http://obs-alloy.jobradar-local.svc.cluster.local:4318` is already in
`values.yaml` — no per-node override needed. Grafana Cloud distinguishes both nodes by the `instance`
and `pod` labels Alloy attaches automatically.

---

## One-Time OCI Bootstrap (per node)

Manual steps, run once per instance before the first automated deploy. Not automated — requires SSH
access and Doppler login on the instance.

1. Install k3s: `curl -sfL https://get.k3s.io | sh -`
2. Install Doppler CLI and authenticate: `doppler login && doppler setup`
3. Create GHCR imagePullSecret:
   `kubectl create secret docker-registry ghcr-pull-secret --docker-server=ghcr.io --docker-username=<github-user> --docker-password=<PAT read:packages>`
4. Open OCI Security List: port 30080 (Node 1 API), port 30300 (Node 1 Console), port 22 (both)
5. Add SSH public key to `~/.ssh/authorized_keys` for the `OCI_SSH_USER` account
6. Run initial deploy manually: `doppler run -- helm upgrade --install jobradar k8s/helm/jobradar -f values.prod-node1.yaml --set images.backend.tag=<sha> --set images.console.tag=<sha>`

---

## Local Dev Compatibility

`make k8s-build && make k8s-up` is unaffected. The `values.prod-*.yaml` files are only used in CI.
Local dev continues to use `values.local.yaml` (gitignored) with `:local` image tags and in-cluster
NATS. The new `enabled` flags all default to `true` in `values.yaml` so local deploys run the full
stack as before.

---

## Files to Create

| File                                       | Purpose                                                       |
| ------------------------------------------ | ------------------------------------------------------------- |
| `.github/workflows/deploy.yml`             | Change detection + conditional builds + parallel node deploys |
| `k8s/helm/jobradar/values.prod-node1.yaml` | Serving node Helm overrides (committed, tag-agnostic)         |
| `k8s/helm/jobradar/values.prod-node2.yaml` | Pipeline node Helm overrides (committed, tag-agnostic)        |

## Files to Modify

| File                                                       | Change                                                        |
| ---------------------------------------------------------- | ------------------------------------------------------------- |
| `k8s/helm/jobradar/templates/api/deployment.yaml`          | Add `enabled` guard + `deploy/backend-sha` annotation         |
| `k8s/helm/jobradar/templates/console/deployment.yaml`      | Add `enabled` guard + `deploy/console-sha` annotation         |
| `k8s/helm/jobradar/templates/scheduler/deployment.yaml`    | Add `enabled` guard + `deploy/backend-sha` annotation         |
| `k8s/helm/jobradar/templates/crawler/deployment.yaml`      | Add `enabled` guard + `deploy/crawler-sha` annotation         |
| `k8s/helm/jobradar/templates/orchestrator/deployment.yaml` | Add `enabled` guard + `deploy/orchestrator-sha` annotation    |
| `k8s/helm/jobradar/templates/crawler-node/deployment.yaml` | Add `enabled` guard + `deploy/crawler-node-sha` annotation    |
| `k8s/helm/jobradar/templates/filter/deployment.yaml`       | Add `enabled` guard + `deploy/backend-sha` annotation         |
| `k8s/helm/jobradar/templates/enricher/deployment.yaml`     | Add `enabled` guard + `deploy/backend-sha` annotation         |
| `k8s/helm/jobradar/values.yaml`                            | Add `enabled: true` defaults; update `images.*` to GHCR repos |
| `infra/CLAUDE.md`                                          | Document OCI bootstrap and prod deploy workflow               |

---

## Design Decisions

| Decision                                   | Choice                                             | Rationale                                                                                    |
| ------------------------------------------ | -------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| Flux CD vs SSH + helm upgrade              | SSH + helm upgrade                                 | Flux uses ~150 MB RAM — too costly on 1 GB nodes                                             |
| Image registry                             | GHCR                                               | Free, OIDC-backed from GHA, no PAT to manage                                                 |
| Image architecture                         | `linux/arm64` only                                 | OCI Free Tier A1 is Ampere ARM; `CGO_ENABLED=0` cross-compiles trivially via buildx + QEMU   |
| Image tags                                 | SHA (7 chars) + `:latest` alias                    | SHA is immutable and enables `helm rollback`; `:latest` for convenience                      |
| Rebuild scope                              | Per-image path filters + shared-code catch-all     | Service changes → one image; shared code (`service/`, `platform/`) → all backend images      |
| Deploy scope                               | Per-node conditional on image change flags         | A frontend-only push never SSHes into Node 2                                                 |
| Pod restart precision                      | Deploy annotation with image SHA baked in          | Only pods referencing a changed annotation get a new pod spec hash → zero spurious restarts  |
| Current-tag retrieval for unchanged images | `helm get values` on the live release              | Deploy job always knows the exact running SHA; no separate state file needed                 |
| Service split                              | Serving vs Pipeline                                | Only in-cluster dep (`crawler-node → orchestrator`) stays on Node 2; zero cross-node traffic |
| CrawlerNode replicas                       | 3 (Node 2)                                         | Fits in practice (Go RSS ~25 MB each); drop to 2 if OOM events appear in Grafana             |
| Secrets                                    | Doppler on each node + GitHub repo secrets for SSH | Existing Doppler workflow unchanged; SSH key is the only new secret                          |
| Values files                               | Two committed prod overrides, tag-agnostic         | Auditable in git; tags are runtime state, not source code                                    |
