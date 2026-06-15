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
| APP   | Pipeline | `147.224.253.250` | Scheduler + Orchestrator + CrawlerNode ×3 + Filter + Enricher | `values.prod-app.yaml`   |

Node IPs and usernames are stored in Doppler prod as `OCI_INFRA_IP`, `OCI_INFRA_USERNAME`, `OCI_APP_IP`, `OCI_APP_USERNAME`. GitHub PAT (read:packages) is stored as `GITHUB_PAT`.

### CI/CD cycle — how every push to `main` deploys

```
push to main
     │
     ▼
[detect-changes]  ← dorny/paths-filter checks which files changed
     │
     ├─ backend/ changed?      → [build-backend]      → ghcr.io/trushilpatel/jobradar-backend:<sha>
     ├─ orchestrator/ changed? → [build-orchestrator] → ghcr.io/trushilpatel/jobradar-orchestrator:<sha>
     ├─ crawler-node/ changed? → [build-crawler-node] → ghcr.io/trushilpatel/jobradar-crawler-node:<sha>
     └─ frontend/ changed?     → [build-console]      → ghcr.io/trushilpatel/jobradar-console:<sha>
                                          │
                   ┌──────────────────────┤
                   ▼                      ▼
          [deploy-infra]          [deploy-app]
          (if backend or          (if backend, orch,
           console changed)        or crawler-node changed)
          SSH → helm upgrade      SSH → helm upgrade
          INFRA node only         APP node only
```

Key properties:

- A frontend-only push rebuilds only `jobradar-console` and deploys only to INFRA — APP node is never touched
- For images not rebuilt, the deploy job reads the currently running tag from `helm get values` on the live release so no pods restart unnecessarily
- Each Deployment has a `deploy/<image>-sha` annotation baked into its pod spec — Kubernetes only rolls pods whose annotation changed
- All five images are ARM64 (`linux/arm64`) built via buildx + QEMU on GitHub-hosted x86 runners

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

### Why traefik/servicelb/metrics-server are disabled

k3s installs Traefik ingress, service load balancer, and metrics-server by default. These consume ~150–200 MB of RAM we cannot spare on 1 GB nodes. JobRadar uses NodePort services (not Ingress or LoadBalancer), so none of these components are needed. Disabled via `/etc/rancher/k3s/config.yaml` (written before k3s install by the bootstrap script).

### OCI failure modes

| Symptom                                              | Cause                                                                                                                                                                                                                                                                                                                                                                                                                    | Fix                                                                                                                                                                                                                                  |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Node SSH unreachable after bootstrap                 | OOM during install — SSH daemon killed                                                                                                                                                                                                                                                                                                                                                                                   | Hard reboot from OCI Console; re-run bootstrap script                                                                                                                                                                                |
| `ErrImagePull` on all pods                           | `ghcr-pull-secret` missing or expired                                                                                                                                                                                                                                                                                                                                                                                    | Re-run bootstrap step 6; rotate GitHub PAT if expired                                                                                                                                                                                |
| `TLS handshake timeout` on kubectl                   | k3s API not ready yet                                                                                                                                                                                                                                                                                                                                                                                                    | Wait 30s; k3s is still starting                                                                                                                                                                                                      |
| `helm: command not found` on node                    | Helm not installed                                                                                                                                                                                                                                                                                                                                                                                                       | Re-run bootstrap script                                                                                                                                                                                                              |
| Pods not restarting after push                       | Image tag annotation unchanged                                                                                                                                                                                                                                                                                                                                                                                           | Confirm the build job ran in GHA; check deploy workflow logs                                                                                                                                                                         |
| `connection refused` on 30080/30300                  | Pods not scheduled yet                                                                                                                                                                                                                                                                                                                                                                                                   | Check `kubectl get pods -n jobradar-local`                                                                                                                                                                                           |
| Pods can't reach k3s API; CoreDNS `CrashLoopBackOff` | Stale `KUBE-ROUTER-*` iptables chains restored by `netfilter-persistent` after reboot block pod→API connections. The INPUT chain REJECT drops NEW packets from the pod CIDR because ClusterIP DNAT routes pod→API connections through INPUT (not FORWARD). OCI `/etc/resolv.conf` → `127.0.0.53` (systemd-resolved stub) causes CoreDNS `forward . /etc/resolv.conf` to forward to the pod's own loopback (unreachable). | Run `bash scripts/oci-k3s-netfix.sh` on the node — installs `jobradar-iptables.service` (removes stale chains, adds pod CIDR ACCEPT to INPUT before every k3s start) and `/etc/rancher/k3s/resolv.conf` (OCI DNS). Then restart k3s. |
