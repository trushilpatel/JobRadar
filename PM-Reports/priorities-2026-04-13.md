# JobRadar PM Priorities — 2026-04-13 (evening refresh, UI + filter polish)

## 1. State of the product

Pipeline runs end-to-end for one user: scheduler → crawler → Gemini filter → analyzer → `job_feed`, with five React pages (Dashboard 312 loc, Jobs 821 loc, Companies 336 loc, Applications 420 loc, Insights 409 loc). SOA refactor merged this morning (`c4c0423`); pagination landed (`197e10c`); ATS coverage spans Greenhouse/Lever/Ashby/Google plus Apple, Meta, MSFT, Netflix, Uber, Rippling. Observability wired. Working tree clean.

Strategy tonight: **harden what exists** — the crawler is not running today, so the analyzer-accuracy eval is parked until tomorrow and the four initiatives below are the focus. Three of the four (Jobs/Dashboard/Companies UI) have zero dependency on a live crawler, so they can be shipped cleanly this evening. Filter simplification is a pure code refactor; validation waits a day. Multi-user, auth, tailoring, notifications — all remain deferred.

## 2. Top 4 prioritized initiatives

### #1 — Simplify the relevance filter
**Why #1:** User-reported complexity. A smaller, more predictable filter is a prerequisite to trusting every downstream view. Also cuts LLM spend. Code-only refactor — safe to land today even without the crawler running; full pipeline validation runs tomorrow against the analyzer eval.

**Success criterion:** `pipeline/filter/worker.go` drops from 189 LOC to ≤100; scoring prompt reduced to one paragraph + a 5-item rubric; every reject carries a single-string reason surfaced in the UI tooltip; `go test ./pipeline/filter/...` stays green.

**Subtasks:**
1. Read `filter/worker.go` end-to-end; inventory every branching decision in `Designs/filter-simplification.md`. ~30 min.
2. Collapse scoring into one template `pipeline/llm/prompts/score.go`; remove any title-stage vs body-stage split. ~60 min.
3. Return `{score, reason}` only — drop multi-dimensional score tuples. Migrate `job_pipeline_ledger.reason` to hold the string. ~45 min.
4. Delete dead branches, helpers, and tests that exist only to cover them. Run `go vet` + the package test. ~30 min.
5. Surface `reason` on the Jobs UI rejection tooltip (read-only). ~30 min.
6. Rewrite `pipeline/filter/CLAUDE.md` against the new contract. ~20 min.

**Effort:** ~3.5h · **Risks:** Behavior drift — keep old prompt in git for one week; re-run analyzer eval tomorrow once crawler is back. Simplifying too aggressively can drop recall; explicitly watch the pass-rate before/after.

---

### #2 — Jobs page UI overhaul
**Why #2:** Jobs is the workhorse screen (821 LOC today — almost certainly bloated). This is where the single user spends most of their time. With the reject-reason from #1 available, the table can finally show "why was this filtered?" on hover. Fully achievable tonight with no crawler.

**Success criterion:** Single-screen table answers "what's new, what's worth my time" in under 5 seconds. Keyboard-driven: `j/k` navigate, `s` save, `a` apply, `d` dismiss. Filters persisted in URL. Detail drawer on click.

**Subtasks:**
1. Inventory `JobsPage.tsx` + `jobs-columns.tsx`; list every existing column and split "keep / drop / redesign." ~30 min.
2. Redesign column set — Company (logo+name), Title, Comp, Location, Age, Score (colored badge), Actions. Update `jobs-columns.tsx`. ~75 min.
3. Slide-over detail drawer — full JD, tech-stack chips, responsibilities bullets, action bar. New `JobDetailDrawer.tsx`. ~90 min.
4. URL-synced filters: query params for status, company, min-score, posted-since. ~60 min.
5. Keyboard shortcuts via one `useHotkeys` hook; `?` shows the legend. ~45 min.
6. Empty, loading, and error states reviewed and consistent. ~30 min.

**Effort:** ~5.5h · **Risks:** Breaking the current mental model — ship behind a `?v=2` query flag for the first day. Chip overflow on narrow widths; define breakpoint behavior.

---

### #3 — Dashboard + Insights clarity pass
**Why #3:** Dashboard (312 LOC) is the first screen on login; today it dumps six stat tiles with no narrative. It should answer three questions in one glance: "Did anything happen overnight?", "What should I act on first?", "Is the pipeline healthy?" Insights (409 LOC) needs one-sentence commentary per chart or the user skims past.

**Success criterion:** Dashboard renders under 300 LOC after extracting reusable `StatTile` and `PipelineHealth` components. Top of the page is a "Today's focus" card listing ≤5 high-score `new` jobs that link into the Jobs drawer. Insights charts each carry a single-sentence `<figcaption>` explaining the trend.

**Subtasks:**
1. Extract `components/StatTile.tsx` and `components/PipelineHealth.tsx`; unit-test the render. ~45 min.
2. Rebuild DashboardPage as: Today's Focus (top), 4-stat row (middle), Pipeline Health (bottom). ~75 min.
3. `GET /dashboard/focus` endpoint in `service/dashboard/` — top 5 `new` jobs ordered by score desc, age asc. ~45 min.
4. Insights: plain-string `<figcaption>` generator driven by the metric delta (no LLM). ~30 min.
5. Visual polish — consistent badge colors, skeleton loaders, spacing-scale audit. ~60 min.

**Effort:** ~4.5h · **Risks:** "Today's focus" feels stale if scans didn't run — always render a prominent last-scan timestamp alongside. Since the crawler is off today, test with seeded data.

---

### #4 — Companies page + scan-health surfacing
**Why #4:** Companies (336 LOC) today lets the user manage tracked orgs but hides scan failures. With 11+ adapters, adapter breakage is the #1 silent failure mode — the user finds out only when a company stops producing jobs. The Companies page is the right place to surface scan health. Perfect fit for a no-crawler evening: all data comes from the existing `company_scan_log` table.

**Success criterion:** Each row shows last-scan status (✓ / ✗ / ⌛), last-scan duration, jobs-found-last-scan, and success rate over the last 7 scans. Broken adapters (≥2 consecutive errors) float to the top with a red banner. Clicking the status opens the latest `company_scan_log` entry.

**Subtasks:**
1. Extend `GET /companies` response with `last_scan`, `success_rate_7d`, `jobs_last_scan`; query joins `company_scan_log`. ~60 min.
2. Update `companies-columns.tsx` — add Health, Last Scan, Jobs Last Scan columns. ~60 min.
3. Row-level drill-in modal showing the last 7 `company_scan_log` rows. ~60 min.
4. Default sort broken-first; red banner at top with count + link to the drill-in. ~30 min.
5. "Rescan now" already exists — add a toast with the enqueued queue position. ~30 min.

**Effort:** ~4h · **Risks:** Scan-log table could slow the page — aggregate server-side with `LIMIT 7` per company. Stale data ok tonight since crawler is off; mark scans older than 48h with a ⌛ badge rather than ✓.

## 3. "Do tomorrow" pick

**Initiative #1, subtasks 1 + 2 — inventory `filter/worker.go` and collapse the scoring prompt into `pipeline/llm/prompts/score.go`.** ~90 focused minutes. Pure code, no crawler needed. Unblocks subtask 3 (ledger reason migration) and subtask 5 (Jobs UI tooltip, which also shows up in initiative #2). If energy remains, jump to initiative #2 subtask 1 (column inventory) — it's pure reading and planning.

## 4. Ideas parked (not now)

- **LLM analyzer accuracy eval (today's old #1)** — revisit tomorrow once the crawler is running again; without live scans the eval has no fresh inputs to grade against.
- **Resume tailoring / cover letter / interview prep** — feature additions on top of an unpolished base; revisit after this polish pass.
- **Multi-user / auth / hosted alpha / waitlist** — explicit strategy: prove single-user first.
- **Notifications / daily digest** — needs #2 (Jobs UI) and #3 (Dashboard focus) first.
- **LLM $-cost instrumentation** — revisit when a new LLM-touchpoint feature ships; not a blocker for today's polish work.

---

_Changelog vs. earlier 2026-04-13 reports:_ Strategy pivoted twice today — first to "single-user feature-complete," then to "harden what exists," and now trimmed further: **analyzer-accuracy eval deferred to tomorrow** (crawler not running today). Top 4 are now **filter simplification**, **Jobs UI**, **Dashboard/Insights clarity**, **Companies scan-health** — in that order so the Jobs UI work (#2) picks up the new reject-reason string from #1. "Do tomorrow" pick shifted to filter inventory + prompt collapse (90 min, pure code, no crawler dependency).
