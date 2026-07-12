# infra/ — Dockerfiles + local k8s

Dockerfiles for backend services. The local k3d cluster is driven from `k8s/helm/` charts — this file documents the deploy workflow.

k8s (k3d) is the only deployment target. There is no Docker Compose setup.

## Crawler images are pure Go

`Dockerfile.crawler-node` and `Dockerfile.orchestrator` are **pure Go** — no Node.js, no Chromium, no Playwright. All adapters speak plain HTTP. Do not add browser dependencies to either image.

All ATS adapters (Greenhouse, Lever, Ashby, Apple, Microsoft, Netflix, Rippling, Uber, Google, Meta) live in `backend/apps/crawler-v2/adapters/ats/` and work over HTTP / JSON / GraphQL / sitemap. Google uses HTTP + RE2 regex over server-rendered HTML (`backend/apps/crawler-v2/adapters/ats/google/`); Meta uses sitemap-driven listing + JSON-LD detail (`backend/apps/crawler-v2/adapters/ats/meta/`).

Base image is `alpine:3.20` (CGO_ENABLED=0 binary has no glibc dependency; ~8 MB vs ~80 MB for debian). Non-root user UID 65532 (`jobradar`) matches the k8s `securityContext.runAsUser`.

## Security hardening

All pods run with `runAsNonRoot: true`. Worker pods (api, crawler-node, orchestrator, filter, enricher, scheduler, migrate) use the shared `jobradar.workerSecurityContext` helper (`readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, UID 65532).

Console (nginx) also has `readOnlyRootFilesystem: true`. Nginx needs three writable paths at runtime — all three are mounted as in-memory `emptyDir` volumes:

| Mount path         | sizeLimit |
| ------------------ | --------- |
| `/tmp`             | 16Mi      |
| `/var/cache/nginx` | 16Mi      |
| `/var/run`         | 4Mi       |

Do not add writable host-path mounts. If a new workload needs temporary storage, use an in-memory `emptyDir` with an explicit `sizeLimit`.

## Local k8s prerequisites — always verify before deploying

- **Doppler CLI must be authenticated** — `doppler login` once per machine. All `make k8s-bootstrap` and `make k8s-build-console` targets pull secrets from Doppler at runtime; no `.env` file is read. To bootstrap for production: `DOPPLER_CONFIG=prod make k8s-bootstrap`.
- **kubectl context must be `k3d-jobradar`**, never `docker-desktop`. Check: `kubectl config current-context`. Switch: `kubectl config use-context k3d-jobradar`
- **Storage class is `local-path`** in both Helm charts — the only class k3d provides. Never change it to `standard` (that's Docker Desktop)
- **Images must be built and imported into k3d** before deploying — `make k8s-build` or `make k8s-build-{backend,crawler-node,orchestrator,console}`. Images imported into k3d are NOT available in Docker Desktop's Kubernetes and vice versa
- **`k8s/helm/jobradar/values.local.yaml` is required** — copy it from `values.local.yaml.example` and fill in `api.supabaseProjectUrl`. `make k8s-up` aborts if the file is missing, and the chart aborts at render time if the value is empty (otherwise the api pod CrashLoopBackOffs with `SUPABASE_PROJECT_URL must be set`). The file is gitignored and lives only in the main checkout — in a **git worktree** symlink it in first (the `make k8s-up` error prints the exact `ln -s` command)
- Redis is **external (Upstash)** — not deployed in-cluster
- NATS is **external (Synadia Cloud)** in prod; local k3d runs an **in-cluster** NATS (`nats.external: false` in `values.local.yaml`)

## Deploy sequence

```bash
kubectl config use-context k3d-jobradar    # verify context first
make k8s-redeploy                           # build + import + helm sync + roll all app pods
make k8s-logs-crawler-node                   # verify
```

`make k8s-redeploy` is the blessed full-redeploy command. It chains `k8s-build` → `k8s-up` → `kubectl rollout restart` of every app deployment (nats excluded). The explicit rollout is **required**: images use the fixed `:local` tag, so a re-import is invisible to Helm (no pod-template diff) and the old image keeps running — see the `k3d stale image snapshot` memory. Use `make k8s-up` alone only for config/values changes that don't need new images.

## Iterate on the orchestrator / crawler-node

```bash
make k8s-build-orchestrator                 # rebuild + reimport orchestrator image
make k8s-build-crawler-node                 # rebuild + reimport crawler-node image
make k8s-up                                 # helm sync (applies replica count / env changes without rebuild)
make k8s-logs-orchestrator
make k8s-logs-crawler-node
```

## Scale crawler-node concurrency

Two orthogonal knobs — both live in `k8s/helm/jobradar/values.local.yaml`, no image rebuild needed:

| Knob                        | Where               | What it controls                                            |
| --------------------------- | ------------------- | ----------------------------------------------------------- |
| `crawlerNode.replicas`      | `values.local.yaml` | Number of crawler-node pods (each is an independent worker) |
| `crawlerNode.maxConcurrent` | `values.local.yaml` | Assignments one pod holds at the same time                  |

**Effective concurrency = `replicas × maxConcurrent`**

Example — 3 pods × 3 concurrent = up to 9 companies crawled in parallel:

```yaml
# k8s/helm/jobradar/values.local.yaml
crawlerNode:
  replicas: 3
  maxConcurrent: '3'
```

After editing, apply with `make k8s-up` (no image rebuild needed — replica count is a Deployment spec field).

For the 8 GB M1 k3d node, `replicas: 3` with `maxConcurrent: 3` is a safe default. Each crawler-node pod is capped at 64Mi memory; 3 pods = 192Mi additional pressure. Check `kubectl top nodes` before raising replicas significantly.

## Trigger a scan manually

```bash
curl -X POST http://localhost:30080/companies/<company-id>/scan
make k8s-logs-crawler-node
```

## Change the LLM models (filter / enricher)

LLM slot config lives in `k8s/helm/jobradar/files/llm-slots.yaml` (filter slots + enricher slots), mounted as configmap `jobradar-llm-slots`. It is **not** baked into the image, so changing it needs a Helm sync, not a rebuild:

```bash
# edit k8s/helm/jobradar/files/llm-slots.yaml
make k8s-up                                  # helm upgrade — regenerates the configmap
kubectl rollout restart deployment/jobradar-filter -n jobradar-local    # pod must restart to remount
```

Models are pinned to a preview-access key — most GA models return 404 or `limit:0` → 429. Before switching to a model, verify it returns HTTP 200 from **inside the cluster** (same egress as the workers): `kubectl run gemtest --rm -i --image=curlimages/curl -- sh -c 'curl ... :generateContent?key=...'`. A local-machine curl is unreliable here. See the `LLM model configuration` memory for the current pinned models and known-good alternatives.

## Resource limits reference

Tuned for a single-node k3d cluster on an 8 GB M1. Never raise limits without checking available headroom (`kubectl top nodes`).

| Component    | Req mem | Limit mem      | Req CPU | Limit CPU |
| ------------ | ------- | -------------- | ------- | --------- |
| api          | 64Mi    | 128Mi          | 100m    | 500m      |
| orchestrator | 64Mi    | 64Mi           | 50m     | 300m      |
| crawler-node | 64Mi    | 64Mi (per pod) | 50m     | 200m      |
| filter       | 32Mi    | 64Mi           | 100m    | 500m      |
| enricher     | 32Mi    | 64Mi           | 100m    | 500m      |
| scheduler    | 64Mi    | 64Mi           | 50m     | 200m      |
| console      | 32Mi    | 64Mi           | 25m     | 100m      |
| migrate      | 32Mi    | 128Mi          | 50m     | —         |

CPU limits are required on all workloads to prevent a runaway pod from starving k3d. The migrate job has no CPU limit intentionally (short-lived; bursty is acceptable).

For crawler-node, the per-pod limit is 64Mi. At 3 replicas = 192Mi total. If raising replicas, check `kubectl top nodes` — the 8 GB M1 k3d node has a fixed memory budget.

## Common failure modes

| Symptom                                      | Cause                                                                      | Fix                                                                                                                                                                                                                       |
| -------------------------------------------- | -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `k8s-bootstrap` exits "Doppler fetch failed" | Not logged in to Doppler CLI                                               | `doppler login`, then re-run                                                                                                                                                                                              |
| `ErrImagePull` on all pods                   | Wrong kubectl context (docker-desktop) or images not imported              | Switch context; `make k8s-build`                                                                                                                                                                                          |
| filter/enricher `CrashLoopBackOff`           | `secrets/jobradar_NATS.creds` empty or wrong, or NATS secret not recreated | Fill creds file, run `make k8s-bootstrap`, then restart pods                                                                                                                                                              |
| `TLS handshake timeout`                      | k3d API server still starting after restart                                | Wait ~30s; `until kubectl get nodes 2>/dev/null \| grep -q Ready; do sleep 3; done`                                                                                                                                       |
| Namespace stuck `Terminating`                | Finalizers not cleared                                                     | Force-clear: `kubectl get ns <name> -o json \| python3 -c "import sys,json; d=json.load(sys.stdin); d['spec']['finalizers']=[]; print(json.dumps(d))" \| kubectl replace --raw "/api/v1/namespaces/<name>/finalize" -f -` |

## OCI Production Deployment

Two Oracle Cloud Free Tier ARM A1 instances (1 OCPU, 1 GB RAM each + 2 GB swap). Deployed automatically on every push to `main` via `.github/workflows/deploy.yml`. Only changed images are rebuilt; only affected nodes are deployed; only pods whose image tag annotation changed are restarted.

**Node roles:**

| Node  | Name     | IP                | Services                                                      | Values file              |
| ----- | -------- | ----------------- | ------------------------------------------------------------- | ------------------------ |
| INFRA | Serving  | `161.153.66.17`   | API + Console                                                 | `values.prod-infra.yaml` |
| APP   | Pipeline | `147.224.253.250` | Scheduler + Orchestrator + CrawlerNode ×1 + Filter + Enricher | `values.prod-app.yaml`   |

CrawlerNode replicas is pinned to **1** (not the chart's base default of 3) — the external NGS NATS free-tier account caps concurrent connections at 4, and orchestrator + filter + enricher already consume 3, leaving room for exactly one crawler-node publisher connection. See the comment in `values.prod-app.yaml`.

Node IPs and usernames are stored in Doppler prod as `OCI_INFRA_IP`, `OCI_INFRA_USERNAME`, `OCI_APP_IP`, `OCI_APP_USERNAME`. GitHub PAT (read:packages) is stored as `GITHUB_PAT`.

### CI/CD cycle — how every push to `main` deploys

Two separate workflows run on push to `main`:

**`ci.yml`** (also runs on PRs): lint + unit tests + Codecov upload.

**`release.yml`**: detect which services changed → compile changed Go binaries → build per-service images → deploy to OCI nodes.

```
push to main
     │
     ├─► [ci.yml]  lint + tests (parallel with release.yml)
     │
     └─► [release.yml]
              │
              ▼
         [detect-changes]  ← dorny/paths-filter per service
              │
              ├─ api/ changed?          → [build-api]          → ghcr.io/trushilpatel/jobradar-api:<sha>
              ├─ scheduler/ changed?    → [build-scheduler]    → ghcr.io/trushilpatel/jobradar-scheduler:<sha>
              ├─ filter/ changed?       → [build-filter]       → ghcr.io/trushilpatel/jobradar-filter:<sha>
              ├─ enricher/ changed?     → [build-enricher]     → ghcr.io/trushilpatel/jobradar-enricher:<sha>
              ├─ orchestrator/ changed? → [build-orchestrator] → ghcr.io/trushilpatel/jobradar-orchestrator:<sha>
              ├─ crawler-node/ changed? → [build-crawler-node] → ghcr.io/trushilpatel/jobradar-crawler-node:<sha>
              └─ frontend/ changed?     → [build-console]      → ghcr.io/trushilpatel/jobradar-console:<sha>
                                                  │
                             ┌────────────────────┤
                             ▼                    ▼
                    [deploy-infra]         [deploy-app]
                    (if api or             (if scheduler, filter,
                     console changed)       enricher, orch, or
                    SSH → helm upgrade      crawler-node changed)
                    INFRA node only        SSH → helm upgrade
                                           APP node only
```

Key properties:

- Each service has its own image — deploying a filter fix never restarts the API or scheduler
- A frontend-only push rebuilds only `jobradar-console` and deploys only to INFRA — APP node is never touched
- For images not rebuilt, the deploy step reads the currently running tag from `helm get values` so unchanged pods are never restarted
- Each Deployment has a `deploy/<service>-sha` annotation — Kubernetes only rolls pods whose annotation changed
- All images are `linux/amd64` built on GitHub-hosted x86 runners (no QEMU needed)
- Rollback: trigger `deploy.yml` (manual dispatch) — select node + Helm revision; or SSH and run `helm rollback jobradar <revision>` directly

### One-time bootstrap per node

Use `scripts/oci-node-bootstrap.sh` — idempotent, safe to re-run on any node (including a third node in future):

```bash
# Copy to node and run
scp scripts/oci-node-bootstrap.sh ubuntu@<node-ip>:~/
ssh ubuntu@<node-ip>

# Optional: pass GitHub PAT to skip the interactive prompt
GH_USER=trushilpatel GH_PAT=$(doppler secrets get GITHUB_PAT --plain) bash ~/oci-node-bootstrap.sh
```

The script handles in order: swap (2 GB), k3s config (traefik/servicelb/metrics-server disabled, `write-kubeconfig-mode: 0644`, OCI resolv.conf), k3s install, jobradar-iptables.service (clears stale KUBE-ROUTER chains on reboot), Helm, jq, Doppler CLI, namespace, ghcr-pull-secret.

**Remediation for existing nodes**: if a node is failing with `TLS handshake timeout` after a reboot, run `scripts/oci-k3s-netfix.sh` on it — idempotent, safe to re-run.

After the script: run `doppler login && doppler setup` (project: `job-radar`, config: `prod`) — this is interactive and cannot be scripted.

### GitHub Actions secrets required

| Secret                       | Value                                                                            |
| ---------------------------- | -------------------------------------------------------------------------------- |
| `OCI_INFRA_HOST`             | `161.153.66.17`                                                                  |
| `OCI_APP_HOST`               | `147.224.253.250`                                                                |
| `OCI_SSH_USER`               | `ubuntu`                                                                         |
| `OCI_SSH_KEY`                | Private key content (Ed25519)                                                    |
| `VITE_CLERK_PUBLISHABLE_KEY` | Clerk publishable key (same as Doppler `VITE_CLERK_PUBLISHABLE_KEY`) — for build |
| `CLERK_JWKS_URL`             | Clerk JWKS URL (same as Doppler `CLERK_JWKS_URL`) — passed at deploy time        |
| `CLERK_ISSUER_URL`           | Clerk issuer URL (same as Doppler `CLERK_ISSUER_URL`) — passed at deploy time    |
| `CODECOV_TOKEN`              | Codecov upload token                                                             |

`GITHUB_TOKEN` for GHCR push is injected automatically — no extra secret needed.

### OCI Security List ports

| Node  | Ports                                                        |
| ----- | ------------------------------------------------------------ |
| INFRA | TCP 22 (SSH), 30080 (API NodePort), 30300 (Console NodePort) |
| APP   | TCP 22 (SSH) only                                            |

### Why swap is required

The 1 GB nodes have no swap by default. k3s alone uses ~600–700 MB, leaving ~140–200 MB for the OS and pods. Without swap, the kernel OOM-kills processes during bootstrap (particularly Doppler CLI install + k3s startup together). The 2 GB swapfile added by the bootstrap script prevents this. The swapfile persists across reboots via `/etc/fstab`.

The APP node additionally runs `zram-tools` (`/etc/default/zramswap`: `ALGO=lz4 SIZE=128 PRIORITY=100`) — a 128Mi compressed-RAM swap device at a higher swap priority than `/swapfile`, so the kernel prefers it first. Disk-backed swap on this node was measured at 20–66% iowait under load (the APP node runs 5 pipeline pods vs. INFRA's 2, and is far more memory-contended); zram avoids the disk round-trip for the hottest swapped pages, with the 128Mi cap bounding worst-case extra RAM overhead on a box with almost no slack. `/swapfile` remains as overflow once zram fills. Not yet applied to INFRA (no measured pressure there — `kubectl top`/`vmstat` first if considering it).

### Why traefik/servicelb/metrics-server are disabled

k3s installs Traefik ingress, service load balancer, and metrics-server by default. These consume ~150–200 MB of RAM we cannot spare on 1 GB nodes. JobRadar uses NodePort services (not Ingress or LoadBalancer), so none of these components are needed. Disabled via `/etc/rancher/k3s/config.yaml` (written before k3s install by the bootstrap script).

### Prod telemetry — Grafana Cloud, direct (no in-cluster Alloy)

Local k3d ships metrics/traces/logs to Grafana Cloud via the `observability` Helm chart (Alloy DaemonSet — see `k8s/helm/observability/`). **That chart is never deployed to OCI prod** — `release.yml`/`deploy.yml` only ever `helm upgrade` the `jobradar` chart. A full Alloy (OTLP receiver + log tailing + scrape) is also too heavy for these 1 GB nodes, and `enricher`/`scheduler`/`filter`/`orchestrator` already run `dnsPolicy: Default` (see each `deployment.yaml`) so they can't resolve an in-cluster Service name anyway. Prod instead uses two lightweight, direct mechanisms — both gated by values so local dev (`values.yaml` defaults) is unaffected:

- **Traces**: each binary already pushes OTLP client-side (`backend/common/tracing/tracer.go`) — prod just points `otelEndpoint` straight at Grafana Cloud's public OTLP gateway (`https://otlp-gateway-prod-us-west-0.grafana.net/otlp/v1/traces`) instead of an in-cluster collector. `tracer.go` auto-detects `https://` and skips `WithInsecure()`; Basic-auth comes from the standard `OTEL_EXPORTER_OTLP_HEADERS` env var, which the OTel SDK reads natively. Wired via `otelCredsSecret` (a k8s Secret name, empty by default) added to each service's `envFrom` in the Helm chart.
- **Metrics**: a single `vmagent` pod per node (`templates/vmagent/`, ~12–32Mi) — scrape-only, no in-cluster collector needed. Config in `values.prod-{app,infra}.yaml`'s `vmagent.scrapeTargets` list (per-node — INFRA scrapes just `api`, APP scrapes its 5 pipeline services' existing metrics Services). `remoteWrite.basicAuth.passwordFile` reads from a mounted Secret, never a committed value.

**Secrets, created once per node (not part of any script/CI — SSH + kubectl, sourced from Doppler prod):**

```bash
kubectl create secret generic grafana-cloud-otel-creds -n jobradar-local \
  --from-literal="OTEL_EXPORTER_OTLP_HEADERS=$(doppler secrets get GRAFANA_CLOUD_OTEL_HEADERS --plain --config prod --project job-radar)"
kubectl create secret generic grafana-cloud-prom-creds -n jobradar-local \
  --from-literal="REMOTE_WRITE_URL=$(doppler secrets get GRAFANA_CLOUD_PROM_URL --plain --config prod --project job-radar)" \
  --from-literal="REMOTE_WRITE_USER=$(doppler secrets get GRAFANA_CLOUD_PROM_USER --plain --config prod --project job-radar)" \
  --from-literal="password=$(doppler secrets get GRAFANA_CLOUD_PROM_TOKEN --plain --config prod --project job-radar)"
```

Doppler prod keys: `GRAFANA_CLOUD_TEMPO_URL`, `GRAFANA_CLOUD_OTEL_HEADERS`, `GRAFANA_CLOUD_PROM_URL`, `GRAFANA_CLOUD_PROM_USER`, `GRAFANA_CLOUD_PROM_TOKEN` (same Grafana Cloud stack as local dev's `values.local.yaml`, just centralized). Logs are intentionally not shipped from prod yet (lowest priority, heaviest Alloy component — `kubectl logs` is the fallback); a true per-company Prometheus breakdown was also deliberately deferred (would need unbounded per-company label cardinality or a new Postgres datasource in Grafana Cloud — neither added without a separate decision).

### OCI failure modes

| Symptom                                                                                                                           | Cause                                                                                                                                                                                                                                                                                                                                                                                                                    | Fix                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| --------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Node SSH unreachable after bootstrap                                                                                              | OOM during install — SSH daemon killed                                                                                                                                                                                                                                                                                                                                                                                   | Hard reboot from OCI Console; re-run bootstrap script                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `ErrImagePull` on all pods                                                                                                        | `ghcr-pull-secret` missing or expired                                                                                                                                                                                                                                                                                                                                                                                    | Re-run bootstrap step 6; rotate GitHub PAT if expired                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `TLS handshake timeout` on kubectl                                                                                                | k3s API not ready yet                                                                                                                                                                                                                                                                                                                                                                                                    | Wait 30s; k3s is still starting                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `helm: command not found` on node                                                                                                 | Helm not installed                                                                                                                                                                                                                                                                                                                                                                                                       | Re-run bootstrap script                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Pods not restarting after push                                                                                                    | Image tag annotation unchanged                                                                                                                                                                                                                                                                                                                                                                                           | Confirm the build job ran in GHA; check deploy workflow logs                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `connection refused` on 30080/30300                                                                                               | Pods not scheduled yet                                                                                                                                                                                                                                                                                                                                                                                                   | Check `kubectl get pods -n jobradar-local`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Pods can't reach k3s API; CoreDNS `CrashLoopBackOff` after a reboot                                                               | Stale `KUBE-ROUTER-*` iptables chains restored by `netfilter-persistent` after reboot block pod→API connections. The INPUT chain REJECT drops NEW packets from the pod CIDR because ClusterIP DNAT routes pod→API connections through INPUT (not FORWARD). OCI `/etc/resolv.conf` → `127.0.0.53` (systemd-resolved stub) causes CoreDNS `forward . /etc/resolv.conf` to forward to the pod's own loopback (unreachable). | Run `bash scripts/oci-k3s-netfix.sh` on the node — installs `jobradar-iptables.service` (removes stale chains, adds pod CIDR ACCEPT to INPUT before every k3s start) and `/etc/rancher/k3s/resolv.conf` (OCI DNS). Then restart k3s.                                                                                                                                                                                                                                                                                                             |
| CoreDNS `CrashLoopBackOff` in steady state (no reboot) — cascades into filter/enricher failing to resolve Supabase/NATS/LLM hosts | The APP node runs at 40–55% CPU steal + heavy swap-induced iowait under normal pipeline load. CoreDNS's _stock_ probe (liveness `timeoutSeconds:1`, readiness `periodSeconds:2/timeoutSeconds:1`) is too tight for that — a healthy CoreDNS misses the deadline and gets restarted or pulled from Service endpoints, taking DNS down for every pod still on `dnsPolicy: ClusterFirst`.                                   | Two layers: (1) `scheduler`/`orchestrator`/`filter`/`enricher` run with `dnsPolicy: Default` (bypasses CoreDNS — see the `dnsPolicy` comment in each `k8s/helm/jobradar/templates/*/deployment.yaml`), so only `crawler-node` still depends on CoreDNS at all. (2) `jobradar-coredns-harden.timer` (installed by `oci-node-bootstrap.sh`, runs every 3 min) widens CoreDNS's own probe thresholds and self-heals after k3s reverts them on restart — previously this was a one-off `kubectl patch` that silently reverted and was never noticed. |
