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

- `AssignmentStore.ClaimNextForNode` uses `FOR UPDATE SKIP LOCKED` — never remove this; concurrent nodes must not claim the same row.
- `ExtendHeartbeats` returns `acked` and `missed` IDs separately. `missed` means `locked_by` no longer matches (reclaimed by `ResetStuckForNodes`); the node must abort those assignments immediately.
- `CompleteForNode` and `FailForNode` both return `ErrQueueRowMissing` when `locked_by != nodeID` — never silently no-op on ownership mismatch.
- `LedgerStore.MarkSeenBatch` must be called **before** `StreamPublisher.PublishWithMsgID` (ledger-before-publish invariant). This ensures the ledger row exists before the NATS message can be consumed by the filter worker.
- Node tokens are stored as HMAC-SHA256 hashes in `crawler_nodes.token_hash`; the raw token is returned exactly once at provisioning and never stored. `RetireNode` clears `token_hash` atomically.
