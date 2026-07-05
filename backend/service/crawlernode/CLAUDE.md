# crawlernode/ — Crawler-node orchestration: work assignment, heartbeat, result submission, and node provisioning

## Wire status

`wire.go` is an **empty stub** (`package crawlernode` — no `ProviderSet`). The crawlernode slice is wired directly in `apps/orchestrator/wire.go`. Do not add a `ProviderSet` here without coordinating with the orchestrator wiring.

## Decorator chain

```
RepositoryService (NodeStore) → ImplService → ValidationService → RestService → RestRouter

Separate stores (not in the main chain):
  AssignmentRepositoryService  (AssignmentStore)
  LedgerRepositoryService      (LedgerStore)
  ScanLogRepositoryService     (ScanLogStore)
  ScanSettingsRepositoryService (ScanSettingsReader)
```

The admin surface is a separate `CrawlerNodeAdminRestService` + `CrawlerNodeAdminRestRouter` wired outside the main decorator chain (same pattern as `audit/`).

## Interfaces

| Interface            | Declared in             | Purpose                                                                                                                                     |
| -------------------- | ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `NodeService`        | `CrawlerNodeService.go` | Main contract for the entire chain (node-facing + admin endpoints)                                                                          |
| `NodeStore`          | `CrawlerNodeService.go` | CRUD on `crawler_nodes` table + auth lookup                                                                                                 |
| `AssignmentStore`    | `CrawlerNodeService.go` | `company_scan_queue` in node-claiming mode (`ClaimNextForNode`, `ExtendHeartbeats`, `CompleteForNode`, `FailForNode`, `ResetStuckForNodes`) |
| `LedgerStore`        | `CrawlerNodeService.go` | Narrow `job_pipeline_ledger` facade for the orchestrator (dedup reads/writes)                                                               |
| `ScanLogStore`       | `CrawlerNodeService.go` | Writes scan outcomes to `company_scan_log`                                                                                                  |
| `StreamPublisher`    | `CrawlerNodeService.go` | NATS JetStream publish with dedup (`PublishWithMsgID`)                                                                                      |
| `ScanSettingsReader` | `CrawlerNodeService.go` | Reads `adapter_config` per company (provided by the scan/company slices)                                                                    |

## Key invariants

- `AssignmentStore.ClaimNextForNode` lists up to 20 queued/due candidates with a plain (unlocked) SELECT, then walks them in order: `AdapterDelayGate.TryReserve` (see below) skips any candidate whose adapter type is still on its `ats.inter_company_delay_s` cooldown, and a targeted `UPDATE ... WHERE id = ? AND status = 'queued'` performs the actual claim. Losing that UPDATE (0 rows — another claimant or `ResetStuckForNodes` won first) is not fatal: the walk continues to the next candidate. Safety against double-claiming comes from the targeted per-row UPDATE's `RowsAffected` check, not from locking the listing SELECT — there is deliberately no `FOR UPDATE SKIP LOCKED` in this path anymore.
- `AdapterDelayGate` (`AdapterDelayGate.go`) is a process-local, mutex-guarded `map[string]time.Time` that enforces `ats.inter_company_delay_s` across every claim this orchestrator process handles, backed by the existing `AtsService` (reuses its 5-minute `MemoryCache` — never build a second cache for this). A lookup failure (unknown adapter, `AtsService` error) is treated as NOT eligible — fail safe, never "always eligible" on a lookup failure.
- `ExtendHeartbeats` returns `acked` and `missed` IDs separately. `missed` means `locked_by` no longer matches (reclaimed by `ResetStuckForNodes`); the node must abort those assignments immediately.
- `CompleteForNode` and `FailForNode` both return `ErrQueueRowMissing` when `locked_by != nodeID` — never silently no-op on ownership mismatch.
- `LedgerStore.MarkSeenBatch` must be called **before** `StreamPublisher.PublishWithMsgID` (ledger-before-publish invariant). This ensures the ledger row exists before the NATS message can be consumed by the filter worker.
- Node tokens are stored as HMAC-SHA256 hashes in `crawler_nodes.token_hash`; the raw token is returned exactly once at provisioning and never stored. `RetireNode` clears `token_hash` atomically.
