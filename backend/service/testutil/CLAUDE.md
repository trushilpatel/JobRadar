# testutil/ — Shared test helpers for user-scoping assertions across domain test suites

## What lives here

This is **not** a domain slice — it has no service, no repository, no router, no wire setup. It contains cross-domain test utilities shared by the per-domain `tests/` folders.

| File              | Purpose                                                                                                                  |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `userid_guard.go` | `AssertUserScoped(t, name, fn)` — verifies that a repo method returns a non-nil error when called with an empty `userID` |

## Usage

```go
testutil.AssertUserScoped(t, "JobRepo.List", func(uid string) error {
    _, err := repo.List(ctx, uid, filter)
    return err
})
```

Call this for every user-scoped repository method. A missing `userID` must fail loudly — a forgotten ctx scoping that silently broadens a query to all users is the failure mode this guards against.

## Import rule

`testutil` must never import any concrete service slice (`service/job/services`, `service/user/services`, etc.). It depends only on `"testing"`. Importing a concrete slice would pull implementation code into every test binary that uses the helper.
