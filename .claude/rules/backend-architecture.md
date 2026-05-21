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
