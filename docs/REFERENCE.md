# JobRadar — Reference

All API routes require auth (`Authorization: Bearer <jwt>` or `X-Dev-Token` locally).
Base URL: `http://localhost:8080`

---

## API Routes

### Companies
| Method | Path | Source |
|---|---|---|
| GET | `/companies` | [CompanyRestRouter.go](../backend/service/company/services/CompanyRestRouter.go) |
| POST | `/companies` | [CompanyRestRouter.go](../backend/service/company/services/CompanyRestRouter.go) |
| PATCH | `/companies/active` | [CompanyRestRouter.go](../backend/service/company/services/CompanyRestRouter.go) |
| PATCH | `/companies/{id}` | [CompanyRestRouter.go](../backend/service/company/services/CompanyRestRouter.go) |
| DELETE | `/companies/{id}` | [CompanyRestRouter.go](../backend/service/company/services/CompanyRestRouter.go) |

### Scan
| Method | Path | Source |
|---|---|---|
| GET | `/companies/queue` | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |
| DELETE | `/companies/queue/{entry_id}` | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |
| POST | `/companies/scan-all` | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |
| POST | `/companies/{id}/scan` | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |
| GET | `/companies/{id}/scan-stats` | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |
| GET | `/companies/{id}/analytics` | [ScanRestRouter.go](../backend/service/scan/services/ScanRestRouter.go) |

### Adapter Config
| Method | Path | Source |
|---|---|---|
| GET | `/adapter-configs` | [AdapterConfigRestRouter.go](../backend/service/scan/services/AdapterConfigRestRouter.go) |
| PATCH | `/adapter-configs/{adapter_type}` | [AdapterConfigRestRouter.go](../backend/service/scan/services/AdapterConfigRestRouter.go) |
| PATCH | `/companies/{id}/scan-interval-override` | [AdapterConfigRestRouter.go](../backend/service/scan/services/AdapterConfigRestRouter.go) |
| GET | `/companies/{id}/scan-config` | [AdapterConfigRestRouter.go](../backend/service/scan/services/AdapterConfigRestRouter.go) |

### Jobs
| Method | Path | Source |
|---|---|---|
| GET | `/jobs` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| GET | `/jobs/counts` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| GET | `/jobs/{id}` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| DELETE | `/jobs/{id}` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| POST | `/jobs/dismiss/bulk` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| PATCH | `/jobs/{id}/save` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| PATCH | `/jobs/{id}/status` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |
| PATCH | `/jobs/{id}/notes` | [JobRestRouter.go](../backend/service/job/services/JobRestRouter.go) |

### Applications
| Method | Path | Source |
|---|---|---|
| GET | `/jobs/{id}/applications` | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| POST | `/jobs/{id}/applications` | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| PATCH | `/jobs/{id}/applications/{app_id}/stage` | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| PATCH | `/jobs/{id}/applications/{app_id}/note` | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| GET | `/applications` | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |
| PATCH | `/applications/{id}/stage` | [ApplicationRestRouter.go](../backend/service/application/services/ApplicationRestRouter.go) |

### Dashboard
| Method | Path | Source |
|---|---|---|
| GET | `/dashboard/stats` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/activity` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/progress` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/market` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/heatmap` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/interviews` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/velocity` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/tech-trends` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |
| GET | `/dashboard/pipeline` | [DashboardRestRouter.go](../backend/service/dashboard/services/DashboardRestRouter.go) |

### Insights
| Method | Path | Source |
|---|---|---|
| GET | `/insights/tech-stack` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET | `/insights/company-activity` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET | `/insights/funnel` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET | `/insights/location` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET | `/insights/sponsorship` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET | `/insights/velocity` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |
| GET | `/insights/role-categories` | [InsightsRestRouter.go](../backend/service/insights/services/InsightsRestRouter.go) |

---

## Kafka Topics

| Topic | Producer | Consumer | Notes |
|---|---|---|---|
| `job_listings.raw` | crawler | filter | Main pipeline — new raw jobs |
| `job_listings.raw.retry.1m` | filter | filter | Transient failures — retry after 1 min |
| `job_listings.raw.retry.10m` | filter | filter | Retry after 10 min |
| `job_listings.raw.retry.1h` | filter | filter | Retry after 1 hour |
| `job_listings.raw.dlq` | filter | — | Unrecoverable — manual inspection |
| `job_listings.filtered` | filter | analyzer | Relevant jobs passed to enrichment |
| `job_listings.filtered.retry.1m` | analyzer | analyzer | Transient failures — retry after 1 min |
| `job_listings.filtered.retry.10m` | analyzer | analyzer | Retry after 10 min |
| `job_listings.filtered.retry.1h` | analyzer | analyzer | Retry after 1 hour |
| `job_listings.filtered.dlq` | analyzer | — | Unrecoverable — manual inspection |

Topic constants: [`backend/platform/kafka/topics.go`](../backend/platform/kafka/topics.go)

---

## LLM Slots

Config file: [`backend/common/config/llm-slots.yaml`](../backend/common/config/llm-slots.yaml)
API keys expand from env vars at load time (`${GEMINI_KEY}`).

| Worker | Model | RPM | RPD | TPM in |
|---|---|---|---|---|
| filter | `gemini-3.1-flash-lite-preview` | 15 | 200 | 250k |
| enricher | `gemma-4-31b-it` | 15 | 1,500 | 1M |
