# resume/ — Resume CRUD + object storage; synchronous text extraction for keyword matching

## No LLM dependency

`service/resume` has **zero** dependency on `service/llm` — a prior design (LLM-based keyword extraction, background goroutine + retry endpoint) was fully removed; see `docs/superpowers/specs/2026-07-05-resume-job-keyword-match-design.md`. Do not reintroduce an LLM call here — if you're looking at an old reference to `ResumeKeywordExtractor`, `POST /resumes/{id}/keywords`, or a `keywords` column, it's stale.

## Decorator chain

```
RestRouter → RestService → ValidationService → ImplService → RepositoryService (terminal)
```

Bare `NewX` constructor naming (no `Resume` prefix) — this slice's naming/layout was the template `applyqueue/` was told to mirror exactly (see `applyqueue/CLAUDE.md`).

## Upload runs text extraction synchronously, in-request

`ResumeImplService.Upload` calls `textextract.ExtractText` (PDF/DOCX → plain text, `service/resume/textextract`) **before** the object-storage write, in the same request — not a background goroutine, no retry endpoint. A file that fails extraction (scanned/image-only PDF, corrupt file, below the minimum text-length threshold) fails the upload itself with a clear, typed error (`ErrBadRequest` with a user-facing message) rather than uploading successfully and silently having no matchable content later. Order of operations on success: extract text → write to object storage → insert DB row (with `extracted_text` already populated) → on a failed DB insert, delete the just-written object immediately (the failure is known synchronously, so there's no orphan-cleanup race).

`ExtractedText` (column added in migration 090, replacing the earlier LLM-derived `keywords` JSONB column) is stored whitespace-normalized and is never displayed to the user directly — its only consumer is resume-vs-job keyword matching, which happens **client-side** in the frontend (`src/lib/resumeJobMatch.ts`), not in this backend. `applyqueue/`'s `attachJobKeywords` attaches the job's raw keywords to each queue row; the frontend does the actual matching against `extracted_text`. No backend matching/scoring logic exists in this domain or in `applyqueue/`.

## Key invariants

- `Upload` enforces the per-user resume cap (`resumecommon.ResumeLimitPerUser` = 50) inside the same DB transaction as the insert. On Postgres, a session-scoped advisory lock (`pg_advisory_xact_lock`, keyed by a hash of the user id) serializes concurrent uploads from the same user for the transaction's lifetime, closing the race where two requests both read `count=49` before either commits. SQLite (used by unit tests) has no advisory-lock primitive, so that step is skipped there — the concurrent-race guarantee itself is an integration-tier concern, verified against real Postgres, not sqlite unit tests.
- `ValidationService.Upload` sniffs the real content type via magic bytes (`filetype.Detect`) — it never trusts the client's declared `Content-Type` header or file extension. Only PDF and DOCX are accepted.
- A blank `label` on upload falls back to the original filename (minus extension); `Rename` requires a non-empty label — there's no filename to fall back to for an explicit rename, and rename-to-blank is ambiguous UX rather than an obvious "reset."
- `Delete` removes the DB row first, then best-effort deletes the object from storage; a storage-delete failure is logged but never blocks the user-visible delete (an orphaned object is accepted debt — no reaper yet — since a DB row kept alive by a failed storage delete would be the worse, user-visible failure mode).
- `RepositoryService.Download` is intentionally unreachable in the real chain — `ImplService.Download` overrides it and never delegates further (object-storage access does not belong in the GORM-only repository layer); the repo's implementation exists only so it satisfies `Service` completely.
- `StorageKey` is excluded from every JSON response (`json:"-"`) — it's an internal object-storage path, never exposed to a client.
- `MaxFileSizeBytes` (2MB) matches the `resumes.size_bytes` CHECK constraint in migration 086 — keep both in sync if this ever changes.
