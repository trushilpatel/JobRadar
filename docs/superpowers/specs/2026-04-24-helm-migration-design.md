# Helm Migration Design

**Date:** 2026-04-24

## Goal

Migrate all existing raw Kubernetes manifests to Helm charts and add the full observability stack (Prometheus, Grafana, Loki, Promtail, Jaeger, supabase-audit-sync) that currently only exists in docker-compose.

## Environment

- **Cluster:** k3d (k3s in Docker), single node, local dev
- **Namespace:** `jobradar-local`
- **StorageClass:** `local-path` (k3d default) — explicitly set on all PVC requests

## Two-Chart Structure

```
k8s/
  helm/
    jobradar/           ← custom chart for all app services
    observability/      ← thin wrapper chart for observability stack
```

### Chart 1: `jobradar`

Custom Helm chart replacing all existing raw manifests under `k8s/`. Contains templates for:

| Component | Kind |
|---|---|
| api | Deployment + Service (http :8080, metrics :9100) |
| console | Deployment + Service |
| scheduler | Deployment + ClusterIP Service (:9101) |
| crawler | Deployment + ClusterIP Service (:9102) |
| filter | Deployment + ClusterIP Service (:9103) |
| enricher | Deployment + ClusterIP Service (:9104) |
| kafka | StatefulSet + Service (ClusterIP + Headless) |
| redis | StatefulSet + Service |
| migrate | Job |

> **Note:** The worker formerly called `analyzer` in the k8s manifests maps to `enricher` in prometheus.yml. All Helm templates and scrape targets use `enricher` consistently.

Each worker gets a dedicated ClusterIP Service exposing its metrics port so Prometheus can scrape it by DNS name.

All existing ConfigMaps and Secrets become `values.yaml` defaults. Secrets reference k8s Secret objects (not stored in chart).

### Chart 2: `observability`

Thin umbrella chart with community chart dependencies + one custom template:

| Component | Source | Notes |
|---|---|---|
| Prometheus + Grafana | `prometheus-community/kube-prometheus-stack` | AlertManager and node-exporter disabled in values |
| Loki + Promtail | `grafana/loki-stack` | Promtail enabled inside loki-stack; no standalone Promtail dep |
| Jaeger | `jaegertracing/jaeger` | NodePort pinned for UI |
| supabase-audit-sync | Custom Deployment template | See config below |

#### kube-prometheus-stack values (key overrides)

```yaml
alertmanager:
  enabled: false
nodeExporter:
  enabled: false
grafana:
  service:
    type: NodePort
    nodePort: 3001
  additionalDataSources:
    - name: Loki
      type: loki
      url: http://loki:3100
  dashboardProviders:
    dashboardproviders.yaml:
      providers:
        - name: jobradar
          folder: JobRadar
          type: file
          options:
            path: /var/lib/grafana/dashboards/jobradar
  dashboards:
    jobradar:
      # mounted from ConfigMap built from infra/grafana/dashboards/*.json
```

#### Prometheus scrape targets (static, in values)

```yaml
prometheus:
  additionalScrapeConfigs:
    - job_name: jobradar_api
      static_configs:
        - targets: ['jobradar-api.jobradar-local.svc.cluster.local:9100']
    - job_name: jobradar_scheduler
      static_configs:
        - targets: ['jobradar-scheduler.jobradar-local.svc.cluster.local:9101']
    - job_name: jobradar_crawler
      static_configs:
        - targets: ['jobradar-crawler.jobradar-local.svc.cluster.local:9102']
    - job_name: jobradar_filter
      static_configs:
        - targets: ['jobradar-filter.jobradar-local.svc.cluster.local:9103']
    - job_name: jobradar_enricher
      static_configs:
        - targets: ['jobradar-enricher.jobradar-local.svc.cluster.local:9104']
```

#### Loki datasource wiring

Loki is NOT auto-wired by kube-prometheus-stack. It requires an explicit entry under `grafana.additionalDataSources` in kube-prometheus-stack values (shown above).

#### Promtail (inside loki-stack)

Uses k8s pod log discovery — mounts `/var/log/pods` on the node. Configured via `loki-stack.promtail.config` in values. No Docker socket access needed.

#### supabase-audit-sync

Custom Deployment template in this chart. Config carried over from docker-compose:

| Env var | Source |
|---|---|
| `SUPABASE_DB_URL` | k8s Secret |
| `LOKI_URL` | `http://loki:3100/loki/api/v1/push` |
| `AUDIT_SYNC_INTERVAL_SECONDS` | values.yaml (default 900) |
| `AUDIT_SYNC_STATE_FILE` | `/var/lib/audit-sync/state.json` (PVC) |

Image: `jobradar_supabase_audit_sync` (built locally, same as docker-compose).

## Access

| Service | Method | Address |
|---|---|---|
| Grafana | NodePort (pinned) | `localhost:3001` |
| Jaeger UI | NodePort (pinned) | `localhost:16686` |
| Prometheus | ClusterIP (port-forward) | `kubectl port-forward svc/prometheus 9090:9090` |
| Loki | ClusterIP internal only | — |

> NodePort values must be explicitly set in chart values — both kube-prometheus-stack and jaeger assign them dynamically by default.

## Install Commands

```bash
helm install jobradar ./k8s/helm/jobradar -n jobradar-local
helm install obs ./k8s/helm/observability -n jobradar-local
```

## Migration Sequence

1. Scaffold `jobradar` chart structure + Chart.yaml + values.yaml
2. Migrate app services one by one (api → console → workers → kafka → redis → migrate job)
3. Validate all pods healthy via `kubectl get pods -n jobradar-local`
4. Delete old raw manifests under `k8s/` (everything except `k8s/helm/`)
5. Scaffold `observability` chart with Chart.yaml dependencies + values.yaml
6. Build Grafana dashboard ConfigMap from `infra/grafana/dashboards/*.json`
7. `helm dependency update` then deploy observability chart
8. Verify Grafana shows metrics (Prometheus) and logs (Loki)
9. Deploy supabase-audit-sync template, verify logs appear in Loki
