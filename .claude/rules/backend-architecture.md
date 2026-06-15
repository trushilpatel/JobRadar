---
paths:
  - 'backend/**/*.go'
---

# Backend Architecture — Service-Oriented Access

All data access goes through the service layer. No component queries the database directly.

## Layering rules

- **Handlers** call services only — never stores or GORM directly
- **Services** call stores only — never raw GORM or `db.Where(...)` etc.
- **Stores** are the only place SQL/GORM queries live
- **Workers / pipeline components** depend on service interfaces, never concrete types or stores

## What this means in practice

- If a handler needs data, it calls a service method
- If a service needs data, it calls a store method
- New DB operations always go in `store/` — one file per table
- Cross-table queries belong in the store of the primary table involved
- Never import a store package directly from a handler or worker

## Dependency injection

- All service and store dependencies are wired via interfaces in `wire.go`
- Concrete types are never referenced outside their own package except in wire setup

## SOLID — Go application

These are the hard rules from `engineering-principles.md` applied concretely to Go:

- **Single Responsibility** — one struct/service, one reason to change. Split when a service does unrelated things (e.g. scanning AND analytics).
- **Open/Closed** — new ATS providers, LLM backends, etc. are addable without touching existing ones. Extend via new implementations, not by modifying working code.
- **Liskov Substitution** — implementations of an interface must be fully interchangeable. Don't add methods to a concrete type that callers depend on — put them in the interface.
- **Interface Segregation** — keep interfaces small and focused. Prefer multiple small interfaces over one large one. Don't force a type to implement methods it doesn't need.
- **Dependency Inversion** — high-level components (workers, handlers) depend on abstractions (interfaces). Low-level components (stores, HTTP clients) implement those abstractions. Already enforced by the service layer rule — keep it that way.

**Practical reminders:** prefer composition over embedding; a function that does one thing beats a flexible one that does three; new behaviour via new types/functions, not optional parameters on existing ones.
