# applyqueue/ — Apply-queue list + resume attachment; on-read resume-match recommendations

## Deviation from canonical calling convention

Every `Service` method takes `userID` as an **explicit parameter**, threaded through every layer (`RestService` pulls it from ctx via `identity.MustFromContext`, then passes it down explicitly) — this deliberately mirrors `resume/`'s convention, not `job/`'s ctx-implicit pattern (where `identity.UserIDFrom(ctx)` is called again at each layer that needs it). This slice's mandate was to mirror `resume/`'s file layout and calling convention exactly; do not "fix" this to match `job/` — it is intentional, not drift.

```
RestRouter → RestService → ValidationService → ImplService → RepositoryService (terminal)
```

`ImplService` is currently a pure pass-through (`implService{ BaseService }`, no overrides) — kept as an explicit layer (not skipped) so the chain stays uniform with every other slice and a future orchestration need (e.g. invalidating a cached queue page after `SetResume`) has an obvious home without reshaping the chain.

## Three constructors over one stateless struct

`repositoryService` (in `ApplyQueueRepositoryService.go`) backs **three** separate constructors, each returning a different narrow interface:

| Constructor                 | Returns                  | Consumed by                                                                                       |
| --------------------------- | ------------------------ | ------------------------------------------------------------------------------------------------- |
| `NewRepositoryService`      | `Service`                | The terminal repo in the main chain (`List`, `SetResume`)                                         |
| `NewEngagementChecker`      | `EngagementChecker`      | `ValidationService` — confirms `user_job_state` exists for (userID, jobID) before a resume attach |
| `NewResumeOwnershipChecker` | `ResumeOwnershipChecker` | `ValidationService` — confirms a resume belongs to userID before attach                           |

This is deliberate, not accidental duplication: a single constructor returning a combined interface would create two candidate providers for `Service` once `ImplService`'s constructor joins the set, which is exactly the kind of ambiguity Wire's decorator-chain graphs can't resolve (see the wire-codegen note in `docs/tech-debt/2026-07-05-backend-service-boilerplate-debt.md`).

## Validation performs live DB reads — not pure field validation

`ApplyQueueValidationService.go`'s `SetResume` calls `EngagementChecker.HasEngagement` and `ResumeOwnershipChecker.ResumeOwnedByUser` — both hit the DB. This is authorization/ownership checking, not the pure in-memory field validation `job/`'s and `company/`'s `ValidationService` layers do. If you're using another slice's `ValidationService` as a template, don't assume "Validation" always means side-effect-free.

## Resume-match recommendations (`ApplyQueueMatching.go`)

Computed on every `List` read, never cached, never stored:

- `loadJobTechTags` reads `job_tech_tags` (the cleaned tags — see `techtag.Clean` — not the raw, uncleaned `jobs.tech_stack` JSONB column) for every job on the page in one query.
- `loadUserResumeKeywords` reads every resume owned by the caller (id, label, keywords) in one query.
- Matching itself (Jaccard similarity over the in-memory tag/keyword sets) is pure Go — deliberately not a SQL computation or a cached fit-score table, mirroring the frozen spec's own precedent of no fit-score cache until proven necessary by p95 latency (see issue #264).
- Two queries total per page, never one query per row — the "join once per page" requirement.

## Key invariants

- `List`'s keyset cursor sorts on `user_job_state.updated_at DESC, job_id DESC` — `updated_at` (not `added_at`/`saved_at`) because it already reflects the most recent engagement action (save, status change, resume attach), the most useful "recently active" ordering for a queue the user is actively working through.
- `terminalStatusExclusion` is applied to every `List` query regardless of filter: a dismissed or terminal-outcome (offer/rejected) job never appears in the apply queue, even if `saved_at` is still set (it survives a dismiss — see `UserJobStateRepository.Upsert`'s `StatusDismissed` case).
- `deriveStatus` restricts every response to exactly `{saved, applied, interviewing}`, matching the frontend's `ApplyQueueStatusSchema` three-value union.
- `SetResume(resumeID == nil)` deletes the `resume_job_attachments` row (idempotent — no error if absent); `SetResume(resumeID != nil)` upserts via `ON CONFLICT (user_id, job_id) DO UPDATE` — a second attach overwrites the first rather than erroring.
