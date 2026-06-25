# ats/ — ATS registry: list, get, and patch entries from the `ats` table

## Decorator chain

```
RepositoryService → ImplService → CacheService → ValidationService → RestService → RestRouter
```

The `CacheService` sits between `ImplService` and `ValidationService` — one position inward from `user/` (which caches after validation). This is intentional: ATS data is global (not per-user) and read-heavy.

## Interface

`AtsService` — `ListATS`, `GetATS`, `UpdateATS`. All layers implement this interface.

## Key invariants

- The `ats` table is the canonical registry of adapter types. `company_scan_settings.adapter_type` and `company_scan_queue.adapter_type` both carry FK constraints referencing `ats(name)` — never insert raw adapter type strings elsewhere.
- The `CacheService` manages two key families: `"ats:list"` (full list) and `"ats:<name>"` (per-entry). `UpdateATS` deletes both keys before **and** after the write to prevent stale reads between the two evictions.
- A nil cache or `ttl <= 0` turns `AtsCacheService` into a pass-through — tests set a noop cache without changing the wire shape.
- The admin REST surface (`GET /admin/ats`, `PATCH /admin/ats/:name`) is the only mutation path. No user-facing write surface exists.
