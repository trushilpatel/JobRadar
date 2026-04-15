# JobRadar PM Priorities — 2026-04-14

## 1. State of the product

End-to-end pipeline still green for the single user: scheduler → crawler → Gemini filter → analyzer → `job_feed`, feeding five React pages (Dashboard 312, Jobs 821, Companies 336, Applications 420, Insights 409 LOC). ATS coverage spans Greenhouse/Lever/Ashby/Google plus Apple, Meta, MSFT, Netflix, Uber, Rippling. Observability wired (Prometheus + Loki + Grafana).

**Critical signal: no new commits since `197e10c` (2026-04-13 AM).** Yesterday's plan (filter simplification → Jobs UI → Dashboard → Companies health) did not ship. Working tree has only the prior PM report edit and a minor `ScanStatsDrawer.tsx` tweak. The most obvious gap is therefore **execution momentum**, not missing features — every initiative below is carried over, re-sequenced to front-load the smallest shippable unit that unblocks the others.

## 2. Top 5 prioritized initiatives

### #1 — Ship filter simplification (carried from 2026-04-13)
**Why #1:** Highest-leverage item still open. Shrinks `pipeline/filter/worker.go` (189 LOC), cuts LLM spend per scored job, and surfaces a single `reason` string that Jobs UI (#2) and Companies health (#4) both consume. Pure code refactor, no crawler dependency, finishable in one focused session.

**Success criterion:** `backend/pipeline/filter/worker.go` ≤100 LOC; prompt is one paragraph + 5-item rubric in `pipeline/llm/prompts/score.go`; filter returns `{score:int, reason:string}`; `job_pipeline_ledger.reason` stores the string; `go test ./pipeline/filter/...` green; `go vet ./...` clean.

**Subtasks:**
1. Read `backend/pipeline/filter/worker.go` + tests end-to-end; write a one-page decision inventory to `Designs/filter-simplification.md`. ~30 min.
2. Create `backend/pipeline/llm/prompts/score.go` with single template; delete any title-stage vs body-stage split. ~60 min.
3. Return `{score, reason}` only; migration `migrations/NNN_ledger_reason_string.sql` for ledger column type/semantic. ~45 min.
4. Delete dead branches + their tests; run `go vet` and package tests. ~30 min.
5. Surface `reason` in `frontend/src/pages/jobs-columns.tsx` as a tooltip on filtered rows. ~30 min.
6. Rewrite `backend/pipeline/filter/CLAUDE.md` against the new contract. ~20 min.

**Effort:** ~3.5h · **Risks:** Recall drop — log old vs new pass-rate side-by-side for 48h before deleting the old prompt. Ledger migration must be backward-compatible if ledger rows are in flight.

---

### #2 — Jobs page UI overhaul
**Why #2:** Jobs is the daily workhorse and the single user's primary surface. At 821 LOC it's the biggest LOC risk in the codebase. With #1's `reason` available, we can finally show *why* a job was filtered — a differentiator vs. every opaque job board. No crawler dependency.

**Success criterion:** "What's new, what's worth my time" answered in <5s. Keyboard-driven (`j/k/s/a/d`), URL-synced filters, slide-over detail drawer, columns trimmed to Company / Title / Comp / Location / Age / Score / Actions.

**Subtasks:**
1. Inventory `JobsPage.tsx` + `jobs-columns.tsx`; split columns into keep/drop/redesign in a gist comment. ~30 min.
2. Rewrite `jobs-columns.tsx` with the new column set + colored score badge. ~75 min.
3. New `components/jobs/JobDetailDrawer.tsx` — JD, tech chips, responsibilities, action bar. ~90 min.
4. URL-synced filters (status/company/min-score/posted-since) via `useSearchParams`. ~60 min.
5. `useHotkeys` hook + `?` legend modal. ~45 min.
6. Audit empty / loading / error states for consistency. ~30 min.

**Effort:** ~5.5h · **Risks:** Mental-model break — ship behind `?v=2`. Narrow-width chip overflow — define 3 breakpoints.

---

### #3 — Companies page scan-health surfacing
**Why #3:** With 11+ ATS adapters, silent adapter breakage is the #1 failure mode before launch. `company_scan_log` already has the data — just not exposed. Cheap win that buys observability for the single user *and* becomes a selling point ("we tell you when a company stops posting vs. our adapter broke"). Elevated above Dashboard because broken adapters invalidate every downstream metric.

**Success criterion:** Each row shows last-scan ✓/✗/⌛, duration, jobs-found-last-scan, 7-scan success rate. Rows with ≥2 consecutive errors float up under a red banner. Click → drawer with last 7 `company_scan_log` rows.

**Subtasks:**
1. Extend `GET /companies` in `backend/api/` to join `company_scan_log` with `LIMIT 7` subquery per company. ~60 min.
2. Update `frontend/src/pages/companies-columns.tsx` — Health, Last Scan, Jobs Last Scan cols. ~60 min.
3. Reuse/extend existing `ScanStatsDrawer.tsx` (already in flight in working tree) for the 7-row drill-in. ~45 min.
4. Default sort broken-first; red banner + count at top of page. ~30 min.
5. Toast on "Rescan now" showing queue position from `company_scan_queue`. ~20 min.

**Effort:** ~3.5h · **Risks:** N+1 query if join is naive — use a single CTE. Confusing ⌛ when crawler itself is paused — key off `scheduler_last_tick`, not just per-company age.

---

### #4 — Dashboard "Today's focus" + Insights captions
**Why #4:** Dashboard is the first screen on login but today it dumps six stat tiles with no narrative. One card that says "here are 5 jobs to look at right now" converts Dashboard from a status page to an action page — the single most visible quality jump pre-launch. Ranked below #3 because its payoff depends on adapters actually producing jobs (i.e., #3 shipping first).

**Success criterion:** DashboardPage ≤300 LOC after extracting `StatTile` + `PipelineHealth`. Top card lists ≤5 high-score `new` jobs (linking into the Jobs drawer). Every Insights chart carries a one-sentence `<figcaption>` driven by the metric delta (no LLM).

**Subtasks:**
1. Extract `components/StatTile.tsx` + `components/PipelineHealth.tsx` with render tests. ~45 min.
2. Rebuild `DashboardPage.tsx` layout: focus card → stat row → pipeline health. ~60 min.
3. `GET /dashboard/focus` in `backend/service/dashboard/` — top 5 `new` jobs, score desc, age asc. ~45 min.
4. Insights: deterministic caption generator keyed off delta sign/magnitude. ~30 min.
5. Visual polish — badge color scale, skeleton loaders, spacing audit. ~45 min.

**Effort:** ~3.5h · **Risks:** Stale "focus" if scans didn't run — always render last-scan timestamp. Deterministic captions can read robotic — cap at 3 templates per metric.

---

### #5 — Pre-launch landing page + waitlist
**Why #5:** New this report. The previous 4 plans all assumed "single-user polish first, launch later" — but we've now spent multiple days on polish with no commits. A static landing page + waitlist is a **30-minute cost that de-risks the entire launch plan**: it starts collecting interest, forces a one-sentence positioning statement, and gives a real funnel to measure. Explicitly a non-code item except for a single static page. Ranks last because it's strategically important but small.

**Success criterion:** Live URL with positioning headline, 3 feature bullets, "why not LinkedIn/Simplify/Huntr" one-liner, email capture wired to a real list (Buttondown / Resend / Google Form — pick the one that takes 10 min to set up). First 3 signups from personal network within 48h.

**Subtasks:**
1. Write positioning: 1 headline, 3 sub-points, 1 differentiation line. Commit as `Designs/positioning.md`. ~30 min.
2. Pick capture tool — decision in `Designs/positioning.md` (recommend Buttondown free tier). ~15 min.
3. Single-page HTML (or Next.js static) in `marketing/`; deploy to Vercel/Netlify. ~75 min.
4. DM 5 friends + 3 r/cscareerquestions regulars for waitlist signups. ~30 min.
5. Add `utm_source` tracking stub so we can measure where signups come from. ~20 min.

**Effort:** ~2.5h · **Risks:** Positioning drift — don't over-invest in copy until we have #2 (Jobs UI) screenshots. Treat the page as disposable.

## 3. "Do tomorrow" pick

**Initiative #1, subtasks 1 + 2 — inventory `backend/pipeline/filter/worker.go` and collapse the scoring prompt into `backend/pipeline/llm/prompts/score.go`.** Same pick as yesterday because yesterday didn't happen — but *only the first 90 minutes*. Ship a commit before lunch. Momentum matters more right now than scope; a merged PR resets the "no commits in 2 days" signal that otherwise dominates this report.

## 4. Ideas parked (not now)

- **Resume tailoring / cover letter / interview prep** — end-to-end coverage is the long-term moat, but polish base first; revisit post-launch.
- **Multi-user / auth / hosted alpha** — single-user proof first; auth is a week of distraction we can't afford pre-positioning.
- **LLM $-cost instrumentation dashboard** — re-open when #1 lands; the new filter contract is the right place to add per-call cost logging.
- **Notifications / daily digest** — needs #2 and #4 shipped as prerequisites; the digest *is* a rendered "Today's focus."
- **Analyzer accuracy eval harness** — parked again; without #1's cleaner filter output, the eval grades a moving target.

---

_Changelog vs. 2026-04-13:_ No commits shipped yesterday, so #1 (filter simplification), #2 (Jobs UI), #3 (Companies health), #4 (Dashboard/Insights) all carry over. **Companies health promoted from #4 → #3** (above Dashboard) because broken-adapter detection unblocks every metric downstream. **New #5 — landing page + waitlist** added as a small strategic hedge against continued polish-only days; explicitly called out as a non-code item.
