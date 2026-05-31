# k8s Helm Optimization — 2026-05-23

## Summary

Three targeted fixes to the `k8s/helm/jobradar` chart addressing resource correctness, security hardening, and infrastructure cleanup.

## Changes

**Resource rightsizing** — CPU limits added to all workloads (was missing entirely, allowing runaway pods to starve k3d). Crawler memory ceiling dropped from 1Gi to 256Mi (pure-HTTP poller). Scheduler memory ceiling dropped from 256Mi to 128Mi. Alloy memory ceiling dropped from 512Mi to 384Mi.

**Crawler image hardening** — base image switched from `debian:bookworm-slim` to `alpine:3.20`. CGO_ENABLED=0 binary has no glibc dependency; alpine is ~8 MB vs ~80 MB. Non-root user UID 65532 added, matching the k8s `securityContext.runAsUser` used by all other workloads.

**Console pod hardening** — `readOnlyRootFilesystem: true` enabled. Nginx requires three writable paths at runtime (`/tmp`, `/var/cache/nginx`, `/var/run`); all three mounted as in-memory `emptyDir` volumes with explicit size limits.

**Scheduler probes** — liveness and readiness probes added on `:9103` (metrics server already exposed `/health` and `/ready`; scheduler was the only worker without k8s health checks).

**Nginx forwarding headers** — `X-Forwarded-For` and `X-Forwarded-Proto` added to both proxy locations in the ConfigMap and `nginx.conf` so the API receives real client IP and protocol.

**migrate job backoffLimit** — reduced from 10 to 3 to avoid creating 10 failed pods on cold-start DB miss.

**Docker Compose removed** — k3d is now the single deployment target. Docker Compose, self-hosted observability stack (Loki/Prometheus/Grafana/Promtail), Caddy TLS ingress, CrowdSec WAF, VPS bootstrap scripts, and the legacy Kafka entrypoint were all deleted.

## Affected files

- `k8s/helm/jobradar/values.yaml`
- `k8s/helm/jobradar/templates/*/deployment.yaml` (all workloads)
- `k8s/helm/jobradar/templates/migrate/job.yaml`
- `k8s/helm/jobradar/templates/console/configmap.yaml`
- `k8s/helm/observability/values.yaml`
- `infra/Dockerfile.crawler`
- `frontend/nginx.conf`
- `Makefile`
- `infra/CLAUDE.md`
