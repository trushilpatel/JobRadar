# JobRadar — Product Requirements Document

> End-to-end automated job tracking system with multi-platform ATS support, built in Go with a full-featured UI.

---

## 1. Project Overview

### 1.1 Problem Statement

Job seekers targeting 50–100 specific companies face a painful manual process: visiting each company's careers page daily, checking for new roles, filtering out irrelevant postings, and keeping track of what they've seen and applied to. This is repetitive, time-consuming, and easy to miss.

### 1.2 Solution

JobRadar is an automated job tracking system that:

- Monitors up to 100+ company careers pages on a configurable schedule
- Auto-detects which ATS platform each company uses and uses the optimal fetch strategy
- Diffs snapshots to surface only new jobs since the last scan
- Scores job matches against the user's defined keywords and preferences
- Delivers a clean UI for managing companies, reviewing jobs, and tracking applications
- Notifies the user via email or Slack when high-match jobs appear

### 1.3 Goals

- **Zero daily manual effort** — user opens the app and sees what's new, already ranked
- **Easily extensible** — adding a new company or ATS platform should take minutes, not hours
- **Low infrastructure** — runs as a single Go binary, stores data in SQLite, no cloud required
- **Cost-efficient** — minimize or eliminate Claude API usage by using native ATS APIs wherever possible

---

## 2. System Architecture

### 2.1 High-Level Layers

```
cmd/                  → binary entrypoints (daemon + CLI)
internal/port/        → interfaces only — the contract layer
internal/model/       → shared structs, no business logic
internal/core/        → business logic (engine, diff, registry, pool)
internal/adapter/     → ATS platform implementations
internal/infra/       → store, notifier, scheduler implementations
config/               → config loading and validation
```

### 2.2 Core Design Principles

- **Dependency inversion** — core engine only depends on `port` interfaces, never on concrete adapters or infra
- **Plugin pattern** — adding a new ATS adapter requires one new file + one line in the registry
- **Interface segregation** — `Fetcher`, `Detector`, `Store`, `Notifier`, `Scheduler` are separate interfaces
- **Zero coupling between layers** — `internal/adapter` never imports `internal/infra` and vice versa
- **Pure functions where possible** — diff engine and normalizer are stateless, easily testable

### 2.3 Port Interfaces

```go
// internal/port/port.go

type Fetcher interface {
    Name() string
    FetchJobs(ctx context.Context) ([]model.Job, error)
}

type Detector interface {
    Detect(rawURL string) bool
}

type Adapter interface {
    Fetcher
    Detector
}

type Store interface {
    SaveSnapshot(company string, jobs []model.Job) error
    LoadSnapshot(company string) ([]model.Job, error)
    SaveDiff(result model.DiffResult) error
    LoadRecentDiffs(limit int) ([]model.DiffResult, error)
}

type Notifier interface {
    Notify(ctx context.Context, diff model.DiffResult) error
}

type Scheduler interface {
    Run(ctx context.Context) error
    Stop()
}
```

### 2.4 Normalized Job Model

All adapters map their platform-specific response to this shared struct:

```go
// internal/model/job.go

type Job struct {
    ID           string
    Title        string
    Company      string
    Location     string
    Remote       bool
    URL          string
    Department   string
    EmployType   string    // full-time, contract, part-time
    PostedAt     time.Time
    UpdatedAt    time.Time
    Description  string
    Source       string    // "greenhouse", "lever", "ashby", etc.
    Raw          any       // original response for debugging
}

type DiffResult struct {
    Company   string
    ScannedAt time.Time
    New       []Job
    Removed   []Job
}
```

---

## 3. ATS Adapter Layer

### 3.1 Supported Platforms

| Platform | Detection | Method | Auth | Pagination |
|---|---|---|---|---|
| Greenhouse | URL contains `greenhouse.io` | Native JSON API | None | None (single response) |
| Lever | URL contains `lever.co` | Native JSON API | None | None (single response) |
| Ashby | URL contains `ashbyhq.com` | Native JSON API | None | None (single response) |
| Workable | URL contains `workable.com` | Native JSON API | None | Cursor-based |
| SmartRecruiters | URL contains `smartrecruiters.com` | Native JSON API | None | Offset-based |
| Workday | URL contains `myworkdayjobs.com` | REST POST | None | Offset-based |
| Custom (chromedp) | Fallback — all other URLs | Headless browser | None | DOM-based |

### 3.2 Adapter API Endpoints

**Greenhouse**
```
GET https://boards-api.greenhouse.io/v1/boards/{token}/jobs?content=true
GET https://boards-api.greenhouse.io/v1/boards/{token}/jobs/{id}
```

**Lever**
```
GET https://api.lever.co/v0/postings/{company}?mode=json
```

**Ashby**
```
GET https://api.ashbyhq.com/posting-api/job-board/{company}?includeCompensation=true
```

**Workday**
```
POST https://{company}.wd1.myworkdayjobs.com/wday/cxs/{company}/{board}/jobs
Body: {"limit": 20, "offset": 0}
```

**SmartRecruiters**
```
GET https://api.smartrecruiters.com/v1/companies/{company}/postings?limit=100&offset={n}
```

### 3.3 Adding a New Adapter

To add support for a new platform (e.g. Apple, Meta, Google):

1. Create `internal/adapter/{platform}.go`
2. Implement `Detect(url string) bool` and `FetchJobs(ctx) ([]model.Job, error)`
3. Add the adapter to the registry slice in `internal/core/registry.go`
4. Done — no other files change

For JavaScript-heavy pages (SPAs, React apps), extend `internal/adapter/chromedp.go` base and override the page interaction logic.

### 3.4 Registry

```go
// internal/core/registry.go
var adapters = []port.Adapter{
    &GreenhouseAdapter{},
    &LeverAdapter{},
    &AshbyAdapter{},
    &WorkableAdapter{},
    &SmartRecruitersAdapter{},
    &WorkdayAdapter{},
    &GoogleAdapter{},    // chromedp
    &MetaAdapter{},      // chromedp
    &AppleAdapter{},     // chromedp
    &ChromedpFallback{}, // always last — catch-all
}

func Get(url string) port.Adapter {
    for _, a := range adapters {
        if a.Detect(url) {
            return a
        }
    }
    return &ChromedpFallback{URL: url}
}
```

---

## 4. Core Engine

### 4.1 Run Loop

```
Schedule trigger (cron / manual)
    → Worker pool: fetch all companies concurrently (max N goroutines)
    → Per company:
        registry.Get(url) → adapter.FetchJobs(ctx)
        store.LoadSnapshot(company)
        diff.Compute(prev, current)
        store.SaveSnapshot(company, current)
        store.SaveDiff(result)
        if len(result.New) > 0 → notifier.Notify(result)
    → Log scan summary
```

### 4.2 Worker Pool

- Configurable concurrency limit (default: 10 parallel fetches)
- Per-company timeout (default: 30s)
- Retry logic: 2 retries with exponential backoff on transient errors
- Errors are collected and reported in scan summary — one company failing does not stop others

### 4.3 Diff Engine

- Compares job IDs from previous snapshot vs current
- New = in current, not in previous
- Removed = in previous, not in current
- Uses `updated_at` / `createdAt` timestamps where available for accuracy
- Purely functional — no side effects, fully unit testable

### 4.4 Match Scoring

Each new job is scored 0–100 against the user's keyword profile:

- Title keywords match → up to 50 pts
- Location / remote preference match → up to 20 pts
- Department / team match → up to 20 pts
- Seniority level match → up to 10 pts

Score thresholds:
- 80–100 → High match (green)
- 60–79  → Medium match (amber)
- Below 60 → Low match (gray)

---

## 5. Configuration

### 5.1 config.yaml Structure

```yaml
schedule: "0 8 * * *"     # cron — run daily at 8am
concurrency: 10            # max parallel fetches
timeout: 30s               # per-company fetch timeout
retries: 2

keywords:
  titles:      ["backend engineer", "software engineer", "platform engineer", "golang"]
  locations:   ["remote", "san francisco", "new york"]
  departments: ["engineering", "infrastructure", "platform"]
  seniority:   ["senior", "staff", "principal"]
  exclude:     ["intern", "sales", "marketing"]

notifications:
  email:
    enabled: true
    smtp_host: smtp.gmail.com
    smtp_port: 587
    from: you@gmail.com
    to: you@gmail.com
  slack:
    enabled: false
    webhook_url: https://hooks.slack.com/...
  min_match_score: 70     # only notify for jobs above this score

store:
  type: sqlite            # sqlite | file
  path: ./data/jobs.db

companies:
  - name: GitHub
    url: https://job-boards.greenhouse.io/github
  - name: Stripe
    url: https://job-boards.greenhouse.io/stripe
  - name: Linear
    url: https://jobs.lever.co/linear
  - name: Vercel
    url: https://jobs.lever.co/vercel
  - name: Notion
    url: https://jobs.lever.co/notion
  - name: Google
    url: https://careers.google.com/jobs
  - name: Meta
    url: https://metacareers.com/jobs
```

### 5.2 CLI Commands

```bash
# Start the daemon (runs on schedule)
jobradar start

# Run a one-off scan immediately
jobradar scan

# Scan a single company
jobradar scan --company github

# List all companies and their adapter type
jobradar companies list

# Add a new company interactively
jobradar companies add --url https://jobs.lever.co/acme

# Show recent diffs
jobradar jobs new --since 24h

# Show match score for a job
jobradar jobs score --id greenhouse-12345
```

---

## 6. User Interface

### 6.1 Navigation

| Section | Purpose |
|---|---|
| Dashboard | Today's summary — new jobs, scan status, activity feed, weekly chart |
| New jobs | Full paginated feed of new jobs with search, filters, and match scores |
| Companies | Manage the list of 100 companies — add, edit, remove, view status |
| Applied | Kanban-style tracker for jobs the user has applied to |
| Saved | Bookmarked jobs for later review |

### 6.2 Dashboard

**Metric cards (top row):**
- New jobs today
- Companies monitored (with error count)
- Saved jobs (with keyword match count)
- Applied count

**New jobs today panel:**
- Shows top 5 highest-match jobs
- Each row: company logo/initials, job title, location, match score badge, time ago
- Clicking a row opens the job detail view

**Activity feed panel:**
- Timestamped log: scan completed, jobs found, errors, companies added, applications submitted

**Weekly chart:**
- Bar chart of jobs found per day over the last 7 days

**Scan errors strip:**
- Shows companies that failed the last scan with error reason
- Quick-fix button per entry

### 6.3 New Jobs Feed

- Full-text search across title, company, location, description
- Filter pills: All / High match / Remote / By department / Today / Saved
- Each job card shows: company, title, location, remote badge, match score, time posted
- Sort by: match score (default), newest, company name
- Infinite scroll or pagination (configurable)
- Mark as saved / dismiss / open original URL

### 6.4 Company Management

- List view with: company name, ATS platform badge, job count, last scan time, scan status
- Filter by ATS platform, scan status (ok / error)
- Search by company name
- Per-company actions: scan now, edit URL, pause monitoring, remove
- Scan status indicators: green (ok), amber (warning), red (error)

**Add company flow:**
1. User pastes careers page URL
2. System auto-detects ATS platform instantly (shown as green confirmation strip)
3. User optionally adds custom name and keywords for this company
4. Save — company is added to config and included in the next scan
5. Optional: trigger immediate scan for just this company

### 6.5 Job Detail View

- Full job title, company, location, remote, department, employment type
- Posted date and source platform
- Match score breakdown (which keywords matched and why)
- Full job description (rendered)
- Status selector: New / Saved / Applied / Interviewing / Rejected / Offer
- Notes field (free text, persisted)
- Action buttons: Apply (opens URL), Save, Dismiss
- History: when first seen, when status last changed

### 6.6 Applied / Kanban Tracker

Columns: Applied → Phone Screen → Technical → Final Round → Offer / Rejected

Each card shows company name, role title, date applied, last activity date. Drag-and-drop between columns. Click to open full detail with notes and timeline.

---

## 7. Notifications

### 7.1 Email Digest

- Sent after each scan if new high-match jobs found (score ≥ configured threshold)
- Groups jobs by company
- Each job: title, location, match score, direct link
- Plain text + HTML versions

### 7.2 Slack Notification

- One message per scan summary if new jobs found
- Attachment blocks per high-match job with Apply button

### 7.3 Notification Rules

- Configurable minimum match score threshold (default: 70)
- Configurable cooldown per company (e.g. don't re-notify same company within 24h unless new role)
- Option: notify on any new job, regardless of score

---

## 8. Storage

### 8.1 SQLite Schema (simplified)

```sql
-- companies
CREATE TABLE companies (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    url         TEXT NOT NULL,
    adapter     TEXT NOT NULL,   -- greenhouse, lever, etc.
    active      BOOLEAN DEFAULT TRUE,
    created_at  DATETIME,
    last_scan   DATETIME,
    last_status TEXT             -- ok | error | pending
);

-- jobs
CREATE TABLE jobs (
    id          TEXT PRIMARY KEY,
    company_id  TEXT REFERENCES companies(id),
    title       TEXT,
    location    TEXT,
    remote      BOOLEAN,
    url         TEXT,
    department  TEXT,
    employ_type TEXT,
    posted_at   DATETIME,
    first_seen  DATETIME,
    match_score INTEGER,
    status      TEXT DEFAULT 'new',   -- new|saved|applied|interviewing|rejected|offer
    notes       TEXT,
    raw_json    TEXT
);

-- scan_log
CREATE TABLE scan_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    scanned_at  DATETIME,
    company_id  TEXT,
    jobs_found  INTEGER,
    jobs_new    INTEGER,
    error       TEXT
);
```

### 8.2 File-Based Store (fallback)

- One JSON file per company snapshot: `data/snapshots/{company}.json`
- Diff results appended to `data/diffs.jsonl`
- Suitable for local use without SQLite dependency

---

## 9. External Dependencies

| Package | Purpose |
|---|---|
| `robfig/cron/v3` | Cron scheduling |
| `chromedp/chromedp` | Headless Chrome for JS-heavy pages |
| `mattn/go-sqlite3` | SQLite storage |
| `spf13/viper` | Config loading (YAML + env vars) |
| `go-mail/mail` | Email notifications |
| `slack-go/slack` | Slack notifications |
| `spf13/cobra` | CLI subcommands |

All other logic uses Go stdlib (`net/http`, `encoding/json`, `sync`, `context`, `time`).

---

## 10. Project File Structure

```
job-radar/
├── cmd/
│   ├── tracker/main.go        # daemon entrypoint
│   └── cli/main.go            # CLI entrypoint
├── internal/
│   ├── port/
│   │   └── port.go            # all interfaces
│   ├── model/
│   │   ├── job.go
│   │   └── diff.go
│   ├── core/
│   │   ├── engine.go          # main run loop
│   │   ├── registry.go        # adapter detection + routing
│   │   ├── diff.go            # diff computation
│   │   ├── normalizer.go      # raw → model.Job
│   │   ├── scorer.go          # keyword match scoring
│   │   └── worker_pool.go     # concurrent fetch
│   ├── adapter/
│   │   ├── greenhouse.go
│   │   ├── lever.go
│   │   ├── ashby.go
│   │   ├── workday.go
│   │   ├── workable.go
│   │   ├── smartrecruiters.go
│   │   ├── chromedp_base.go   # shared headless browser logic
│   │   ├── google.go
│   │   ├── meta.go
│   │   └── apple.go
│   └── infra/
│       ├── store/
│       │   ├── sqlite.go
│       │   └── file.go
│       ├── notify/
│       │   ├── email.go
│       │   └── slack.go
│       └── scheduler/
│           └── cron.go
├── config/
│   ├── config.go
│   └── config.yaml
├── ui/                        # frontend (web UI)
│   ├── src/
│   └── dist/
├── data/                      # runtime data (gitignored)
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 11. Development Phases

### Phase 1 — Core pipeline (MVP)
- [ ] Define all port interfaces
- [ ] Implement model structs
- [ ] Greenhouse, Lever, Ashby adapters
- [ ] Registry with auto-detection
- [ ] Diff engine
- [ ] SQLite store
- [ ] Worker pool with concurrency limit
- [ ] Engine run loop
- [ ] Config loading from YAML
- [ ] CLI: `scan`, `companies list`, `jobs new`

### Phase 2 — Full adapter coverage
- [ ] Workday adapter (POST-based pagination)
- [ ] Workable + SmartRecruiters adapters
- [ ] chromedp base adapter
- [ ] Google, Meta, Apple adapters (chromedp)
- [ ] Error reporting + retry logic
- [ ] Cron scheduler daemon

### Phase 3 — Notifications
- [ ] Match scorer
- [ ] Email notifier
- [ ] Slack notifier
- [ ] Notification rules engine (threshold, cooldown)

### Phase 4 — UI
- [ ] Dashboard with metrics + activity feed
- [ ] New jobs feed with search + filters
- [ ] Company management + add flow with ATS auto-detection
- [ ] Job detail with status tracker + notes
- [ ] Applied kanban board
- [ ] Saved jobs view

### Phase 5 — Polish
- [ ] Scan error dashboard + manual retry
- [ ] Per-company keyword overrides
- [ ] Weekly email digest option
- [ ] Export jobs to CSV
- [ ] Unit tests for diff engine, scorer, registry
- [ ] Integration tests per adapter
- [ ] README + setup guide

---

## 12. Non-Goals (out of scope for v1)

- Multi-user / auth — single-user local tool only
- Cloud hosting or SaaS
- Resume parsing or auto-apply
- Browser extension
- Mobile app
- Resume tailoring per job (Claude integration deferred to v2)

---

## 13. Success Metrics

- User can add 100 companies and run a full scan in under 2 minutes
- New jobs from Greenhouse/Lever/Ashby companies appear within the scheduled scan window
- Match scoring surfaces at least 1 high-match job per week for an active job seeker
- Adding a new ATS platform takes under 1 hour of development time
- Zero manual daily effort required once configured
