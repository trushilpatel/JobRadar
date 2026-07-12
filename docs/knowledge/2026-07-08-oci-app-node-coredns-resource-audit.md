# OCI APP Node Resource Audit — CoreDNS Crash-Loop Root Cause & Fix

**Date:** 2026-07-08
**Symptom:** APP node (`147.224.253.250`) CoreDNS restarting ~44 times/41h; filter/enricher intermittently failing to resolve Supabase/NATS/LLM hosts; live `TLS handshake timeout` on the k3s API server under load.
**Status:** CoreDNS hardening, zram, and image pruning are **live** on both prod nodes. The `dnsPolicy` Helm template change is **implemented but not yet deployed** — it ships on the next merge to `main` via the normal CI/CD pipeline.
**Touched:** `k8s/helm/jobradar/templates/{scheduler,orchestrator,filter,enricher}/deployment.yaml`, `scripts/oci-node-bootstrap.sh`, `infra/CLAUDE.md`

---

## What We Found

Live SSH inspection of both OCI production nodes (see `infra/CLAUDE.md` § OCI Production Deployment for node roles).

| Metric                     | INFRA (`161.153.66.17`) | APP (`147.224.253.250`)                                   |
| -------------------------- | ----------------------- | --------------------------------------------------------- |
| Load avg (2 vCPU capacity) | 0.17–0.30               | up to **7.33** (366% of capacity)                         |
| CPU steal (peak)           | ~1%                     | up to **55%**                                             |
| iowait (peak)              | ~1%                     | up to **66%**                                             |
| Swap used                  | 347Mi / 2Gi             | 561Mi / 2Gi, actively thrashing                           |
| CoreDNS restarts           | 27 / 24 days            | **44 / 41 hours**                                         |
| Pipeline pod restarts      | n/a (api/console: 0)    | all 5 pods restarted together ~2026-07-07 06:30–07:12 UTC |

The APP node's own k3s API server logged `TLS handshake timeout` and `"FinishRequest: post-timeout activity"` live during the audit — this is not a historical/reboot-only issue, it was actively happening at investigation time.

### Root cause chain

1. Oracle's ARM A1 free-tier hypervisor oversubscribes the node — CPU steal up to 55% is external contention, not fixable from our side.
2. That steal, combined with disk-backed swap thrashing, pushes iowait to 48–66% under normal pipeline load (APP runs 5 pods vs. INFRA's 2).
3. CoreDNS's **stock** liveness/readiness probes (`timeoutSeconds:1`) can't survive that — a healthy CoreDNS misses the deadline and gets restarted or pulled from Service endpoints.
4. Every pod in the chart defaults to `dnsPolicy: ClusterFirst` (unset anywhere in `k8s/helm/jobradar/templates/`), so **all** DNS — including external lookups to Supabase, NATS (`connect.ngs.global`), and the Gemini LLM API — routes through CoreDNS. When it drops, filter/enricher/scheduler/orchestrator all lose DNS even though none of them actually need Kubernetes service discovery.

### The mitigation already existed and had silently reverted

`scripts/oci-node-bootstrap.sh` already had a CoreDNS probe-widening step (added previously), with its own comment warning: _"k3s re-applies its bundled CoreDNS addon on k3s restart/upgrade, which reverts this patch. Re-run this script... if restarts become frequent."_ Live inspection confirmed CoreDNS was running with the **stock** (unwidened) probe values — the patch had been applied once, reverted on a later k3s restart, and nobody re-ran the script. This was the actual proximate cause, not a novel problem.

### Which services actually need CoreDNS

Traced the real call graph (not just the pipeline diagram):

| Service      | In-cluster DNS need                                                                                                                    |
| ------------ | -------------------------------------------------------------------------------------------------------------------------------------- |
| crawler-node | **Yes** — polls orchestrator at `http://jobradar-orchestrator.jobradar-local.svc.cluster.local:8082` (`k8s/helm/jobradar/values.yaml`) |
| orchestrator | No — only Supabase + NATS; never calls another pod itself                                                                              |
| scheduler    | No — only Supabase + NATS                                                                                                              |
| filter       | No — only Supabase, NATS, LLM API                                                                                                      |
| enricher     | No — only NATS, LLM API, external ATS hosts                                                                                            |

Also confirmed live: every service's `otelEndpoint` config points at `http://obs-alloy.jobradar-local.svc.cluster.local:4318` (another in-cluster DNS name, for traces), but **no `alloy` pod or service exists on either node today** — that dependency is currently a silent no-op, not an active cause of anything, but will become a second real reason to need CoreDNS once the observability chart (in progress — see untracked `infra/observability/dashboards/*.json`) is deployed to APP.

Empirically verified (via a debug pod on the live node) that `dnsPolicy: Default` pods resolve external hosts fine through `169.254.169.254` (OCI's DNS) without touching CoreDNS at all.

---

## The Fix

### 1. `dnsPolicy: Default` on the four DNS-external-only services

`k8s/helm/jobradar/templates/{scheduler,orchestrator,filter,enricher}/deployment.yaml` — each pod spec now sets `dnsPolicy: Default`, with a comment documenting the otel-endpoint tradeoff so it's not a silent trap if Alloy gets deployed later. `crawler-node` is untouched (still `ClusterFirst`, the k8s default) since it's the only one that needs Service-name resolution.

Effect once deployed: if CoreDNS crash-loops again, only crawler-node's ability to fetch new work stalls briefly — filter/enricher/scheduler/orchestrator keep running.

**Not yet live** — Helm template changes only take effect through the normal `release.yml` CI/CD pipeline (PR → `dev` → `main` → auto-deploy). Verified with `helm lint` and `helm template` (exactly 4 deployments render `dnsPolicy: Default`; `crawler-node` renders none).

### 2. CoreDNS probe hardening promoted to a self-healing systemd timer

`scripts/oci-node-bootstrap.sh` — replaced the one-off `kubectl patch` with `/usr/local/bin/jobradar-coredns-harden.sh` run by `jobradar-coredns-harden.timer` (every 3 min, idempotent no-op when already applied). Also widened readiness probe timing this time (previously only liveness was patched; readiness at `periodSeconds:2/timeoutSeconds:1` was flapping Service endpoints independently of restarts).

Applied live on both nodes. Verified probe values took effect and the timer is `active`.

### 3. zram on the APP node

Installed `zram-tools`, configured `/etc/default/zramswap`: `ALGO=lz4 SIZE=128 PRIORITY=100` — a 128Mi compressed-RAM swap device at a higher priority than the existing disk-backed `/swapfile` (`-2`), so the kernel prefers it first. Bounds worst-case extra RAM overhead at 128Mi on a box with almost no slack. Not applied to INFRA (no measured pressure there).

Documented in `infra/CLAUDE.md` § "Why swap is required". **Not yet added to `oci-node-bootstrap.sh`** — installed manually on the live node only; a future re-bootstrap of APP (or a third node taking its role) would not get this automatically.

### 4. Image pruning

Cross-referenced the live-running SHA per service against Helm's own release history (`helm get values ... --revision N`) before deleting anything — kept current (`05db2de`) and the N-1 rollback revision (`d762fd4`) for all 5 services, removed everything else (superseded SHA tags, floating `latest` tags never referenced by prod deploys, the orphaned `jobradar-backend` v1-monolith image, and the pre-rename `jobradar-crawler` image). Reclaimed ~4GB (`/dev/sda1`: 15G → 11G used).

### 5. Doc fixes

`infra/CLAUDE.md` — "CrawlerNode ×3" corrected to "×1" (actual `values.prod-app.yaml` pins `replicas: 1` for the NATS 4-connection budget; the table was stale and risked a future contributor "fixing" it back to 3). Added a new OCI-failure-modes row distinguishing this steady-state CPU-steal cause from the existing reboot/iptables-triggered CoreDNS failure mode.

---

## Live Incident During Rollout

Patching CoreDNS's Deployment (to widen its probes) changed its pod template, which triggered a Kubernetes rolling update — at a moment the APP node was already under load (load avg ~4.8–5). The new CoreDNS pod sat `Pending` for ~1 minute (the API server itself was timing out under CPU steal) while the old pod was already marked `Terminating`, producing a real ~1 minute DNS gap.

`filter` hit that gap (`db open failed: ... read udp ...:53: connection refused`) and, being already on `ClusterFirst` (pre-fix), went into `CrashLoopBackOff`. Once the new CoreDNS pod reached `1/1 Running` (confirmed via polling), filter's _next_ scheduled retry would have succeeded on its own, but its kubelet backoff had grown to several minutes — force-deleted the pod to get an immediate clean restart rather than waiting it out. All 5 pipeline pods + CoreDNS confirmed healthy afterward.

This is itself evidence for the fix: had `dnsPolicy: Default` already been deployed, filter would not have depended on CoreDNS during that gap at all.

---

## Current State

Both nodes: CoreDNS `1/1 Running`, all pipeline pods `Running`, hardening timer `active`. zram holding ~127Mi of swapped data on APP (near its 128Mi cap — saturated but functioning; see follow-ups).

---

## Not Done / Follow-ups

- **Deploy the `dnsPolicy` change** — needs a PR merged to `dev`/`main`; not yet pushed.
- **zram is already near its 128Mi cap** — if swap pressure persists after this settles, raising `SIZE` in `/etc/default/zramswap` to ~192–256 is the next lever (traded off against the box's near-zero RAM slack).
- **Add the zram systemd config to `oci-node-bootstrap.sh`** — currently only applied by hand on the live node; not reproducible for a re-bootstrap or a third node.
- **CPU limits sum to 90% of node capacity** with no headroom reserved for k3s/containerd/CoreDNS itself — not touched; lowering limits risks throttling real work and deserves its own decision, not a bundled change.
- **INFRA doesn't need zram** — no measured pressure; re-check with `vmstat`/`kubectl top` before adding it there.
- **Oracle CPU steal (up to 55%)** is hypervisor-side oversubscription, a noisy-neighbor effect — not fixable by workload tuning. If it stays this severe, the fallback is a paid always-on shape or re-provisioning the instance.
- **Self-hosting NATS in-cluster** would remove the 4-connection budget that currently caps `crawlerNode.replicas` at 1 — a bigger project, not started.

---

## How to Diagnose Similar Issues in Future

1. **Check whether a documented mitigation actually reverted.** `oci-node-bootstrap.sh`'s own comments are the first place to look — several steps note k3s/OCI will silently undo them on restart.
2. **Distinguish steady-state resource starvation from the reboot-triggered iptables failure mode** — both produce `CrashLoopBackOff`/`TLS handshake timeout` symptoms but have different fixes (see the two separate rows in `infra/CLAUDE.md` § OCI failure modes).
3. **Before patching any live Deployment's pod template on the APP node, expect a rollout** — even a probe-only change creates a new ReplicaSet. On a node already near capacity, time it carefully and watch the rollout complete before moving to the next change.
4. **`vmstat 1 5` is the fastest signal** — `wa` (iowait) and `st` (CPU steal) columns immediately show whether a node is externally starved vs. internally misconfigured.
