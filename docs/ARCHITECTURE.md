# JobRadar — System Architecture

## System Map

```mermaid
graph TB
  subgraph Browser
    U[User]
  end

  subgraph GitHub
    GHA[GitHub Actions]
    GHCR[Container Registry\nGHCR]
    GHA -->|push image| GHCR
  end

  subgraph Cloud
    Supa[Supabase\nAuth + PostgreSQL]
    Gemini[Google Gemini\nLLM API]
  end

  subgraph VPS["VPS — Oracle Linux 9 (Docker Compose)"]
    Caddy[Caddy\nTLS · Reverse Proxy\n:80 / :443]

    subgraph App["Application"]
      Console[Frontend\nReact + Nginx\n:3000]
      API[API Server\nChi router\n:8080]
    end

    subgraph Workers["Pipeline Workers"]
      Scheduler[Scheduler\nevery 1 min]
      Crawler[Crawler\nATS adapters]
      Filter[Relevance Filter\nLLM batch scoring]
      Analyzer[Job Post Analyzer\nLLM enrichment]
    end

    subgraph Infra["Infrastructure"]
      Kafka[Kafka / Redpanda\n:29092 internal]
      Redis[Redis\n:6379]
    end

    subgraph Obs["Observability"]
      Prometheus[Prometheus\n:9090]
      Grafana[Grafana\n:3001]
      Loki[Loki\n:3100]
      Jaeger[Jaeger\n:16686]
    end
  end

  U -->|HTTPS| Caddy
  Caddy --> Console
  Caddy --> API
  Caddy -->|grafana.*| Grafana

  Console -->|magic link OTP| Supa
  Console -->|REST| API
  API -->|JWT verify + data| Supa

  Scheduler -->|INSERT scan queue| Supa
  Crawler -->|CLAIM queue row + scan log| Supa
  Crawler -->|PUBLISH raw jobs| Kafka
  Filter -->|CONSUME| Kafka
  Filter -->|score titles| Gemini
  Filter -->|rate limit| Redis
  Filter -->|PUBLISH relevant| Kafka
  Filter -->|mark irrelevant| Supa
  Analyzer -->|CONSUME| Kafka
  Analyzer -->|extract structured data| Gemini
  Analyzer -->|rate limit| Redis
  Analyzer -->|INSERT job_feed + analytics| Supa

  Prometheus -->|datasource| Grafana
  Loki -->|datasource| Grafana
  API & Crawler & Filter & Analyzer -.->|OTel traces| Jaeger
  API & Scheduler & Crawler & Filter & Analyzer -.->|/metrics :910x| Prometheus

  GHA -->|SSH + docker compose up| VPS
  GHCR -->|docker pull| VPS
```

---

## Job Pipeline — Data Flow

```mermaid
flowchart LR
  A([companies\ntable]) -->|stale or never scanned| B[Scheduler\nevery 60s]
  B -->|INSERT| C[(company_scan_queue)]
  C -->|CLAIM row| D[Crawler\nATS adapters]
  D -->|fetch careers page| D
  D -->|dedup via ledger| E[(job_pipeline_ledger)]
  D -->|PUBLISH| F[/job_listings.raw/]
  D -->|UPDATE| G[(company_scan_log)]

  F -->|CONSUME batch 50–100| H[Relevance Filter\nLLM scoring]
  H -->|relevant| I[/job_listings.filtered/]
  H -->|irrelevant| E

  I -->|CONSUME| J[Job Post Analyzer\nLLM enrichment]
  J -->|fetch full page HTML| J
  J -->|INSERT| K[(job_feed)]
  J -->|INSERT| L[(job_analytics)]
  J -->|UPDATE| E
```

---

## CI/CD Pipeline (Scheduler)

```mermaid
flowchart LR
  A[Push to main\nbackend/** changed] --> B[Lint\nGo + Frontend]
  B --> C[Build\njobradar_backend image]
  C --> D[Push\nGHCR :sha-abc123]
  D --> E[SSH into VPS]
  E --> F[docker compose pull scheduler]
  F --> G[docker compose up -d scheduler\nzero-downtime restart]
```

---

## Database Tables

| Table | Written by | Read by | Purpose |
|---|---|---|---|
| `companies` | API | Scheduler, Crawler | Tracked company catalog |
| `company_scan_queue` | Scheduler, API | Crawler | Durable work queue — one row per pending scan |
| `company_scan_log` | Crawler | API / Dashboard | Per-run scan summary (fetched, new, errors) |
| `job_pipeline_ledger` | Crawler, Filter, Analyzer | Crawler | Dedup + pipeline state tracking |
| `job_feed` | Analyzer | API | Active jobs — new / saved / applied / dismissed |
| `job_analytics` | Analyzer | API / Insights | Append-only enriched job snapshot; never deleted |
| `job_application_tracker` | API | API | Application stage history per job |
| `job_dismissal_log` | API | API | Dismiss log with reason and note |

---

## Component Reference

| Component | Binary | Port | Profile | Memory |
|---|---|---|---|---|
| API Server | `/app/api` | `:8080` | `app` | 128 MB |
| Scheduler | `/app/scheduler` | `:9101` | `workers` | 128 MB |
| Crawler | `/app/crawler` | `:9102` | `workers` | 1 GB (Chromedp) |
| Relevance Filter | `/app/filter` | `:9103` | `workers` | 128 MB |
| Job Post Analyzer | `/app/enricher` | `:9104` | `workers` | 128 MB |
| Migration runner | `/app/migrate` | — | `app` | — |
| Frontend (Nginx) | — | `:3000` | `app` | — |
| Caddy | — | `:80/:443` | `proxy` | — |
| Kafka (Redpanda) | — | `:29092` | `infra` | 256 MB |
| Redis | — | `:6379` | `infra` | — |
| Prometheus | — | `:9090` | `obs` | — |
| Grafana | — | `:3001` | `obs` | — |
| Loki | — | `:3100` | `obs` | — |
| Jaeger | — | `:16686` | `obs` | — |

---

## Key Design Decisions

- **All binaries ship in one image** (`jobradar_backend`) — built once, deployed by selecting the command (`/app/api`, `/app/scheduler`, etc.)
- **Kafka is the pipeline bus** — no direct worker-to-worker calls; failures recorded in `job_pipeline_ledger`, not re-queued
- **PostgreSQL is Supabase-hosted** — auth and DB in one managed service; no self-hosted Postgres container
- **Caddy is the sole public ingress** — all services bind loopback internally; only Caddy binds `:80`/`:443`
- **LLM rate limiting via Redis** — Filter and Analyzer share a Redis-backed slot pool to stay within Gemini quotas
