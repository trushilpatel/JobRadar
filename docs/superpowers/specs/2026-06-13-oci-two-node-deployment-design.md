# OCI Two-Node Production Deployment

**Date:** 2026-06-13
**Status:** Approved — pending implementation
**Supersedes:** `2026-05-23-ghcr-flux-cd-gitops-design.md` (Flux CD rejected; too heavy for 1 GB nodes)

---

## Goal

Deploy the full JobRadar stack across two Oracle Cloud Free Tier ARM A1 instances (1 GB RAM each), with automated CI/CD on every push to `main` and full observability via the existing Grafana Cloud stack.

---

## Instance Roles

All inter-service communication routes through external managed services (Supabase, Synadia Cloud NATS, Upstash Redis). The only in-cluster dependency is `crawler-node → orchestrator` — both live on Node 2. The two nodes therefore have zero cross-node traffic.

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

Hosts the background job engine. Downtime here does not affect the UI; NATS buffers messages and the scheduler re-enqueues on next tick.

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

Note: sum of pod limits (~1,088 Mi) exceeds the 1 GB ceiling. This is intentional — Go binaries are lean and actual CrawlerNode RSS is ~20–30 MB each. If OOM events appear in Grafana, drop CrawlerNode replicas from 3 → 2.

---

## CI/CD Pipeline

**Trigger:** push to `main`

**Approach:** GitHub Actions builds multi-arch images, pushes to GHCR, then SSHes into each OCI node and runs `helm upgrade`. Node 1 and Node 2 deploy in parallel (independent releases).

```
push to main
     │
     ▼
[build] GitHub Actions (ubuntu-latest, arm64 via buildx + QEMU)
  • docker buildx build --platform linux/arm64
  • images: jobradar-backend, jobradar-crawler, jobradar-console
  • tags: ghcr.io/trushilpatel/jobradar-<name>:<sha> + :latest
  • auth: GITHUB_TOKEN (OIDC, auto-expires, no PAT needed)
     │
     ▼ (parallel)
[deploy-node1]                    [deploy-node2]
SSH → OCI Node 1                  SSH → OCI Node 2
helm upgrade jobradar             helm upgrade jobradar
  -f values.prod-node1.yaml         -f values.prod-node2.yaml
  --set image.tag=<sha>             --set image.tag=<sha>
```

### Image registry

The existing chart tracks five independent images, each with its own Dockerfile:

| `values.yaml` key     | GHCR path                                    | Dockerfile                | Contains                                  |
| --------------------- | -------------------------------------------- | ------------------------- | ----------------------------------------- |
| `images.backend`      | `ghcr.io/trushilpatel/jobradar-backend`      | `Dockerfile.backend`      | api, scheduler, filter, enricher, migrate |
| `images.crawler`      | `ghcr.io/trushilpatel/jobradar-crawler`      | `Dockerfile.crawler`      | crawler                                   |
| `images.orchestrator` | `ghcr.io/trushilpatel/jobradar-orchestrator` | `Dockerfile.orchestrator` | orchestrator                              |
| `images.crawlerNode`  | `ghcr.io/trushilpatel/jobradar-crawler-node` | `Dockerfile.crawler-node` | crawler-node                              |
| `images.console`      | `ghcr.io/trushilpatel/jobradar-console`      | `Dockerfile.console`      | React + nginx                             |

No new Dockerfiles needed. Console image bakes `VITE_*` vars at build time via Doppler.

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

The existing chart deploys all services unconditionally. We need per-service `enabled` flags so each node only runs its assigned subset.

### New `enabled` flags (values.yaml defaults)

```yaml
api:
  enabled: true
console:
  enabled: true
scheduler:
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

Each Deployment template gains a top-level `{{- if .Values.<service>.enabled }}` guard. This is the only chart change — no template logic otherwise changes.

### values.prod-node1.yaml (committed)

```yaml
# Node 1: Serving only
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
    pullPolicy: Always
  console:
    repository: ghcr.io/trushilpatel/jobradar-console
    pullPolicy: Always
```

### values.prod-node2.yaml (committed)

```yaml
# Node 2: Pipeline only
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
    pullPolicy: Always
  crawler:
    repository: ghcr.io/trushilpatel/jobradar-crawler
    pullPolicy: Always
  orchestrator:
    repository: ghcr.io/trushilpatel/jobradar-orchestrator
    pullPolicy: Always
  crawlerNode:
    repository: ghcr.io/trushilpatel/jobradar-crawler-node
    pullPolicy: Always
```

Image tags are passed at deploy time — not committed to these files:

```bash
helm upgrade jobradar k8s/helm/jobradar \
  -f values.prod-node1.yaml \
  --set images.backend.tag=abc1234 \
  --set images.console.tag=abc1234
```

This keeps the prod values files tag-agnostic in git while the SHA is the deploy-time source of truth.

---

## Observability

Both nodes run the existing `obs` Helm chart (Grafana Alloy DaemonSet). No changes to the chart needed — Alloy scrapes pod metrics, tails logs, and forwards OTLP traces to Grafana Cloud exactly as it does locally.

The OTLP endpoint inside each cluster remains `http://obs-alloy.jobradar-local.svc.cluster.local:4318` — the same value already in `values.yaml`. No per-node config change needed.

Grafana Cloud will show metrics/logs/traces from both nodes in the same dashboards, distinguished by the `instance` and `pod` label that Alloy attaches automatically.

---

## One-Time OCI Bootstrap (per node)

Manual steps run once per instance before the first deploy. These are not automated — they require SSH access and Doppler login.

1. Install k3s: `curl -sfL https://get.k3s.io | sh -`
2. Install Doppler CLI and run `doppler login && doppler setup`
3. Create GHCR imagePullSecret: `kubectl create secret docker-registry ghcr-pull-secret --docker-server=ghcr.io --docker-username=<github-user> --docker-password=<PAT with read:packages>`
4. Open OCI firewall: port 30080 (API, Node 1 only), 30300 (Console, Node 1 only), 22 (SSH both)
5. Copy SSH public key to `~/.ssh/authorized_keys`
6. Run `doppler run -- helm upgrade --install jobradar k8s/helm/jobradar -f values.prod-node1.yaml` (or node2) for the initial deploy

---

## Local Dev Compatibility

`make k8s-build && make k8s-up` is unaffected. The new `values.prod-*.yaml` files are only used in CI. Local dev continues to use `values.local.yaml` (gitignored) with `:local` image tags and in-cluster NATS.

---

## Files to Create

| File                                       | Purpose                                                        |
| ------------------------------------------ | -------------------------------------------------------------- |
| `.github/workflows/deploy.yml`             | Build multi-arch images + deploy to both nodes on push to main |
| `k8s/helm/jobradar/values.prod-node1.yaml` | Serving node Helm overrides                                    |
| `k8s/helm/jobradar/values.prod-node2.yaml` | Pipeline node Helm overrides                                   |

## Files to Modify

| File                                                       | Change                                                                         |
| ---------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `k8s/helm/jobradar/templates/api/deployment.yaml`          | Add `enabled` guard                                                            |
| `k8s/helm/jobradar/templates/console/deployment.yaml`      | Add `enabled` guard                                                            |
| `k8s/helm/jobradar/templates/scheduler/deployment.yaml`    | Add `enabled` guard                                                            |
| `k8s/helm/jobradar/templates/orchestrator/deployment.yaml` | Add `enabled` guard                                                            |
| `k8s/helm/jobradar/templates/crawler/deployment.yaml`      | Add `enabled` guard                                                            |
| `k8s/helm/jobradar/templates/crawler-node/deployment.yaml` | Add `enabled` guard                                                            |
| `k8s/helm/jobradar/templates/filter/deployment.yaml`       | Add `enabled` guard                                                            |
| `k8s/helm/jobradar/templates/enricher/deployment.yaml`     | Add `enabled` guard                                                            |
| `k8s/helm/jobradar/values.yaml`                            | Add `enabled: true` defaults for all services; update `images.*` repos to GHCR |
| `infra/CLAUDE.md`                                          | Document OCI bootstrap steps and prod deploy workflow                          |

---

## Design Decisions

| Decision                      | Choice                                             | Rationale                                                                                   |
| ----------------------------- | -------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| Flux CD vs SSH + helm upgrade | SSH + helm upgrade                                 | Flux uses ~150 MB RAM — too costly on 1 GB nodes                                            |
| Image registry                | GHCR                                               | Free, OIDC-backed from GHA, no external account                                             |
| Image architecture            | linux/arm64 only                                   | OCI Free Tier A1 is Ampere ARM; Go CGO_ENABLED=0 cross-compiles trivially                   |
| Image tags                    | Git SHA (7 chars)                                  | Immutable per commit; `helm rollback` works cleanly                                         |
| Service split                 | Serving vs Pipeline                                | Only in-cluster dep (crawler-node→orchestrator) stays on Node 2; zero cross-node traffic    |
| CrawlerNode replicas          | 3 (Node 2)                                         | Fits in practice (Go RSS ~25 MB each); sum-of-limits exceeds 1 GB but actual usage does not |
| Secrets                       | Doppler on each node + GitHub repo secrets for SSH | Existing Doppler workflow unchanged; only SSH key is a new secret                           |
| values files                  | Two committed prod overrides                       | Auditable in git; tag passed at deploy time so files are tag-agnostic                       |
