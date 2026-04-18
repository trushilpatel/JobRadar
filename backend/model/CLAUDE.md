# model/ — DB structs and typed constants

## What lives here

- **DB structs** — GORM model types that map directly to database tables
- **Constants** — all typed values for status, stage, reason, adapter, and queue fields

## DB structs

| File                  | Struct(s)        | Table                     |
| --------------------- | ---------------- | ------------------------- |
| `company.go`          | `Company`        | `companies`               |
| `job.go`              | `Job`            | `job_feed`                |
| `job_application.go`  | `JobApplication` | `job_application_tracker` |
| `job_snapshot.go`     | `JobSnapshot`    | `job_analytics`           |
| `job_tech_tag.go`     | `JobTechTag`     | `job_tech_tags`           |
| `seen_ledger.go`      | `JobSeenLedger`  | `job_pipeline_ledger`     |
| `company_scan_log.go` | `CompanyScanLog` | `company_scan_log`        |
| `discarded.go`        | `Discarded`      | `job_dismissal_log`       |

## Constant groups

| Group              | Constants                                                                                                                                                                   | Used for                                     |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------- |
| `AdapterType`      | `AdapterGreenhouse`, `AdapterLever`, `AdapterAshby`, `AdapterGoogle`, `AdapterMeta`, `AdapterNetflix`, `AdapterUber`, `AdapterRippling`, `AdapterMicrosoft`, `AdapterApple` | `companies.adapter_type` + ATS registry keys |
| `ScanStatus`       | `ScanStatusOK`, `ScanStatusError`, `ScanStatusPending`                                                                                                                      | `companies.last_scan_status`                 |
| `SeenReason`       | `SeenReasonFetch`, `SeenReasonLLMIrrelevant`, `SeenReasonUserDismissed`, `SeenReasonError`                                                                                  | `job_pipeline_ledger.reason`                 |
| `EmployType`       | `EmployFullTime`, `EmployPartTime`, `EmployContract`, `EmployInternship`                                                                                                    | `job_feed.employ_type`                       |
| `JobStatus`        | `JobStatusNew`, `JobStatusSaved`, `JobStatusApplied`, `JobStatusInterviewing`, `JobStatusOffer`, `JobStatusDismissed`                                                       | `job_feed.status`                            |
| `ApplicationStage` | `StageApplied`, `StagePhoneScreen`, `StageTechnical`, `StageFinalRound`, `StageOffer`, `StageRejected`                                                                      | `job_application_tracker.stage`              |
| `VelocityTag`      | `VelocitySpike`, `VelocityRising`, `VelocityNormal`                                                                                                                         | insights velocity calculation                |
| `DiscardReason`    | `DiscardReasonDismissed`, `DiscardReasonNotRelevant`                                                                                                                        | `job_dismissal_log.reason`                   |
| `QueueStatus`      | `QueueStatusQueued`, `QueueStatusProcessing`                                                                                                                                | `company_scan_queue.status`                  |
| `QueueSource`      | `QueueSourceScheduler`, `QueueSourceManual`                                                                                                                                 | `company_scan_queue.source`                  |

## Rules

- Always use constants — never raw strings for any constrained column value
- All SQL `CHECK` constraints in migrations must have a matching constant here
- When adding a new adapter: add the constant here AND add the CHECK constraint value to the relevant migration
- DB structs use GORM tags only — do not add business logic to model types

## model/ vs domain/

| Put in `model/`                | Put in `domain/`                                |
| ------------------------------ | ----------------------------------------------- |
| DB structs (GORM models)       | Error types (`ServiceError`, `ValidationError`) |
| Typed constants for DB columns | Sentinel error values (`ErrNotFound`, etc.)     |
