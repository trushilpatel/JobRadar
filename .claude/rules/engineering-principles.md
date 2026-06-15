# Engineering Principles

Cross-cutting principles that apply to every layer: backend, frontend, infra, tooling, migrations. No exceptions for "just a quick change" or "small feature".

---

## Correctness over convenience

Prefer the right solution, even when it means rewriting the entire component. A working solution on top of a wrong foundation compounds debt with every feature added. When the correct path is a full rewrite, say so explicitly and do it — do not paper over structural problems.

**Hard rule:** never choose a shortcut implementation because a rewrite "seems like too much work right now." If it is wrong, fix it properly.

---

## SOLID — applied strictly

SOLID is a hard rule, not a guideline. Go-specific application is in `.claude/rules/backend-architecture.md` (SOLID section). Universal application:

- **Single Responsibility** — every module, function, or component has exactly one reason to change. If a file fetches data _and_ transforms it _and_ renders it, split it into three.
- **Open/Closed** — new behaviour arrives via new types or new implementations, never by modifying working code.
- **Liskov Substitution** — any implementation of an interface is fully interchangeable with any other; no caller should need to know which concrete type it holds.
- **Interface Segregation** — small, focused interfaces only. A caller that depends on 2 of 10 methods should not be coupled to the other 8.
- **Dependency Inversion** — high-level logic depends on abstractions (interfaces); low-level implementations depend on nothing above them.

---

## Loose coupling

- Every cross-module dependency crosses a boundary via an interface or an explicit API — never via a concrete type
- A module may not import another module's internals; it may only consume the interface that module exposes
- Swapping a store, transport, or third-party integration should touch zero business-logic files
- Shared mutable state across modules is banned — if two modules need to share data, define a message, event, or function boundary
- Coupling is measured by: how many files outside the feature's own directory must change when the feature changes. If it is more than 2, the seam design is wrong — fix the seam first

---

## No magic values

- No bare strings for statuses, reasons, stages, roles, topic names, queue names, or any value that appears in more than one place
- No inline numeric literals for timeouts, limits, intervals, port numbers, batch sizes, or retry counts — define a named constant
- Every constant lives in the package that owns the concept and is imported from there
- Frontend: no inline hex colors, no raw `px` values outside a design-token file, no hardcoded API path strings outside `api/*.ts`
- Enums and const maps are the enforcement mechanism — the compiler or linter refuses an unknown value, not a reviewer

---

## Extensibility by design

- Adding a new variant (ATS adapter, job status, user role, UI page, pipeline stage) should require: one new file/type + one line in a registry or config map — nothing else
- Every "list of things" is a registry, config map, or enum — never a scattered set of `switch` branches or `if/else if` chains across files
- If adding a new feature requires editing more than 2 files outside the feature's own directory, there is a seam problem — fix the seam before adding the feature
- New behaviour via new types/functions, not by adding optional parameters to working code

---

## Test-first, edge-cases-first

- **TDD is the default** — write the test before the implementation. If a test cannot be written first, the design is not testable and must be rethought.
- **Every identified edge case must have a corresponding test** — a documented edge case with no test is a latent bug, not a note.
- The coverage gate (≥95% per package, see `CLAUDE.md`) is a floor, not a target. Tests exist to prevent regressions, not to hit a number.
- A test that is flaky, commented out, or marked skip/xfail without a written removal condition is worse than no test — it trains reviewers to ignore red.
- Integration and unit tests are distinct tiers; do not collapse them. Unit tests must be fast and deterministic; integration tests run against real infrastructure.

---

## Formatting is non-negotiable

- All code passes the project formatter before commit: `gofmt`/`goimports` for Go, Prettier for TypeScript/CSS
- Formatter settings live in project config files — never adjust them mid-task to make a check pass
- The pre-commit hook enforces formatting; it is not a suggestion
