# Resume Feature — Finalized Requirements

**Status:** Phase 1 spec frozen, ready for Sprint 0 / M1 build.
**Authored:** 2026-05-23 via 3-agent (UX × Eng × PM) × 5-round design loop.
**Owner:** Trushil Patel.

This document is the source of truth for the resume feature. Frontend mockup already lives at `ui/console/src/features/resumes/` and `ui/console/src/pages/admin/Resume*Page.tsx` — wired with stub data only. Backend slice not yet built.

---

## 0. Process artifact

Spec was produced by running 3 specialist agents (UX, Feature/Engineering, Product) for 5 rounds:

1. **R1 — independent proposals.** Each agent proposed from their lens, no knowledge of the others.
2. **R2 — cross-critique.** Each agent received the other two's R1 and critiqued.
3. **R3 — refinement.** Each agent revised their own proposal absorbing the critiques.
4. **R4 — convergence.** 11 named conflicts settled by vote; resolution table below.
5. **R5 — sign-off.** Each agent gave conditional approval + 3 veto criteria.

Convergence is high enough that no further rounds are required before build.

---

## 1. Scope & timeline

| Phase        | Window          | Headline                                                                               |
| ------------ | --------------- | -------------------------------------------------------------------------------------- |
| **Sprint 0** | Day -3 → Day 0  | Library + parser spike + Gemini JSON dry-runs + Redis Lua benchmark                    |
| **M1**       | Day 0 → Day 17  | Upload + Edit + Batched AI + Apply Queue + Cover letter + Cost guard. **Admin-only.**  |
| **M2**       | Day 18 → Day 32 | Targeting card + analyzer-side scoring + basic-formatting PDF + "Why jobs get skipped" |
| **M3**       | Day 33+         | Gated on M1 metrics — fit-score cache (only if p95>400ms), A/B compare, CL templates   |

Phase 1 is **admin-only**, max **50 resumes per user**, max **20 versions per resume**.

---

## 2. UI surfaces

### Routes

- `/admin/resumes` — Library (existing mockup, wired to real APIs)
- `/admin/resumes/:id` — Detail
- `/admin/resumes/:id/edit` — Editor (full route)
- `/resume/queue` — Apply Queue (**user surface**, not under `/admin`)
- `/resume/queue?edit=<jobId>` — side-sheet editor over the queue list (browser back closes sheet, URL is deep-linkable)

### Editor layout

Single canvas, 3-column desktop:

- **Left rail (collapsible):** ResumePicker, version history list, "Active for Jobs page" toggle
- **Center canvas:** read-only formatted preview via `ResumeRenderer`; gutter ribbon (4px, `aria-label="N comments on this block"`) for anchor indicators. **No inline diff markup on the canvas** — diffs render in the right panel.
- **Right panel (collapsible, tabs):** **Comments** | **Drafts** | **Targeting** (Targeting dormant in M1, lights up M2)

### 4 M1 components

`ResumeRenderer`, `SelectionToolbar`, `CommentPopover`, `ResumePicker`.

**Not in M1:** `InlineDiffSpan`, `CommentQueuePanel` as net-new chrome, `BulkFinalize`, modal-overlay editor.

### Cost counter

Editor header chip via `useQuota()` hook — TanStack Query key `['quota','daily']`, staleTime 30s. Batch Review modal subscribes to the same query key (one source of truth, two render points; no dual fetch).

### Parse-fallback UX

**Persistent yellow banner** above canvas:

> "Imported as plain text — comments and AI batching disabled. Re-upload as DOCX to enable."

Not a one-time toast. Banner is dismissible per session but reappears next visit.

---

## 3. Data model

Already in the existing mockup (immutable per upload):

```sql
user_resumes(id, user_id, name, label, tags, active_version_id, ...)
resume_versions(id, resume_id, version_number, storage_path, ...)
```

New for Phase 1:

```sql
resume_blocks(
  resume_id uuid,
  version_id uuid,
  ordinal int,
  block_id text,                  -- ULID, stable within version
  block_type text,                -- heading|paragraph|bullet|list_item|table_cell|unknown
  content text,
  char_count int,
  PRIMARY KEY (resume_id, version_id, ordinal)
);

resume_comments(
  id uuid PRIMARY KEY,
  resume_version_id uuid NOT NULL,
  comment_group_id uuid NOT NULL,  -- cross-block selections share this
  block_id text NOT NULL,
  char_offset_start int,
  char_offset_end int,
  anchor_quote text NOT NULL,      -- verbatim, for drift detection
  author_id uuid NOT NULL,
  body text,                       -- null on satellite anchors of a group
  status text CHECK (status IN ('open','queued','resolved','dismissed','failed')),
  batch_id uuid NULL,
  created_at timestamptz
);

resume_ai_batches(
  id uuid PRIMARY KEY,
  resume_id uuid,
  requested_by uuid,
  kind text CHECK (kind IN ('apply_queue','rewrite','cover_letter')),
  status text CHECK (status IN ('pending','running','succeeded','failed')),
  prompt_hash text,               -- idempotency
  token_input int,
  token_output int,
  cost_usd numeric(10,6),
  started_at timestamptz,
  finished_at timestamptz,
  error_code text
);
-- Full LLM request/response NOT stored here. Logged to Loki — see §7.

resume_batch_artifacts(
  batch_id uuid,
  job_id uuid,
  kind text CHECK (kind IN ('cover_letter','fit_rationale','tailored_summary')),
  content jsonb,
  created_at timestamptz,
  PRIMARY KEY (batch_id, job_id, kind),
  FOREIGN KEY (batch_id) REFERENCES resume_ai_batches(id) ON DELETE CASCADE
);
```

### Resume rollback (R5 PM catch)

`resume_blocks` is keyed on `(resume_id, version_id, ordinal)` — blocks are versioned with their parent. Accepting a draft creates a NEW `resume_versions` row + new `resume_blocks` rows; prior version's blocks stay intact. **Drafts tab = read-only artifact history** (no separate revisions table). Document this in the migration comments.

---

## 4. AI batching mechanics

- **Submit gate:** Apply button disabled until ≥2 comments queued.
- **One Gemini structured-JSON call** returns `{resume_edits[], cover_letter?}`. Cover letter is **opt-in checkbox** in the batch composer, **default OFF** (PM veto if default ON).
- **JD context payload (~600 tokens):** `title + role_category + seniority + experience_years + tech_stack + responsibilities + location_signal`. **Cut:** salary, raw description.
- **Each `resume_edit` MUST carry its originating `comment_group_id`** in the response schema. Edits without a match surface in a "1 suggestion couldn't be matched" footer row — **never silently shown unanchored**.
- **Drift detection:** verbatim `anchor_quote` re-checked against current block text. **Both sides normalized** before compare: NFKC + collapse whitespace + straighten quotes. **Minimum anchor length 40 chars** enforced in the response schema.
- **Per-version row lock** prevents concurrent batches on the same version.
- **No reaper in M1.** Stuck batches manually failed via `POST /admin/resumes/batches/:id/fail`.

---

## 5. Block parsing (Go)

| Library                           | Role                                      |
| --------------------------------- | ----------------------------------------- |
| `github.com/nguyenthenguyen/docx` | DOCX read (primary)                       |
| `github.com/unidoc/unioffice`     | DOCX table extraction (fallback)          |
| `github.com/fumiama/go-docx`      | DOCX export                               |
| in-house                          | Markdown export (block tree → CommonMark) |

**No PDF in M1** — no parser, no export. Eng swapped out `baliance.com/gooxml` (unmaintained since 2021) for `unidoc/unioffice` (AGPL community edition, fine for admin-only).

### Deterministic reject rule

Parse fails if ANY of:

- Zero `experience` blocks detected
- More than 2 `unknown` blocks
- Parsed chars <60% of source chars

Otherwise → plain-text fallback (single block, comments + batching disabled).

---

## 6. Backend slice

`service/resume/` decorator chain — same convention as existing services:

```
Rest → Validation → Cache → Impl → Repo
```

Storage interface abstracts Supabase Storage (no direct vendor lock-in).

RBAC0 gating: all endpoints behind `RequireRole("admin")` + `AuditAdminMutation`.

---

## 7. Cost ceiling

| Knob               | Value      |
| ------------------ | ---------- |
| Batches/admin/day  | **20**     |
| Tokens/admin/day   | **50,000** |
| Cost/batch ceiling | **$0.012** |

Enforced via Redis sliding-window in `RequireResumeBudget` middleware on `POST /admin/resumes/*/ai/*`. Lua `EVAL` reserves token estimate pre-call, refunds delta after settle (avoids race under concurrent submits).

Keys TTL = 26h, no cron reset needed.

Over-cap response:

```
HTTP 429
Retry-After: <seconds-until-midnight-UTC>
{
  "error": {
    "code": "BUDGET_EXCEEDED",
    "scope": "batches|tokens|per_call",
    "limit": 20,
    "used": 20,
    "resets_at": "2026-05-24T00:00:00Z"
  }
}
```

---

## 8. Logging (OTel + Loki)

Span name: `resume.ai.batch`.

Required fields (via `platform/logger` constants only):

`resume.batch_id` · `resume.id` · `llm.model` · `llm.prompt_tokens` · `llm.completion_tokens` · `llm.cost_usd` · `llm.latency_ms` · `llm.prompt_hash` · `llm.response_excerpt` (1KB cap)

---

## 9. Filter vs scoring split

- **Filter stays GLOBAL.** No per-resume routing in the filter — throughput must not degrade.
- **Per-resume scoring** lives in `service/fit-score/` post-analyzer (M2 deliverable). Jaccard on `tech_stack` overlap + experience proximity + role_category match.
- Computed on-read for M1/M2. Cached only in M3 if p95 fit-score render on Jobs page exceeds 400ms.

The "env vars drive filter prompt" anti-pattern is killed correctly by moving personalization to the **scoring/ranking stage**, not the filtering stage.

---

## 10. Sprint 0 gates (must pass before slice code lands)

1. DOCX parser **≥85% accuracy** on a 20-resume fixture corpus
2. Gemini structured-JSON **≥95% parseable** across 50 dry-run calls with the real JD payload shape
3. Redis Lua reserve-refund **<2ms p99** under 50 concurrent admin requests

If any gate fails → descope before writing slice code, not after.

---

## 11. Pre-launch gates (must pass before flipping admin flag ON)

1. Parse succeeds on **≥4 of 5 Day-0 sample DOCXs** (PM picks the 5 — must include one ATS-template and one Google Docs export)
2. End-to-end batch (parse → edit → generate resume+cover → save artifact → quota decrement) run by **2 non-author humans without intervention**
3. Cost middleware **rejects the 11th batch** when daily cap is set to 10 in staging
4. `resume_ai_batches` ledger **ships in the same PR** as the cost middleware (no shipping the wall without the audit trail)

---

## 12. Success metrics

| Metric               | Threshold                                     | Source                                  |
| -------------------- | --------------------------------------------- | --------------------------------------- |
| Adoption (Day 24)    | ≥1 resume uploaded                            | `user_resumes` count                    |
| Engagement           | median **3–6 comments/batch**                 | `resume_comments` grouped by `batch_id` |
| Cost                 | p95 ≤$0.009/batch; ceiling hit <2% of batches | `resume_ai_batches.cost_usd`            |
| Quality              | ≥40% of batched jobs `applied` within 7 days  | `user_job_state.applied_at` join        |
| Reliability          | batch success rate ≥97%, p95 latency ≤25s     | `resume_ai_batches.status`              |
| Early canary (Day 7) | <50% of admins stop at batch #1               | `batches_per_user_per_7d` (Loki field)  |

### Day-30 single kill metric

**`applied_within_7d_of_batch / total_batches`:**

- **<25%** → kills feature
- **25–40%** → triggers M2 investigation-only (no targeting card build)
- **≥40%** → ships M2 as planned

Application conversion is the only metric that proves resume tailoring changed admin behavior. Adoption and cost prove the feature works, not that it's worth maintaining.

---

## 13. Veto matrix

Any single veto blocks launch.

| ID    | Trigger                                                                                   |
| ----- | ----------------------------------------------------------------------------------------- |
| UX-1  | Daily quota counter only in modal, not editor header                                      |
| UX-2  | SelectionToolbar requires >1 click to attach a comment to a selection                     |
| UX-3  | Apply Queue side-sheet doesn't preserve scroll + selected resume on reopen                |
| Eng-1 | DOCX parser accuracy drops <80% on the fixture corpus during build                        |
| Eng-2 | Gemini malformed `anchor_quote` >5% (smart quotes, whitespace drift)                      |
| Eng-3 | `resume_ai_batches` row-lock p95 >8s under 2 concurrent admins on the same resume version |
| PM-1  | Cover-letter opt-in defaults to ON                                                        |
| PM-2  | Comments tab scope grows to include @mentions, threading, or resolution states            |
| PM-3  | Parser hits 3/5 instead of 4/5 on Day-0 samples and team proposes "fix in M1.1"           |

---

## 14. M1 cuts (do NOT build)

- PDF export (parser or exporter)
- Targeting pill on Jobs page header
- Targeting card itself (until M2 scoring lights up)
- Fit-score cache table
- Reaper worker
- Dismissal-reason filter wiring (insight panel comes in M2)
- Real-time collaborative editing
- Template / theme gallery
- WYSIWYG formatted editing of original file bytes
- Bulk finalize
- Save-and-next cycling
- Modal-overlay editor for Apply Queue
- Per-resume filter routing
- LLM-rationale persistence (returns inline on-demand instead)

---

## 15. M2 deliverables (one PR per item)

1. **Targeting card** (editor right panel) + **analyzer-side per-resume scoring** in `service/fit-score/`, Jaccard computed on-read
2. **Basic-formatting PDF export** with explicit downgrade banner ("Visual styling from your original is not preserved")
3. **"Why jobs get skipped" insight panel** on Resume Detail page
   - Schema: `user_resume_preferences(user_id PK, negative_signals jsonb, last_clustered_at, signal_version)`
   - Sunday 03:00 UTC weekly cron; gated on ≥20 dismissals/90d per user (else skip with log)
   - **Cut from M3 if <30% open-rate within 30 days of M2 launch**

---

## 16. M3 (metric-gated, do NOT preemptively build)

- Fit-score cache table, ONLY if p95 Jobs-page fit-score render exceeds 400ms in production
- A/B resume comparison
- Cover-letter template library

---

## 17. Top risks (R5 callouts)

| Lens | Risk                                                      | Mitigation (no scope expansion)                                                                                     |
| ---- | --------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| UX   | User can't tell which AI edit answers which comment       | Schema requires `comment_group_id` on every `resume_edit`; unmatched edits → "1 suggestion couldn't be matched" row |
| Eng  | Anchor-quote drift (whitespace, smart quotes, paraphrase) | Normalize both sides (NFKC + collapse + straighten) before compare; min 40-char anchor length enforced in schema    |
| PM   | Admin runs one batch, never returns — feature is a demo   | Log `batches_per_user_per_7d` from Day 1; >50% one-batch stop → loop is broken before Day-30 metrics roll up        |

---

## 18. North-star sentence

> _"On day 1, an admin opens a job, highlights three weak lines on their resume, comments, clicks Run, and accepts targeted edits in under 90 seconds."_

---

## 19. Glossary of decisions (Round 4 vote table)

| #   | Decision                                                                                          | Vote                  |
| --- | ------------------------------------------------------------------------------------------------- | --------------------- |
| C1  | Per-batch ceiling **$0.012**                                                                      | UX, PM (Eng accepted) |
| C2  | **No fit-score cache table** M1/M2; revisit only if p95>400ms                                     | Unanimous             |
| C3  | **No targeting pill on Jobs page header.** Allowed in JobDetailPanel + ResumePicker + batch modal | UX, PM (Eng accepted) |
| C4  | Targeting card **M2** (with the scoring it depicts). No M1 placeholder                            | Unanimous             |
| C5  | **Normalized `resume_blocks` table** (not jsonb tree on version row)                              | Eng, PM (UX accepted) |
| C6  | **`resume_batch_artifacts` table** for cover letters + fit rationale                              | Unanimous             |
| C7  | Deterministic parse-reject rule + plain-text fallback                                             | Synthesized           |
| C8  | **`comment_group_id`** column kept for cross-block selections                                     | Unanimous             |
| C9  | M2 dismissal-reason insight panel on Resume Detail page, cron-fed, kill-gated                     | Unanimous             |
| C10 | Apply Queue at **`/resume/queue?edit=<jobId>`** (user surface, not admin path)                    | Unanimous             |
| C11 | Quota counter in editor header; modal subscribes to same TanStack key                             | Synthesized           |

---

## 20. References

- Frontend mockup: `ui/console/src/features/resumes/` and `ui/console/src/pages/admin/Resume*Page.tsx`
- Sidebar nav entries: `ui/console/src/components/Sidebar.tsx`
- Routes: `ui/console/src/App.tsx`
- Backend skeleton (not yet built): planned `backend/service/resume/`
- Existing JobRadar conventions: `backend/service/CLAUDE.md`, `ui/console/CLAUDE.md`, `.claude/rules/`
