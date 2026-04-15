# Filter Simplification — Design Spec

**Owner:** Trushil  ·  **Status:** ready to implement  ·  **Scope:** `backend/pipeline/filter/`, `backend/pipeline/llm/`, `backend/model/`, `backend/migrations/`, `backend/store/seen_ledger.go` (reason column), Jobs UI tooltip.

---

## 1. Why we're changing it

Today's `pipeline/filter/worker.go` (189 LOC) is fine structurally. The pain is elsewhere:

1. **Opaque rejections.** The Gemini call returns only a bare `[]string` of relevant IDs. Every irrelevant job collapses into the single ledger value `llm_irrelevant`. The user has no idea *why* a job was rejected and cannot trust the filter.
2. **Prompt carries logic that belongs in code.** The 25-line rubric mixes (a) deterministic rules (title = "Director" → exclude, location not in USA → exclude) with (b) ambiguous judgement ("is this a plausible match for a backend SWE"). Deterministic rules should be deterministic Go, not LLM vibes. Moving them out cuts tokens, cuts latency, and removes a class of false-negative misfires.
3. **`ScoreBatch` doesn't see location.** The prompt tells Gemini to evaluate location, but `ScorerInput` carries only `{ID, Title}`. Location is in `RawJob.Location`. The model either hallucinates or ignores the location rule. This is a live bug.
4. **No per-job score.** Everything is a binary keep/drop. With a numeric score we can sort the Jobs page, set a min-score threshold, and build a "Today's focus" tile. Free with one prompt-shape tweak.
5. **No room to evolve.** When we later add resume tailoring / daily digest, we need a "fit score" and a reason. Building those on top of the current binary signal means rewriting the filter anyway. Do it once.

### Non-goals

- User profiles / multi-user. Keep everything driven by `config.TargetRoles` and `config.YearsExp` for now.
- LLM cost metering. Separate initiative.
- Replacing Gemini. Model stays.
- Changing the Kafka topology. Same topics, same partitioning.

### Success criteria

| # | Target | How we measure |
|---|---|---|
| 1 | `pipeline/filter/worker.go` ≤ 100 LOC (from 189) | `wc -l` |
| 2 | Every non-relevant job carries a single-string reason | query `job_pipeline_ledger.filter_reason` distinct values; ≥ 5 non-null reasons |
| 3 | Pre-LLM rejection cuts Gemini batches by ≥ 30% | Prometheus `filter_prefilter_rejected_total` / `filter_jobs_total` |
| 4 | Jobs table shows a "why rejected" tooltip on dismissed rows | UI manual check |
| 5 | `go test ./backend/pipeline/filter/...` green | CI |
| 6 | No regression in relevant-pass-rate after roll-forward | tomorrow's analyzer eval on 30 labelled jobs |

---

## 2. Design decisions

### 2.1 Three-stage pipeline inside `flushBatch`

```
for each incoming RawJob:
    ┌──────────────────────┐
    │ Stage A: pre-filter  │   pure Go, deterministic
    │ (title + location)   │   → reject with reason string
    └──────────────────────┘
              │ survivors
              ▼
    ┌──────────────────────┐
    │ Stage B: LLM score   │   one Gemini call per batch
    │ (returns score 0-100 │   returns {id, score, reason}
    │  + short reason)     │
    └──────────────────────┘
              │ score ≥ threshold
              ▼
    ┌──────────────────────┐
    │ Stage C: publish to  │   same topic as today
    │ job_listings.filtered│
    └──────────────────────┘
```

Stage A is new. Stage B is a prompt + shape change. Stage C is unchanged.

### 2.2 Pre-filter: deterministic reject rules

Anything that's a hard "no" with a human-readable rule should never touch Gemini. Two buckets:

**Title reject (case-insensitive substring or word-boundary regex):**

| Bucket | Patterns | Reason string |
|---|---|---|
| Management / non-IC | `\b(director|vp|vice president|head of|chief|cto|cio|ceo)\b`, `manager` (unless followed by "of engineering" … see note) | `title:management` |
| Too senior (for now) | `\b(staff|principal|distinguished|fellow|architect)\b` when standalone | `title:too_senior` |
| Too junior / non-FTE | `\b(intern|co-op|coop|apprentice|contract|contractor|part-time)\b` | `title:non_fte` |
| Wrong function | `\b(sales|marketing|recruiter|recruiting|legal|finance|designer|product manager|program manager|technical writer|support|customer success)\b` | `title:wrong_function` |
| Non-eng systems | `\b(systems administrator|sysadmin|it support|help desk|technician|qa tester|test engineer)\b` | `title:non_eng` |

Exception handling:
- `manager` alone is rejected, but `engineering manager of X` is allowed through (hybrid IC/manager roles sometimes land here; cheap to let LLM judge).
- Seniority words as adjectives to an IC role are **not** rejected at this stage: "Staff Software Engineer" passes through because `staff` applies to a SWE title — let the LLM decide. Rule: reject only when the banned word is a direct noun (e.g. `Staff Engineer` standalone, but `Staff Software Engineer` is fine). A tiny regex split handles this.

**Location reject:**

| Rule | Reason string |
|---|---|
| Location contains a non-US country name from a short blocklist (`India`, `Bengaluru`, `Bangalore`, `Hyderabad`, `Pune`, `Chennai`, `Mumbai`, `Delhi`, `UK`, `United Kingdom`, `London`, `Ireland`, `Dublin`, `Germany`, `Berlin`, `Munich`, `France`, `Paris`, `Japan`, `Tokyo`, `Singapore`, `Australia`, `Sydney`, `Canada`, `Toronto`, `Vancouver`, `Poland`, `Warsaw`, `Portugal`, `Lisbon`, `Brazil`, `Mexico`, `Israel`, `Tel Aviv`, `UAE`, `Dubai`) and no US hint | `location:non_us` |
| Blank or "Remote" | pass through (LLM gets to decide if company is US-based) |
| Contains US state abbrev / city / "USA" / "United States" | pass through |

This is a heuristic blocklist, not an allowlist — false positives here are cheap (we rarely want a Bangalore posting anyway, and if the user does, edit the blocklist). Start with ~30 entries, grow from real data.

### 2.3 LLM scorer: from `[]ID` to `[]ScoreResult`

**New `Scorer.ScoreBatch` shape:**

```go
type ScoreResult struct {
    ID     string  `json:"id"`
    Score  int     `json:"score"`   // 0-100
    Reason string  `json:"reason"`  // ≤ 40 chars, lowercased, snake_case-ish
}

type Scorer interface {
    ScoreBatch(ctx context.Context, jobs []ScorerInput) ([]ScoreResult, error)
    Enrich(ctx context.Context, input EnrichInput) (*EnrichResult, error)
}
```

**Threshold** lives in the worker, not the prompt: `FILTER_MIN_SCORE` env (default `60`). Anything below is treated as irrelevant with the reason the model returned.

**Why a score (0-100) instead of a binary?**

- Lets the Jobs UI sort by fit.
- Lets a later "Today's focus" tile pick top-N by score.
- Gives headroom to tune without re-prompting: raise threshold to 70 if noisy, drop to 50 if starved.

**Why a reason from the LLM?** Even after the deterministic pre-filter, borderline cases ("Senior SWE, hybrid NYC, 8+ yrs in Rust") get rejected for non-obvious reasons. A one-phrase reason is cheap and salvages trust.

**Widen `ScorerInput`:**

```go
type ScorerInput struct {
    ID       string
    Title    string
    Location string   // NEW — fixes the latent bug
    Company  string   // NEW — helps the model disambiguate
}
```

### 2.4 Prompt: one paragraph, strict schema

Current prompt is 25 lines + rubric. Replace with this (final copy, ready to paste into `pipeline/llm/prompts/score.go`):

```
Return ONLY a JSON array. No markdown. No preamble.

You are rating software engineering jobs for one candidate:
• Target roles: {{roles}}
• Experience: {{yoe}}+ years
• Location: United States (remote OK if US-based)

For each job below, return an object {id, score, reason}.

score is 0-100:
  90-100  bullseye IC SWE role, titles like Senior/SWE II-III, US/remote, matches target.
  60-89   plausible match with minor mismatches (adjacent stack, unclear seniority, hybrid).
  30-59   stretch — wrong level or unclear fit, but not disqualifying.
  0-29    poor fit — we will discard below threshold.

reason is ≤ 6 words, lowercase, snake_case-ish. Examples:
  "strong_backend_match"
  "adjacent_stack_but_ok"
  "seniority_unclear"
  "non_us_location"
  "wrong_function"

When in doubt, score 60. Prefer false positives over false negatives.

Jobs:
{{numbered_list  "id | title | location | company"}}
```

**What went away from the old prompt:**

| Removed | Where it moved |
|---|---|
| "EXCLUDE: Director / VP / …" | Stage A title regex |
| "EXCLUDE: Intern / Co-op / …" | Stage A title regex |
| "EXCLUDE: non-engineering functions" | Stage A title regex |
| "EXCLUDE: non-US locations" | Stage A location blocklist |
| Long include lists | Implied by the role + score rubric |
| Two-paragraph seniority preamble | "score rubric" + "when in doubt, 60" |

This is the "one paragraph + 5-item rubric" target from the initiative — length drops from ~30 lines to ~18, but more importantly the *work* the model is doing collapses from "apply 4 pages of rules" to "score 0-100 with a short reason."

### 2.5 Gemini response schema

Swap `scoreBatchSchema` to an array of objects:

```go
var scoreBatchSchema = &genai.Schema{
    Type: genai.TypeArray,
    Items: &genai.Schema{
        Type: genai.TypeObject,
        Properties: map[string]*genai.Schema{
            "id":     {Type: genai.TypeString},
            "score":  {Type: genai.TypeInteger},
            "reason": {Type: genai.TypeString},
        },
        Required: []string{"id", "score", "reason"},
    },
}
```

MaxOutputTokens stays 4096 — an object is ~30 tokens × 50 jobs = 1500, with headroom.

### 2.6 Ledger schema change

We don't want to conflate a category enum (`fetch`, `llm_irrelevant`, `error`, `user_dismissed`) with a free-form human reason.

**Migration `022_filter_reason.sql`:**

```sql
ALTER TABLE job_pipeline_ledger
    ADD COLUMN filter_reason TEXT NULL;

CREATE INDEX idx_job_pipeline_ledger_filter_reason
    ON job_pipeline_ledger(filter_reason)
    WHERE filter_reason IS NOT NULL;

ALTER TABLE job_pipeline_ledger
    ADD COLUMN filter_score SMALLINT NULL
        CHECK (filter_score IS NULL OR (filter_score >= 0 AND filter_score <= 100));
```

Existing `reason` column keeps holding enum values (`fetch`, `llm_irrelevant`, `error`, `user_dismissed`). New columns carry the detail.

### 2.7 `store/seen_ledger.go` — two new methods

```go
// Upsert the reason detail + score alongside the existing enum reason.
UpdateFilterOutcomeBatch(outcomes []FilterOutcome) error

type FilterOutcome struct {
    JobID        string
    Reason       string  // enum: llm_irrelevant | error | prefilter_title | prefilter_location
    FilterReason string  // free-form short string, surfaced in UI
    FilterScore  *int    // nil for prefilter rejects, 0-100 for LLM rejects
}
```

Single batch upsert keeps the worker's error recovery simple.

### 2.8 Filter config

One place, not three. New file `backend/pipeline/filter/config.go`:

```go
type Config struct {
    MinScore           int           // default 60
    BatchSize          int
    FlushTimeout       time.Duration
    TargetRoles        []string      // injected into prompt
    YearsExp           int
    TitleRejectRegex   *regexp.Regexp // compiled from default patterns
    LocationBlocklist  []string      // lowercase substrings
}
```

Loaded once from env + the hardcoded defaults in `prefilter.go`. Passed into `NewWorker`. No more prompt knobs in env.

### 2.9 Observability

New Prometheus counters in `backend/infra/metrics/`:

```
filter_prefilter_rejected_total{rule="title_management|title_too_senior|title_non_fte|title_wrong_function|title_non_eng|location_non_us"}
filter_llm_rejected_total{reason_bucket="low_score|wrong_function|seniority|location|other"}
filter_score_histogram (0-100 buckets of 10)
```

`reason_bucket` is a small switch on the LLM's free-form reason — lets us graph buckets without a high-cardinality label. Full free-form stays only in Postgres.

Log lines already use `logger/fields.go` — add two field constants:

```go
FieldFilterScore  = "filter_score"
FieldFilterReason = "filter_reason"
```

### 2.10 Jobs UI tooltip (frontend change)

- `GET /jobs` already returns ledger rows; add `filter_reason`, `filter_score` to the response in `service/job/types.go`.
- `jobs-columns.tsx`: when `status == 'dismissed'` or when viewed on a "Rejected" tab, render a 🛈 icon; tooltip shows `filter_reason` + score. Use a native `<abbr title>` first, fancy component later. ~30 min.

---

## 3. File-by-file plan

| File | Action | LOC delta | Time |
|---|---|---:|---:|
| `backend/pipeline/filter/config.go` | **NEW** — `Config` struct, defaults | +60 | 20 min |
| `backend/pipeline/filter/prefilter.go` | **NEW** — compiled regex, location blocklist, `Classify(RawJob) (keep bool, reason string)` + unit tests | +120 src + 150 test | 45 min |
| `backend/pipeline/filter/worker.go` | trim — remove repeated timer dance by extracting `drain(ctx)` helper; integrate Stage A; adopt new `ScoreResult` | ~190 → ~95 | 40 min |
| `backend/pipeline/filter/worker_test.go` | add cases: prefilter-only reject, mixed batch, score below threshold | +80 test LOC | 30 min |
| `backend/pipeline/llm/llm.go` | `ScorerInput` gets `Location`, `Company`. New `ScoreResult` type. Interface signature change. | +15 | 10 min |
| `backend/pipeline/llm/gemini.go` | new schema, new prompt, parsing to `[]ScoreResult`. Keep `Enrich` untouched. | -20 | 35 min |
| `backend/pipeline/llm/prompts/score.go` | **NEW** — template + test that renders deterministic output for known input | +60 src + 40 test | 20 min |
| `backend/migrations/022_filter_reason.sql` | **NEW** | +15 | 10 min |
| `backend/store/seen_ledger.go` | add `UpdateFilterOutcomeBatch` + tests | +60 src + 50 test | 30 min |
| `backend/service/job/store.go` | expose `filter_reason`, `filter_score` on Jobs read | +20 | 15 min |
| `backend/service/job/types.go` | DTO fields | +5 | 5 min |
| `backend/infra/logger/fields.go` | two constants | +2 | 2 min |
| `backend/infra/metrics/metrics.go` | 3 new counters | +15 | 10 min |
| `backend/pipeline/filter/CLAUDE.md` | **NEW** — written after, describes the 3-stage contract | +60 | 15 min |
| `frontend/src/pages/jobs-columns.tsx` | tooltip render on rejected rows | +20 | 20 min |
| `frontend/src/types/job.ts` | add fields | +4 | 5 min |

**Total:** ≈ 6 hours coding + testing. Plus ~30 min smoke tomorrow when the crawler runs.

---

## 4. Concrete Go snippets (drop-in)

### 4.1 `prefilter.go` skeleton

```go
package filter

import (
    "regexp"
    "strings"

    "github.com/trushilpatel/jobradar/pipeline/ats"
)

var (
    // Matches when the banned word is NOT directly followed by "Engineer" /
    // "Developer" / "SWE". "Staff Software Engineer" passes; "Staff Engineer" rejects.
    reTooSenior = regexp.MustCompile(
        `(?i)\b(staff|principal|distinguished|fellow|architect)\b(?!\s+(software|backend|frontend|fullstack|platform|ml|data)\s+(engineer|developer|swe))`,
    )
    reMgmt        = regexp.MustCompile(`(?i)\b(director|vp|vice\s+president|head\s+of|chief|cto|cio|ceo)\b|^\s*(engineering\s+)?manager\b`)
    reNonFTE      = regexp.MustCompile(`(?i)\b(intern|internship|co[-\s]?op|apprentice|contractor?|part[-\s]?time)\b`)
    reWrongFunc   = regexp.MustCompile(`(?i)\b(sales|marketing|recruit(er|ing)|legal|finance|accountant|ux\s+designer|graphic\s+designer|product\s+manager|program\s+manager|technical\s+writer|content|support|customer\s+success)\b`)
    reNonEng      = regexp.MustCompile(`(?i)\b(systems?\s+administrator|sysadmin|it\s+support|help\s+desk|technician|qa\s+tester)\b`)

    nonUSLocations = []string{
        "india", "bengaluru", "bangalore", "hyderabad", "pune", "chennai",
        "mumbai", "new delhi", "uk", "united kingdom", "london", "ireland",
        "dublin", "germany", "berlin", "munich", "france", "paris", "japan",
        "tokyo", "singapore", "australia", "sydney", "canada", "toronto",
        "vancouver", "poland", "warsaw", "portugal", "lisbon", "brazil",
        "mexico", "israel", "tel aviv", "uae", "dubai", "netherlands",
        "amsterdam", "spain", "madrid", "barcelona", "czech", "prague",
    }
    usHints = []string{
        "usa", "united states", "u.s.", " us ", "us)", "(us", "remote - us",
        "remote (us", "new york", "san francisco", "seattle", "boston", "chicago",
        "austin", "atlanta", "denver", "los angeles", "washington",
    }
)

// Classify runs the deterministic pre-filter. Returns keep=false with a short
// reason string on reject; keep=true otherwise.
func Classify(job ats.RawJob) (keep bool, reason string) {
    title := strings.ToLower(job.Title)
    switch {
    case reMgmt.MatchString(title):
        return false, "title:management"
    case reTooSenior.MatchString(title):
        return false, "title:too_senior"
    case reNonFTE.MatchString(title):
        return false, "title:non_fte"
    case reWrongFunc.MatchString(title):
        return false, "title:wrong_function"
    case reNonEng.MatchString(title):
        return false, "title:non_eng"
    }

    loc := strings.ToLower(job.Location)
    if loc == "" {
        return true, ""
    }
    for _, hint := range usHints {
        if strings.Contains(loc, hint) {
            return true, ""
        }
    }
    for _, country := range nonUSLocations {
        if strings.Contains(loc, country) {
            return false, "location:non_us"
        }
    }
    return true, ""
}
```

### 4.2 `worker.flushBatch` after the refactor

```go
func (w *Worker) flushBatch(ctx context.Context, batch []ats.RawJob) {
    if len(batch) == 0 {
        return
    }

    // Stage A — deterministic pre-filter.
    var survivors []ats.RawJob
    var prefilterOutcomes []store.FilterOutcome
    for _, j := range batch {
        if keep, reason := Classify(j); !keep {
            metrics.FilterPrefilterRejected.WithLabelValues(reason).Inc()
            prefilterOutcomes = append(prefilterOutcomes, store.FilterOutcome{
                JobID:        j.ID,
                Reason:       model.SeenReasonLLMIrrelevant, // enum stays stable
                FilterReason: reason,
                FilterScore:  nil,
            })
            continue
        }
        survivors = append(survivors, j)
    }
    if len(prefilterOutcomes) > 0 {
        _ = w.ledger.UpdateFilterOutcomeBatch(prefilterOutcomes)
    }

    if len(survivors) == 0 {
        return
    }

    // Stage B — LLM score.
    inputs := make([]llm.ScorerInput, len(survivors))
    for i, j := range survivors {
        inputs[i] = llm.ScorerInput{ID: j.ID, Title: j.Title, Location: j.Location, Company: j.CompanyName}
    }
    start := time.Now()
    results, err := w.scorer.ScoreBatch(ctx, inputs)
    metrics.FilterLLMDuration.Observe(time.Since(start).Seconds())
    if err != nil {
        w.markBatchError(survivors, err)
        return
    }

    byID := make(map[string]llm.ScoreResult, len(results))
    for _, r := range results {
        byID[r.ID] = r
    }

    var llmOutcomes []store.FilterOutcome
    for _, j := range survivors {
        r, ok := byID[j.ID]
        if !ok {
            llmOutcomes = append(llmOutcomes, store.FilterOutcome{JobID: j.ID, Reason: model.SeenReasonError, FilterReason: "missing_in_llm_response"})
            continue
        }
        metrics.FilterScoreHist.Observe(float64(r.Score))
        if r.Score >= w.cfg.MinScore {
            w.publishDetail(ctx, j)
            continue
        }
        score := r.Score
        llmOutcomes = append(llmOutcomes, store.FilterOutcome{
            JobID:        j.ID,
            Reason:       model.SeenReasonLLMIrrelevant,
            FilterReason: r.Reason,
            FilterScore:  &score,
        })
        metrics.FilterLLMRejected.WithLabelValues(bucketize(r.Reason)).Inc()
    }
    if len(llmOutcomes) > 0 {
        _ = w.ledger.UpdateFilterOutcomeBatch(llmOutcomes)
    }
}
```

Extracting `markBatchError` and the timer dance into helpers drops the main `Run` loop below 60 LOC.

### 4.3 `bucketize(reason string) string`

```go
// bucketize collapses free-form reasons into a low-cardinality Prom label.
func bucketize(r string) string {
    r = strings.ToLower(r)
    switch {
    case strings.Contains(r, "location"):
        return "location"
    case strings.Contains(r, "senior") || strings.Contains(r, "level"):
        return "seniority"
    case strings.Contains(r, "function") || strings.Contains(r, "role"):
        return "wrong_function"
    case strings.Contains(r, "score") || strings.Contains(r, "low"):
        return "low_score"
    default:
        return "other"
    }
}
```

---

## 5. Rollout & rollback

1. Land everything on `main` behind a single env flag `FILTER_V2=true`. When false, old path runs. Default true in dev, false in whatever "prod" we run locally until the first clean 24h.
2. Leave old `worker.go` / `gemini.go` ScoreBatch variants as `scoreBatchV1` / `workerV1` for one week. Delete after the analyzer eval tomorrow confirms no recall regression.
3. Rollback is a one-env flip. Ledger schema change is additive — no rollback needed on the DB.

---

## 6. Open questions (intentionally deferred)

- **Weighting companies by past conversion.** Belongs in a later "applications-outcome insights" initiative; filter stays stateless for now.
- **Multi-role targeting (e.g. backend + ML).** Current single `TargetRoles` list stays; prompt handles it via "Target roles: a, b, c". Revisit when we have a profile.
- **Title-only fast path vs full-JD.** We still score on title + location + company only. Full JD is the analyzer's job downstream. If relevant-pass-rate is acceptable, don't touch this.

---

## 7. Timeline

| Session | Work |
|---|---|
| Tonight, 90 min | `prefilter.go` + tests; `Config` struct; new `ScorerInput`/`ScoreResult` types; migration file. No pipeline execution needed. |
| Tonight, 60 min | Rewrite `gemini.go ScoreBatch` + new prompt file + its parse test. |
| Tonight, 60 min | Refactor `worker.go`; port existing tests to new shape; add prefilter test cases. |
| Tomorrow, 30 min | Smoke against live crawler; tune blocklist from whatever leaks through. |
| Tomorrow, 45 min | Jobs UI tooltip + types. |
| Tomorrow, 30 min | Write `pipeline/filter/CLAUDE.md`. |

**Commit strategy:** one PR = one stage. (1) schema + types + prefilter, (2) gemini + prompt, (3) worker rewrite + tests, (4) UI tooltip. Each green on its own CI run.
