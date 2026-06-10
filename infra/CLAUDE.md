# infra/ — Dockerfiles + local k8s

Dockerfiles for backend services. The local k3d cluster is driven from `k8s/helm/` charts — this file documents the deploy workflow.

k8s (k3d) is the only deployment target. There is no Docker Compose setup.

## Crawler images are pure Go

`Dockerfile.crawler` and `Dockerfile.orchestrator` are **pure Go** — no Node.js, no Chromium, no Playwright. All adapters speak plain HTTP. Do not add browser dependencies to either image.

All ATS adapters (Greenhouse, Lever, Ashby, Apple, Microsoft, Netflix, Rippling, Uber, Google, Meta) work over HTTP / JSON / GraphQL / sitemap. Google uses HTTP + RE2 regex over server-rendered HTML (`backend/apps/crawler/ats/google/http.go`); Meta uses sitemap-driven listing + JSON-LD detail (`backend/apps/crawler/ats/meta/*.go`).

Base image is `alpine:3.20` (CGO_ENABLED=0 binary has no glibc dependency; ~8 MB vs ~80 MB for debian). Non-root user UID 65532 (`jobradar`) matches the k8s `securityContext.runAsUser`.

## Security hardening

All pods run with `runAsNonRoot: true`. Worker pods (api, crawler, filter, enricher, scheduler, migrate) use the shared `jobradar.workerSecurityContext` helper (`readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, UID 65532).

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
- **Images must be built and imported into k3d** before deploying — `make k8s-build` or `make k8s-build-{backend,crawler,console}`. Images imported into k3d are NOT available in Docker Desktop's Kubernetes and vice versa
- **`k8s/helm/jobradar/values.local.yaml` is required** — copy it from `values.local.yaml.example` and fill in `api.supabaseProjectUrl`. `make k8s-up` aborts if the file is missing, and the chart aborts at render time if the value is empty (otherwise the api pod CrashLoopBackOffs with `SUPABASE_PROJECT_URL must be set`). The file is gitignored and lives only in the main checkout — in a **git worktree** symlink it in first (the `make k8s-up` error prints the exact `ln -s` command)
- Redis is **external (Upstash)** — not deployed in-cluster
- NATS is **external (Synadia Cloud)** in prod; local k3d runs an **in-cluster** NATS (`nats.external: false` in `values.local.yaml`)

## Deploy sequence

```bash
kubectl config use-context k3d-jobradar    # verify context first
make k8s-redeploy                           # build + import + helm sync + roll all app pods
make k8s-logs-crawler                       # verify
```

`make k8s-redeploy` is the blessed full-redeploy command. It chains `k8s-build` → `k8s-up` → `kubectl rollout restart` of every app deployment (nats excluded). The explicit rollout is **required**: images use the fixed `:local` tag, so a re-import is invisible to Helm (no pod-template diff) and the old image keeps running — see the `k3d stale image snapshot` memory. Use `make k8s-up` alone only for config/values changes that don't need new images.

## Iterate on the crawler only

```bash
make k8s-build-crawler                      # rebuild + reimport crawler image only
make k8s-restart-crawler                    # rolling restart crawler pod
make k8s-logs-crawler
```

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
make k8s-logs-crawler
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
| crawler      | 128Mi   | 256Mi          | 100m    | 300m      |
| orchestrator | 64Mi    | 128Mi          | 50m     | 300m      |
| crawler-node | 64Mi    | 64Mi (per pod) | 50m     | 200m      |
| filter       | 32Mi    | 128Mi          | 100m    | 500m      |
| enricher     | 32Mi    | 128Mi          | 100m    | 500m      |
| scheduler    | 64Mi    | 128Mi          | 50m     | 200m      |
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
