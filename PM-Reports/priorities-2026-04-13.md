# JobRadar PM Priorities — 2026-04-13 (refresh)

## 1. State of the product

Pipeline works end-to-end: scheduler → crawler → LLM filter → analyzer → `job_feed`, with React UI across Dashboard, Jobs, Companies, Applications, Insights. Prometheus/Loki/Grafana observability shipped (`9b4eaa5`). ATS coverage expanded heavily last week: Google, Netflix, Uber, Apple, Meta, Microsoft, Rippling (`b3504c3`, `d318ddf`). New in flight (uncommitted on `main`): a **service-oriented architecture refactor** — typed request/response envelopes (`backend/api/request`, `backend/api/response`), a `backend/service/` layer with `company` already scaffolded (handler + validator + service + store + wire DI), and a 586-line approved design doc at `docs/superpowers/specs/2026-04-13-service-oriented-architecture-design.md`. Only `company` is done; `job/`, `application/`, `dashboard/`, `insights/`, `scan/` are empty folders. Biggest strategic gap is still: no auth, no hosted URL, no user profile driving the filter, LLM cost unmetered. Zero TODO/FIXME markers in Go code — discipline is good.

## 2. Top 5 prioritized initiatives

### #1 — Finish the SOA refactor (or stop it)
**Why #1:** The `backend/service/` tree is half-built on a dirty worktree. Every other initiative below touches API handlers or stores — merging anything on top of a half-migrated architecture will triple rework. This wasn't on yesterday's list because the refactor hadn't started yet; it now dominates because it blocks all downstream API work (profile endpoint #2, tailor endpoint #5, waitlist handler in #3). Call: finish it this week, don't let it rot.

**Success criterion:** All 6 domains (`company`, `job`, `application`, `dashboard`, `insights`, `scan`) migrated to the 4-layer pattern; old fat handlers deleted; `go test ./...` green; tree commits cleanly.

**Subtasks:**
1. Commit current `company` service + request/response scaffolding as a single logical commit. ~30 min.
2. Port `job` domain (highest endpoint count) using `company` as template — files: `service/job/{handler,validator,service,store,wire}.go`. ~90 min.
3. Port `application` and `dashboard` domains (smaller surface). ~90 min.
4. Port `insights` and `scan`. ~60 min.
5. Delete old handler files in `backend/api/handler/` once parity confirmed; update `cmd/api` wiring. ~30 min.
6. Add one service-layer test per domain mirroring `service/company/service_test.go`. ~60 min.

**Effort:** ~6h
**Risks:** Scope creep — refactor must not add new endpoints. Wire DI churn on every domain addition.

---

### #2 — User Profile-Driven Relevance Filter
**Why #2:** The filter is still hardcoded to one developer's preferences; no outsider gets useful matches. This is the single largest blocker to letting anyone else try JobRadar, and the structural moat vs. Simplify/Teal (keyword-only) and LinkedIn (behavioral only). Also a cost lever: a hard-constraint pre-filter cuts Gemini calls before they happen. Now unblocked to ship onto the new service layer cleanly.

**Success criterion:** User submits resume text + 6 preference fields; `job_listing_relevance_filter` injects the profile into the Gemini prompt; two distinct profiles produce measurably different pass-rates on the same 100-job corpus.

**Subtasks:**
1. Migration `022_user_profiles.sql`: `(id, resume_text, target_roles[], locations[], min_comp, needs_sponsorship, seniority, yoe, updated_at)`.
2. `store/user_profiles.go` Get/Upsert. ~45 min.
3. `service/profile/` scaffold (handler+validator+service+store+wire) following SOA pattern. ~90 min.
4. Extend `pipeline/llm/Scorer` interface to accept `*UserProfile`; update `gemini.go` prompt. ~60 min.
5. Pre-LLM cheap-reject in `pipeline/filter/` on hard constraints (location, sponsorship, comp floor); log reason via `logger/fields.go`. ~60 min.
6. Frontend `ProfilePage.tsx` — resume textarea + prefs form, POST `/profile`. ~90 min.

**Effort:** ~9h
**Risks:** Prompt-shape change may regress scoring — A/B 24h on raw topic before flipping. PDF parsing deferred to v2.

---

### #3 — Deploy a hosted alpha + landing page with waitlist
**Why #3:** Observability is done, Dockerfiles exist, and the SOA refactor gives a stable API surface to expose. Without a URL we cannot validate relevance quality against real users. A live `app.jobradar.app` (basic auth) plus marketing page is the gate to every distribution play (Reddit, IH, PH).

**Success criterion:** `jobradar.app` resolves with value prop + waitlist capture; staging app on `app.jobradar.app` behind basic auth; first 5 waitlist emails captured in a `waitlist` table.

**Subtasks:**
1. Pick hosting: Fly.io (backend+managed Postgres), Vercel (frontend), Upstash Redpanda or single-node Kafka on Fly. Decision doc `Designs/deployment.md`. ~30 min.
2. `fly.toml` for api / crawler / scheduler / analyzer / filter (5 machines). ~90 min.
3. Migrate secrets out of `docker-compose.yml` env blocks into Fly secrets; verify local compose still boots. ~60 min.
4. Single-page `LandingPage.tsx` — hero, 3 screenshots, email capture → `POST /waitlist`. ~60 min.
5. `waitlist` table + handler via new SOA pattern. ~30 min.
6. Buy domain + TLS + basic-auth middleware on app subdomain. ~60 min.

**Effort:** ~7h
**Risks:** Kafka hosting cost; cap at $50/mo — drop to single-node Redpanda if Upstash bites. Fly free-tier cold-start may confuse testers.

---

### #4 — LLM cost instrumentation + batch tuning
**Why #4:** ~3k LLM calls/day/user projected; zero visibility into $/job today. We need a meter and a circuit breaker before any waitlist user joins, otherwise we can't answer "what's our margin?" or prevent a runaway bill. Preempts the most common launch-day failure mode for LLM products.

**Success criterion:** Grafana panel shows tokens + USD/day per service; `FILTER_BATCH_SIZE` env var with a documented precision/recall curve across {5,10,20} on a 50-job hand-labeled corpus; `LLM_DAILY_BUDGET_USD` circuit breaker pauses filter when exceeded.

**Subtasks:**
1. Scorer response adds `InputTokens`, `OutputTokens`, `EstUSD`; Prometheus counter `llm_tokens_total{service,model}`. ~60 min.
2. Grafana JSON panel committed in `deploy/grafana/`. ~30 min.
3. Hand-label 50 jobs (yes/no/maybe) in a CSV; run filter at batch=5,10,20; record precision/recall. ~90 min.
4. Budget breaker daemon reads `sum(llm_cost_usd)`, flips a flag read by filter worker. ~60 min.
5. Notes → `Designs/llm-cost-notes.md`. ~15 min.

**Effort:** ~4.5h
**Risks:** Gemini pricing moves — lock model+pricing table in a const file.

---

### #5 — Resume-tailoring MVP (the demo wedge)
**Why #5:** Every competitor stops at "job found." Tailoring bullets + missing keywords + cover letter draft is the feature users screenshot. Stays at #5 because it depends on #2 (profile) and needs #3 (users) to matter. Also the single best thing to film for a ProductHunt launch video.

**Success criterion:** "Tailor" button on a job opens a panel with (a) 5 rewritten bullets, (b) top 5 missing keywords, (c) cover letter draft. Copy-to-clipboard works. Result cached keyed by `(job_id, profile_hash)`.

**Subtasks:**
1. `pipeline/llm/prompts/tailor.go` with strict JSON schema. ~45 min.
2. `service/tailor/` scaffold — `POST /jobs/:id/tailor`. ~60 min.
3. `job_tailoring` table + migration; cache-first read path. ~45 min.
4. Slide-over panel component wired to endpoint. ~90 min.
5. Rate limit: 20/day/user — token bucket in memory. ~30 min.

**Effort:** ~5.5h
**Risks:** Bullet quality varies — frame as "draft" in the UI. Prompt-injection via JD content; sanitize before inclusion.

## 3. "Do tomorrow" pick

**Initiative #1, subtask 1 — commit the `company` SOA scaffold as a clean logical commit.** The tree is dirty and every other priority requires building on top of the new pattern. Thirty minutes of work that unblocks the next 40 hours; subtask 2 (port `job`) becomes the obvious next pull.

## 4. Ideas parked (not now)

- **Chrome extension for one-click apply** — needs real-user validation first; scope >> value today.
- **Mobile app** — web works on phones; no demand signal.
- **AI interview prep chatbot** — ChatGPT does this for free; weak wedge.
- **LinkedIn/Indeed direct scraping** — ToS and anti-bot costs >> value while ATS coverage is the moat.
- **Kafka → NATS/SQS rewrite** — flagged in `architecture-review-scalability.md` but premature at single-user scale.

---

_Changelog vs. earlier 2026-04-13 report:_ SOA refactor (discovered uncommitted on `main`) jumped to **#1**, displacing "ship the 7 staged ATS adapters" entirely — those adapters are already committed (`b3504c3`, `d318ddf`), so yesterday's #2 is closed. User-profile filter moved 1 → 2; hosting, LLM cost, and tailoring held at 3/4/5. "Do tomorrow" shifted from committing adapters to committing the SOA scaffold.
