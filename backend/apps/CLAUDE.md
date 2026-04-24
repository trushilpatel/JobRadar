# apps/ — Runnable binaries

Each subdirectory is a self-contained app: one `main.go` entry point, its worker logic, and any adapters it owns exclusively.

## Rules

- No business logic in `main.go` — wiring only (open DB, build deps, start worker)
- Workers depend on **service interfaces**, never on concrete store types or GORM
- Each app owns its pipeline logic inline (`package main`) — no separate pipeline packages
- Shared domain logic lives in `service/` — apps import from there, never the reverse
