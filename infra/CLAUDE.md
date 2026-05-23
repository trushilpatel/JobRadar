# infra/ — Dockerfiles + local k8s

Dockerfiles for backend services. The local k3d cluster is driven from `k8s/helm/` charts — this file documents the deploy workflow.

k8s (k3d) is the only deployment target. There is no Docker Compose setup.

## Crawler image is pure Go

`Dockerfile.crawler` is **pure Go** — no Node.js, no Chromium, no Playwright. All adapters speak plain HTTP. Do not add browser dependencies back.

All ATS adapters (Greenhouse, Lever, Ashby, Apple, Microsoft, Netflix, Rippling, Uber, Google, Meta) work over HTTP / JSON / GraphQL / sitemap. Google uses HTTP + RE2 regex over server-rendered HTML (`backend/apps/crawler/ats/google/http.go`); Meta uses sitemap-driven listing + JSON-LD detail (`backend/apps/crawler/ats/meta/*.go`).

## Local k8s prerequisites — always verify before deploying

- **kubectl context must be `k3d-jobradar`**, never `docker-desktop`. Check: `kubectl config current-context`. Switch: `kubectl config use-context k3d-jobradar`
- **Storage class is `local-path`** in both Helm charts — the only class k3d provides. Never change it to `standard` (that's Docker Desktop)
- **Images must be built and imported into k3d** before deploying — `make k8s-build` or `make k8s-build-{backend,crawler,console}`. Images imported into k3d are NOT available in Docker Desktop's Kubernetes and vice versa
- Redis is **external (Upstash)** — not deployed in-cluster
- NATS is **external (Synadia Cloud)** — not deployed in-cluster

## Deploy sequence

```bash
kubectl config use-context k3d-jobradar    # verify context first
make k8s-build                              # build + import all images into k3d
make k8s-up                                 # preflight + helm install
make k8s-logs-crawler                       # verify
```

## Iterate on the crawler only

```bash
make k8s-build-crawler                      # rebuild + reimport crawler image only
make k8s-restart-crawler                    # rolling restart crawler pod
make k8s-logs-crawler
```

## Trigger a scan manually

```bash
curl -X POST http://localhost:30080/companies/<company-id>/scan
make k8s-logs-crawler
```

## Common failure modes

| Symptom                            | Cause                                                                      | Fix                                                                                                                                                                                                                       |
| ---------------------------------- | -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ErrImagePull` on all pods         | Wrong kubectl context (docker-desktop) or images not imported              | Switch context; `make k8s-build`                                                                                                                                                                                          |
| filter/enricher `CrashLoopBackOff` | `secrets/jobradar_NATS.creds` empty or wrong, or NATS secret not recreated | Fill creds file, run `make k8s-bootstrap`, then restart pods                                                                                                                                                              |
| `TLS handshake timeout`            | k3d API server still starting after restart                                | Wait ~30s; `until kubectl get nodes 2>/dev/null \| grep -q Ready; do sleep 3; done`                                                                                                                                       |
| Namespace stuck `Terminating`      | Finalizers not cleared                                                     | Force-clear: `kubectl get ns <name> -o json \| python3 -c "import sys,json; d=json.load(sys.stdin); d['spec']['finalizers']=[]; print(json.dumps(d))" \| kubectl replace --raw "/api/v1/namespaces/<name>/finalize" -f -` |
