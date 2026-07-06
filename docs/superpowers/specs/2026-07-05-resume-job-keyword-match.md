# Resume–Job Keyword Match (No-LLM) — Implementation Spec

**Date:** 2026-07-05
**Design:** `docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md`
**Plan:** `docs/superpowers/plans/2026-07-05-resume-job-keyword-match.md`

## What shipped

Removed the LLM entirely from resume keyword handling, broadened the enricher's job-keyword extraction past pure tech stack, and moved resume↔job matching to a client-side computation. Backend now only serves plain data (resume plain text, job keywords); no scoring happens server-side.

### Enricher / techtag broadening (Part A)

- `backend/service/llm/prompts/Enrich.go`: `tech_stack` field instruction broadened to explicitly include methodologies/practices, non-dev tools/platforms, soft skills, and certifications/domain terms (previously excluded all four).
- `backend/service/job/techtag/clean.go`: the corresponding terms moved from `blocklist` into `canonical` (with normalization where sensible, e.g. "ci/cd" → "CI/CD"). Only genuine noise (generic office tools, bare acronyms, generic OS names) remains blocked. No other consumer of `job_tech_tags` exists yet, so this has no blast radius beyond apply-queue.

### Migration + textextract (Parts B–C)

- Migration `090_resume_extracted_text.sql`: drops `resumes.keywords` (JSONB), adds `resumes.extracted_text` (TEXT, default `''`). No backfill for pre-existing resumes (explicit product decision) — they show no match info until re-uploaded.
- `backend/service/resume/textextract/extract.go`: `collapseWhitespace` now folds all whitespace including newlines (was horizontal-only, preserving newlines for LLM prompt structure that no longer exists) — prevents a PDF/DOCX line-wrap from splitting a multi-word job keyword like "machine learning" across an unmatched newline.

### Resume domain — LLM fully removed (Parts D–F)

- `backend/service/resume/services/ResumeImplService.go`: `Upload` now runs `textextract.ExtractText` synchronously (no LLM — the parser is local and fast) before the storage write, so an unreadable file fails the upload itself (`ErrBadRequest`) instead of succeeding silently with no keywords later. The old background goroutine, `ExtractKeywords`, and their supporting types are gone.
- Removed entirely: `POST /resumes/{id}/keywords` route/handler, `KeywordExtractor`/`KeywordStore` interfaces, `ResumeKeywordExtractor` from the LLM `Scorer` contract, the Gemini implementation + prompt (`prompts/ResumeKeywords.go`), both `llm/dto/ResumeKeyword*.go` files, and `resumecommon.KeywordExtractionTimeout`.
- `backend/apps/api/wire.go` / `wire_gen.go`: `provideKeywordExtractor`, `noopKeywordExtractor`, `buildLLMQuotaStores` removed; `resume.services.NewImplService` call now takes 3 args instead of 5. (`wire_gen.go` was hand-edited to mirror `wire.go`'s change — the `wire` CLI itself fails with "multiple bindings" errors in this sandbox even against the pre-existing, unmodified `wire.go`, confirmed to be a local toolchain issue unrelated to this change.)
- `service/resume` no longer imports `service/llm` at all.

### Apply Queue backend — serves data, not a verdict (Part G)

- `ApplyQueueMatching.go` deleted in full (Jaccard scoring, `bestMatchingResume`, `parseResumeKeywordRows`). `dto.QueueItem` drops `RecommendedResumeID`/`RecommendedResumeLabel`, gains `JobKeywords []string` — the job's canonical `job_tech_tags`, attached raw with no scoring (logic now lives in `ApplyQueueRepositoryService.go`'s `attachJobKeywords`).

### Frontend (Parts H–I)

- `features/resumes`: `Resume.keywords` → `Resume.extracted_text` (string); "Extract keywords" button/badges and `useExtractResumeKeywords` removed from `ResumeCard.tsx`. `useResumes` now reads its `staleTime`/`gcTime` from a new `lib/core/cacheConfig.ts` (`CACHE_TTL.resumes`, 5min/30min) instead of an inline literal — scoped to this feature only; `useApplyQueue` deliberately untouched since it must stay "live" (job status/resume attachment need to read current on every navigation).
- `lib/utils/resumeJobMatch.ts` (new): pure `matchResume`/`pickBestResume`, word-boundary-equivalent matching via alphanumeric-adjacency lookaround (not `\b` — `\b` fails on symbol-suffixed keywords like "C++" when followed by whitespace, since both sides are non-word characters).
- `pages/apply-queue/useApplyQueueResumeMatch.ts` (new): composes `features/apply-queue` + `features/resumes` (which may not import each other) to compute, per row, the "effective" resume (user's explicit attachment, falling back to the client-computed best match) and a one-line match summary. The auto-pick is display-only — never written back as a real attachment; a manual pick always persists via the existing `setApplyQueueResume` mutation.
- `features/apply-queue`: `recommended_resume_id`/`_label` → `job_keywords` in the API contract; `ResumePickerCell` drops the "Recommended" star badge; `apply-queue-columns.tsx` gains a `Match` column and `getEffectiveResume`/`getMatchSummary` column actions (both computed at the page layer, per the cross-feature import restriction).

## Verification

- Backend: full `go build/vet/test -race/lint` clean across the whole module; every touched package ≥95% statement coverage (project gate), several at 97–100%.
- Frontend: full `lint`/`tsc -b`/`vitest run --coverage` clean — 116 test files, 1248 tests passing.
- Manual: verified against the local `make dev` stack with a real logged-in session — Apply Queue's Match column renders correctly (pre-existing un-migrated resumes correctly show `0/N matched`, consistent with the no-backfill decision), Resumes page shows no keyword UI, `POST /resumes/{id}/keywords` returns 404, a malformed file upload is rejected at validation (400), and a well-formed file passes validation and reaches the DB insert step (proving synchronous extraction succeeds) — full upload completion could not be exercised via the dev-bypass auth token specifically, since its synthetic `"dev-bypass"` subject is not a valid UUID and can never satisfy `resumes.user_id`'s FK to `users(id) UUID` (a pre-existing environment constraint, not something this change introduced).

## Known follow-ups (not built)

- No backfill job for `extracted_text` on resumes uploaded before this change (explicit decision — users re-upload to get matching).
- No general migration of other frontend queries onto `CACHE_TTL` (scoped to `useResumes` only, by design).
