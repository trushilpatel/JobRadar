# Documentation Maintenance Rules

Documentation is code. Stale docs are bugs. Every change that affects a documented area must update that area in the same work unit — not as a follow-up.

## What to update and when

| Change type                                              | Required doc update                                                 |
| -------------------------------------------------------- | ------------------------------------------------------------------- |
| New service slice (`service/<domain>/`)                  | `CLAUDE.md` directory map, `docs/ARCHITECTURE.md` service table     |
| New DB table or table schema change                      | `CLAUDE.md` database tables section                                 |
| New API route or route group                             | `CLAUDE.md` relevant endpoint table (dashboard, admin, etc.)        |
| New binary / worker                                      | `CLAUDE.md` workers table, `docs/ARCHITECTURE.md` system map        |
| New env var                                              | `CLAUDE.md` env vars section (or the section nearest to the config) |
| New ATS adapter                                          | `CLAUDE.md` crawler Dockerfile note + adapter status list           |
| New platform package (`platform/<pkg>/`)                 | `CLAUDE.md` directory map                                           |
| Service contract change (interface method added/removed) | The service's own `CLAUDE.md` if it exists                          |
| Multi-user / RBAC / identity change                      | `CLAUDE.md` multi-user section                                      |
| Feature delivery                                         | Spec file in `docs/superpowers/specs/YYYY-MM-DD-<slug>.md`          |

## Spec files (`docs/superpowers/specs/`)

Write one after every non-trivial feature or fix. Keep it tight:

- Date-prefixed filename: `2026-05-11-nil-slice-null-api.md`
- Content: file-level references only (which file does what) — no code snippets
- Three sections max: **What changed**, **Why**, **Files touched**
- No padding, no filler prose

## CLAUDE.md is the source of truth for AI context

- It is the first file any AI reads in a new session — keep it accurate
- If a section is wrong, fix it now; never let it drift
- Add new sections only when a new structural concept is introduced; do not add sections for one-off facts

## What NOT to document

- Implementation details already visible from reading the code
- Git history, PR descriptions, or who changed what
- Temporary state, in-progress work, or TODOs that belong in a task tracker
- Decisions that are already enforced by types, constants, or the compiler

## Architectural suggestions

When making any code change, scan for the following and surface a short suggestion at the end of the response if spotted — one sentence each, no implementation:

- A pattern used in 2+ places that has no shared abstraction yet
- A nil/zero-value assumption that exists elsewhere and will cause the same class of bug
- An interface boundary that is wider than its callers need
- A service or component that now has more than one clear reason to change
- A test gap: a code path that is exercised in prod but has no test coverage

Surface these as **"Architectural note:"** bullets at the bottom of the response. Do not implement them unless the user asks. Do not repeat a suggestion that was already raised in the same session.
