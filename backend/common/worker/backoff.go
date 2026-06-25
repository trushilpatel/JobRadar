package worker

import "time"

// NATSWorkerMaxRetries is the maximum NATS delivery count before a job is
// routed to DLQ on the infra-error path. Matches MaxDeliver-1 on both the
// JOBS_RAW (filter) and JOBS_FILTERED (analyzer) consumers defined in
// common/jetstream/streams.go (MaxDeliver:3 → 2 Nak tiers before Term).
const NATSWorkerMaxRetries = 2

// NATSWorkerBackOff is the per-retry delay applied by the filter and analyzer
// workers. Mirrors the BackOff config on both JOBS_RAW and JOBS_FILTERED
// consumers so the ledger's next_attempt_at field stays meaningful.
// Must match BackOff in ConsumerFilterWorkers and ConsumerAnalyzerWorkers.
var NATSWorkerBackOff = []time.Duration{30 * time.Second, 2 * time.Minute}
