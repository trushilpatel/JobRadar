# Resume–Job Keyword Match (No-LLM) — Design

## Problem

Resume keyword extraction currently depends on an LLM call (`POST /resumes/{id}/keywords`), which is unreliable (the endpoint 500s opaquely on any failure in the storage/parse/LLM chain), costs quota, and produces a standalone "resume keywords" concept that's shown to the user (badges on `ResumeCard`) when the actual goal is narrower: given a job's required keywords, does this resume mention them?

The job side (`job_tech_tags`, via the enricher) is also too narrow today — it explicitly excludes methodologies, soft skills, tools, and domain terms, which are common ATS/recruiter keyword targets.

## Goals

- Remove the LLM entirely from resume-side processing.
- Broaden the enricher's job-keyword extraction to cover methodologies/practices, non-dev tools/platforms, soft skills, and certifications/domain terms — not just core tech stack.
- Match job keywords against resume content directly; never surface a standalone "resume keywords" list to the user.
- Move the actual matching computation to the frontend (client-side), since it only needs data the browser already loads (job keywords + the user's resumes) — backend serves data, not verdicts.
- Show a one-line match summary in the Apply Queue table, next to the resume picker, only once a resume is attached (auto-picked or user-chosen).
- Centralize query-cache TTL configuration for the queries this feature touches, without touching queries that must stay live (e.g. apply-queue's own list).

## Non-goals

- Backfilling `extracted_text` for pre-existing resumes (explicitly declined — re-upload is acceptable).
- Categorized keyword storage (methodologies vs. tools vs. soft skills) — flat list only.
- A detail-panel keyword breakdown or row-level match badge — one-line summary in a column next to the resume picker only.

## 1. Enricher — broaden keyword categories

`service/llm/prompts/Enrich.go`'s `tech_stack` field instructions change from excluding methodologies/soft-skills/patterns/generic-tools to explicitly including:

- Methodologies/practices explicitly named (Agile, Scrum, Kanban, CI/CD, TDD, Microservices, MVC, REST)
- Non-dev tools/platforms explicitly named (Jira, Confluence, Figma, Salesforce, Tableau)
- Soft skills explicitly named (Leadership, Communication, Stakeholder management, Mentorship)
- Certifications/domain terms explicitly named (AWS Certified, PMP, HIPAA, fintech, healthcare)

Field name, JSON shape, and destination table (`job_tech_tags`) are unchanged — this is a content-broadening change, not a schema/pipeline rename, to avoid unnecessary churn on a working, indexed table. Still one flat list per job.

`service/job/techtag/clean.go`'s `blocklist` currently hard-drops exactly the newly-wanted terms (agile, scrum, ci/cd, leadership, communication, teamwork, collaboration, etc.). These move out of `blocklist` and into `canonical` where a normalization makes sense (e.g. "ci/cd" / "ci cd" → "CI/CD"); only genuine noise stays blocked (generic office tools, bare "api"/"json"/"html"/"sql", generic OS names).

`job_tech_tags` has no other active consumer today (confirmed via grep — no insights/trends dashboard reads it), so broadening its contents has no blast radius beyond apply-queue.

## 2. Resume domain — drop the LLM

- Migration: drop `resumes.keywords JSONB`, add `resumes.extracted_text TEXT NOT NULL DEFAULT ''`.
- `POST /resumes` (`ResumeImplService.Upload`) runs text extraction (`textextract.ExtractText`, PDF/DOCX → plain text) **synchronously** as part of the upload request — it no longer waits on an LLM, so it's fast. A file that fails extraction (scanned/image-only PDF, corrupt file, under the minimum-length threshold) fails the upload itself with a clear error, rather than uploading successfully and silently having no keywords later.
- `extracted_text` is stored with whitespace normalized (runs of spaces/newlines collapsed to a single space). PDF extraction commonly line-wraps mid-phrase; without normalization a multi-word job keyword (e.g. "machine learning") could span a stray line break in the raw extracted text and never match. Normalizing once at write time keeps the frontend matcher a simple single-pass regex against one clean string.
- Delete entirely: `POST /resumes/{id}/keywords` route (`ResumeRestRouter.go`), its handler (`ResumeRestService.go`), `ExtractKeywords` from the `ResumeService` contract, `ResumeKeywordExtractor` interface, `ResumeImplService.runKeywordExtraction` / `scheduleKeywordExtraction`, the Gemini `ExtractResumeKeywords` implementation + prompt (`prompts.BuildResumeKeywords`), the `noopKeywordExtractor` wire fallback (`wire_gen.go`), and the "Extract keywords" button + keyword badges on `ResumeCard.tsx`.
- `service/resume` no longer imports `service/llm` — the domain has zero LLM dependency after this change.

## 3. Apply Queue backend — serve data, not a verdict

- Delete `ApplyQueueMatching.go` in full: `jaccardSimilarity`, `bestMatchingResume`, `parseResumeKeywordRows`, `loadUserResumeKeywords`. No matching/scoring logic remains in the backend.
- `dto.QueueItem` drops `RecommendedResumeID` / `RecommendedResumeLabel`, gains `JobKeywords []string` — the job's canonical `job_tech_tags`, fetched the same way `loadJobTechTags` does today (one query per page of jobs), attached raw with no scoring.
- `GET /resumes` response gains `extracted_text` on each resume — the same endpoint the resume picker already calls, so no new network request is introduced on the Apply Queue page.

## 4. Frontend — matching lives at the page layer

`features/apply-queue` and `features/resumes` cannot import each other (iron rule in `features/CLAUDE.md`). The matching function is a pure utility in `src/lib/` (e.g. `resumeJobMatch.ts`) with no feature dependency — it takes plain data:

```ts
function matchResume(jobKeywords: string[], resumeText: string): { matched: number; total: number }
function pickBestResume(
  jobKeywords: string[],
  resumes: { id: string; label: string; extractedText: string }[]
): { id: string; label: string } | null
```

The page component that already composes both features (resumes list + apply-queue items) calls these — same place today's cross-feature mapping already happens for `ApplyQueueResumeOption`.

**Matching algorithm:** for each job keyword, case-insensitive word-boundary regex match against the resume's `extracted_text` (each keyword escaped for regex special characters — required for tags like "C++", "Node.js", "C#"). Score = matched-count / total job keywords. `pickBestResume` picks the highest score among the user's resumes; ties keep the first-encountered (oldest-uploaded) resume, matching the current backend tie-break semantics.

**Auto-pick vs. manual pick:** when no resume is attached yet, the picker defaults to `pickBestResume`'s result. Once the user manually selects a resume (or the auto-pick gets persisted as an attachment), that choice is authoritative — this requires no new logic since `resume_job_attachments` already always wins over any recommendation.

**UI:** a new cell/column next to `ResumePickerCell` in `apply-queue-columns.tsx` shows the one-line summary (e.g. "7/10 matched") computed via `matchResume`, rendered only when a resume is currently attached (auto-picked or user-chosen). No detail-panel breakdown, no row-level percentage badge elsewhere.

## 5. Cleanup / migration

- Single migration: drop `keywords`, add `extracted_text` on `resumes`.
- No backfill for pre-existing resumes (explicit decision) — they show no match info until re-uploaded.
- Remove now-dead LLM plumbing: `ResumeKeywordExtractor` contract, Gemini keyword-extraction method, `BuildResumeKeywords` prompt, related constants (`resumecommon.KeywordExtractionTimeout`, `MinTextLength` moves to the sync-upload path).

## 6. Frontend cache config (scoped to this feature)

Query cache TTLs are currently inline magic numbers scattered per hook (`staleTime: 30_000` repeated across `useResumes.ts`, `useApplyQueue.ts`, `useJobs.ts`, etc.), each independently choosing a value. This change introduces one central config, scoped to what this feature touches — not a repo-wide migration:

`src/lib/core/cacheConfig.ts`:

```ts
// staleTime: how long cached data is considered fresh (no automatic refetch).
// gcTime: how long unused data stays in cache before eviction.
export const CACHE_TTL = {
  resumes: { staleTime: 5 * 60_000, gcTime: 30 * 60_000 }, // rarely changes; upload/delete already invalidate explicitly
} as const
```

`useResumes.ts` references `CACHE_TTL.resumes.staleTime` / `.gcTime` instead of the bare `30_000` literal it has today.

**`useApplyQueue.ts` is explicitly left untouched** — it already has documented reasoning for why it can't fully trust a staleTime window (job status / resume attachment must read as current on every navigation), and is exactly the "important live API" this change must not touch. It is not added to `CACHE_TTL`.

This is additive and extensible: a future domain adopts the same pattern by adding one entry to `CACHE_TTL` and pointing its hook at it — no change to the mechanism itself. A repo-wide migration of every existing `useQuery` call onto this config is out of scope here.

## Testing

- Backend: `textextract` unit tests (short-content rejection, PDF/DOCX parsing) move/adapt to run at upload time; `ResumeImplService.Upload` tests cover the new sync-extraction failure path (bad file → upload fails with a clear error); `techtag.Clean` tests updated for the blocklist→canonical moves; `ApplyQueueRepositoryService` tests updated for `JobKeywords` replacing the recommended-resume fields.
- Frontend: `resumeJobMatch.ts` unit tests — word-boundary correctness (Go vs. Google, C++ escaping), coverage scoring, tie-break on `pickBestResume`, empty-keywords/empty-resumes edge cases. `apply-queue-columns.tsx` / `ResumePickerCell` tests updated for the new match-summary cell and the dropped `recommended` prop plumbing (still sourced from `pickBestResume`, computed at the page layer now instead of read off `item.recommended_resume_id`). `useResumes.ts` test confirms it reads TTLs from `CACHE_TTL.resumes` rather than an inline literal.
