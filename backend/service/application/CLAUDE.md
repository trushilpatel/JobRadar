# application/ — Application tracker: create, list, and stage/note updates per user

## Deviation from canonical layout

The `RepositoryService` does **not** implement `ApplicationService`. It implements a separate `ApplicationRepository` interface (declared in `ApplicationService.go`). The `ImplService` is the terminal `ApplicationService` — it bridges the two by pulling `userID` from ctx and calling the repository with it explicitly.

```
RepositoryService (ApplicationRepository)
  → ImplService (ApplicationService — derives userID from ctx)
    → ValidationService → RestService → RestRouter
```

`ImplService` takes a cross-domain dependency on `job/services.JobService` to verify the job exists before inserting an application row.

## Interfaces

| Interface               | Owner                   | Purpose                                                                      |
| ----------------------- | ----------------------- | ---------------------------------------------------------------------------- |
| `ApplicationService`    | `ApplicationService.go` | User-facing contract; all layers in the chain except the repo implement this |
| `ApplicationRepository` | `ApplicationService.go` | Per-user DB contract; takes `userID` as an explicit parameter                |

## Key invariants

- `userID` is **never** sourced from a parameter — the impl layer calls `identity.UserIDFrom(ctx)` and passes it to the repo. Empty `userID` returns `ErrInternal.WithMessage("user id required")` — a missing ctx scoping must never silently broaden a query.
- Creating an application verifies the job exists via `JobService.Get` but does **not** touch `user_job_state.status` — that is updated separately via the job slice to avoid cross-domain write coupling and recursion.
- `UpdateStage` and `UpdateStageByID` both converge on `repo.UpdateStage(ctx, userID, appID, input)` — the `jobID` URL parameter on the former is validated but unused at the repo layer.
- Pagination for `ListAll` uses page+offset (not keyset cursor) — `application_tracker` is user-scoped and small; the keyset exception documented in `service/CLAUDE.md` applies here.

## Non-canonical files

| File                             | Purpose                                                                                                        |
| -------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `services/ApplicationService.go` | Declares both `ApplicationService` AND `ApplicationRepository` interfaces, plus `ApplicationWithJob` join type |
| `compat.go`                      | Slice-root package with build constraint — no domain logic                                                     |
