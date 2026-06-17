# JobRadar — Architecture

## System Map

```mermaid
graph TB
  subgraph Browser
    U[User]
  end

  subgraph Cloud["Managed / SaaS"]
    Supa[Supabase\nAuth + PostgreSQL]
    Gemini[Google Gemini\nLLM API]
    Upstash[Upstash Redis\nManaged · TLS]
    NATS[Synadia Cloud\nNATS JetStream]
    GCloud[Grafana Cloud\nMimir · Loki · Tempo]
  end

  subgraph K8S["Kubernetes — jobradar-local (Helm)"]
    subgraph App["Application  (helm: jobradar)"]
      Console[Frontend\nReact + Nginx\nNodePort :30300]
      API[API Server\nChi router\nNodePort :30080]
    end

    subgraph Workers["Pipeline Workers  (helm: jobradar)"]
      Scheduler[Scheduler\nevery 1 min]
      Orchestrator[Orchestrator\nHTTP work assignment]
      CrawlerNode[Crawler-node ×N\ncrawler-v2 ATS adapters]
      Filter[Relevance Filter\nLLM batch scoring]
      Analyzer[Job Post Analyzer\nLLM enrichment]
    end

    subgraph Obs["Observability  (helm: obs)"]
      Alloy[Grafana Alloy\nDaemonSet\nscrape · tail · OTLP :4317/:4318]
      AuditSync[supabase-audit-sync\nSupabase audit → Loki]
    end
  end

  U -->|HTTPS| Console
  U -->|HTTPS| API
  Console -->|magic link OTP| Supa
  Console -->|REST| API
  API -->|JWT verify + data| Supa

  Scheduler -->|INSERT scan queue| Supa
  CrawlerNode -->|poll for work / submit results| Orchestrator
  Orchestrator -->|CLAIM queue row + scan log| Supa
  Orchestrator -->|PUBLISH raw jobs| NATS
  Filter -->|CONSUME| NATS
  Filter -->|score titles| Gemini
  Filter -->|rate limit| Upstash
  Filter -->|PUBLISH relevant| NATS
  Filter -->|mark irrelevant| Supa
  Analyzer -->|CONSUME| NATS
  Analyzer -->|extract structured data| Gemini
  Analyzer -->|rate limit| Upstash
  Analyzer -->|INSERT jobs + analytics| Supa

  API & Scheduler & Orchestrator & CrawlerNode & Filter & Analyzer -.->|/metrics + OTLP traces| Alloy
  Alloy -.->|remote_write metrics + logs + OTLP traces| GCloud
  AuditSync -->|read audit log| Supa
  AuditSync -.->|push logs| GCloud
```

---

## Pipeline — Data Flow

```mermaid
flowchart LR
  A([companies]) -->|stale or never scanned| B[Scheduler\nevery 60s]
  B -->|INSERT| C[(company_scan_queue)]
  C -->|CLAIM row| D[Orchestrator + Crawler-node\ncrawler-v2 ATS adapters]
  D -->|dedup via ledger| E[(job_pipeline_ledger)]
  D -->|PUBLISH| F[/job_listings.raw/]
  D -->|UPDATE| G[(company_scan_log)]

  F -->|CONSUME batch| H[Filter\nLLM scoring]
  H -->|relevant| I[/job_listings.filtered/]
  H -->|irrelevant| E

  I -->|CONSUME| J[Analyzer\nLLM enrichment]
  J -->|INSERT| K[(jobs)]
  J -->|INSERT| L[(job_analytics)]
  J -->|UPDATE| E
```

---

## CI

GitHub Actions enforces lint, type-check, and unit tests on PRs and push to `main`. On push to `main`, changed service images are built and deployed automatically to OCI nodes.

| Workflow                             | Trigger             | Purpose                                                                  |
| ------------------------------------ | ------------------- | ------------------------------------------------------------------------ |
| `.github/workflows/ci.yml`           | PR + push to `main` | golangci-lint, ESLint, TypeScript check, Go unit tests, Codecov on push  |
| `.github/workflows/release.yml`      | push to `main`      | Detect changes → build per-service images → deploy INFRA + APP OCI nodes |
| `.github/workflows/deploy.yml`       | Manual dispatch     | Rollback: pick node (infra/app/both) + Helm revision                     |
| `.github/workflows/fix-app-node.yml` | Manual dispatch     | Run `oci-k3s-netfix.sh` on the APP node when k3s is not Ready            |

---

## Database Tables

| Table                     | Written by                                                | Read by                 | Purpose                                                                                       |
| ------------------------- | --------------------------------------------------------- | ----------------------- | --------------------------------------------------------------------------------------------- |
| `companies`               | API (admin)                                               | Scheduler, Orchestrator | Tracked company catalog (operational columns dropped; scan config in `company_scan_settings`) |
| `company_scan_settings`   | API (admin)                                               | Scheduler, Orchestrator | Per-company scan config; companies without a row are skipped                                  |
| `company_scan_queue`      | Scheduler, API                                            | Orchestrator            | Durable work queue — one row per pending scan                                                 |
| `company_scan_log`        | Orchestrator                                              | API / Dashboard         | Per-run scan summary                                                                          |
| `job_pipeline_ledger`     | Orchestrator, Filter, Analyzer                            | Orchestrator            | Dedup + pipeline state tracking                                                               |
| `jobs`                    | Analyzer                                                  | API                     | Global job rows (renamed from `job_feed`); per-user state in `user_job_state`                 |
| `user_job_state`          | API                                                       | API                     | Per-user overlay: status, notes, saved_at, applied_at, dismissal reason/note                  |
| `job_analytics`           | Analyzer                                                  | Insights                | Append-only enriched snapshot; never deleted                                                  |
| `job_application_tracker` | API                                                       | API                     | Application stage history per job; `user_id NOT NULL`                                         |
| `users`                   | API (`RequireIdentity` / `service/user`)                  | API                     | Registered users — id, email (citext), display_name, status                                   |
| `roles`                   | Migration `043` (seeded)                                  | API                     | RBAC0 role catalog — `user`, `admin`                                                          |
| `user_roles`              | API (`service/role`, `RequireIdentity` lazy provisioning) | API                     | User↔role join (composite PK)                                                                 |
| `admin_audit_log`         | `AuditAdminMutation` middleware (`service/audit`)         | API                     | Append-only admin audit trail                                                                 |

---

## Component Reference

| Component           | Binary              | Internal port              | External            | Helm chart | Mem limit       |
| ------------------- | ------------------- | -------------------------- | ------------------- | ---------- | --------------- |
| API Server          | `/app/api`          | `:8080` / `:9100` metrics  | NodePort **:30080** | jobradar   | 128 MB          |
| Scheduler           | `/app/scheduler`    | `:9101` metrics            | —                   | jobradar   | 128 MB          |
| Orchestrator        | `/app/orchestrator` | `:8082` / `:9104` metrics  | —                   | jobradar   | 128 MB          |
| Crawler-node        | `/app/crawler-node` | `:9105` metrics            | —                   | jobradar   | 64 MB (per pod) |
| Filter              | `/app/filter`       | `:9101` metrics            | —                   | jobradar   | 128 MB          |
| Analyzer            | `/app/enricher`     | `:9101` metrics            | —                   | jobradar   | 128 MB          |
| Migration runner    | `/app/migrate`      | —                          | —                   | jobradar   | 128 MB          |
| Frontend (Nginx)    | —                   | `:8080`                    | NodePort **:30300** | jobradar   | 64 MB           |
| NATS JetStream      | —                   | `tls://connect.ngs.global` | Managed (Synadia)   | —          | —               |
| Upstash Redis       | —                   | `:6379` (TLS)              | Managed (external)  | —          | —               |
| Grafana Alloy       | —                   | `:4317`/`:4318` OTLP       | DaemonSet           | obs        | 384 MB          |
| supabase-audit-sync | —                   | —                          | —                   | obs        | 64 MB           |
| Grafana Cloud       | —                   | —                          | SaaS                | —          | —               |
