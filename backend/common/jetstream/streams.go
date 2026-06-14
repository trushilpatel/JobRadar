package jetstream

import (
	"context"
	"fmt"
	"time"

	natsjs "github.com/nats-io/nats.go/jetstream"
)

// EnsureStreams creates all pipeline streams and durable consumers.
// Safe to call on every startup — uses CreateOrUpdate semantics everywhere.
func EnsureStreams(ctx context.Context, js natsjs.JetStream) error {
	if err := ensureScanRequests(ctx, js); err != nil {
		return err
	}
	if err := ensureJobsRaw(ctx, js); err != nil {
		return err
	}
	if err := ensureJobsFiltered(ctx, js); err != nil {
		return err
	}
	return ensureJobsDLQ(ctx, js)
}

func ensureScanRequests(ctx context.Context, js natsjs.JetStream) error {
	if _, err := js.CreateOrUpdateStream(ctx, natsjs.StreamConfig{
		Name:       StreamScanRequests,
		Subjects:   []string{SubjectScanRequests},
		MaxAge:     48 * time.Hour,
		MaxBytes:   16 << 20, // 16 MB
		Storage:    natsjs.FileStorage,
		Retention:  natsjs.WorkQueuePolicy,
		Duplicates: 30 * time.Minute,
	}); err != nil {
		return fmt.Errorf("stream %s: %w", StreamScanRequests, err)
	}
	if _, err := js.CreateOrUpdateConsumer(ctx, StreamScanRequests, natsjs.ConsumerConfig{
		Name:       ConsumerOrchestratorWorkers,
		Durable:    ConsumerOrchestratorWorkers,
		AckPolicy:  natsjs.AckExplicitPolicy,
		AckWait:    15 * time.Minute,
		MaxDeliver: 3,
		BackOff:    []time.Duration{2 * time.Minute, 10 * time.Minute},
	}); err != nil {
		return fmt.Errorf("consumer %s: %w", ConsumerOrchestratorWorkers, err)
	}
	return nil
}

func ensureJobsRaw(ctx context.Context, js natsjs.JetStream) error {
	if _, err := js.CreateOrUpdateStream(ctx, natsjs.StreamConfig{
		Name:       StreamJobsRaw,
		Subjects:   []string{SubjectJobsRawWildcard},
		MaxAge:     7 * 24 * time.Hour,
		MaxBytes:   256 << 20, // 256 MB
		Storage:    natsjs.FileStorage,
		Retention:  natsjs.WorkQueuePolicy,
		Duplicates: 30 * time.Minute,
	}); err != nil {
		return fmt.Errorf("stream %s: %w", StreamJobsRaw, err)
	}
	if _, err := js.CreateOrUpdateConsumer(ctx, StreamJobsRaw, natsjs.ConsumerConfig{
		Name:       ConsumerFilterWorkers,
		Durable:    ConsumerFilterWorkers,
		AckPolicy:  natsjs.AckExplicitPolicy,
		AckWait:    10 * time.Minute,
		MaxDeliver: 3,
		BackOff:    []time.Duration{30 * time.Second, 2 * time.Minute},
	}); err != nil {
		return fmt.Errorf("consumer %s: %w", ConsumerFilterWorkers, err)
	}
	return nil
}

func ensureJobsFiltered(ctx context.Context, js natsjs.JetStream) error {
	if _, err := js.CreateOrUpdateStream(ctx, natsjs.StreamConfig{
		Name:       StreamJobsFiltered,
		Subjects:   []string{SubjectJobsFilteredWildcard},
		MaxAge:     7 * 24 * time.Hour,
		MaxBytes:   256 << 20, // 256 MB
		Storage:    natsjs.FileStorage,
		Retention:  natsjs.WorkQueuePolicy,
		Duplicates: 30 * time.Minute,
	}); err != nil {
		return fmt.Errorf("stream %s: %w", StreamJobsFiltered, err)
	}
	if _, err := js.CreateOrUpdateConsumer(ctx, StreamJobsFiltered, natsjs.ConsumerConfig{
		Name:       ConsumerAnalyzerWorkers,
		Durable:    ConsumerAnalyzerWorkers,
		AckPolicy:  natsjs.AckExplicitPolicy,
		AckWait:    5 * time.Minute,
		MaxDeliver: 3,
		BackOff:    []time.Duration{30 * time.Second, 2 * time.Minute},
	}); err != nil {
		return fmt.Errorf("consumer %s: %w", ConsumerAnalyzerWorkers, err)
	}
	return nil
}

func ensureJobsDLQ(ctx context.Context, js natsjs.JetStream) error {
	if _, err := js.CreateOrUpdateStream(ctx, natsjs.StreamConfig{
		Name:     StreamJobsDLQ,
		Subjects: []string{SubjectJobsDLQ},
		MaxAge:   30 * 24 * time.Hour,
		MaxBytes: 64 << 20, // 64 MB
		Storage:  natsjs.FileStorage,
	}); err != nil {
		return fmt.Errorf("stream %s: %w", StreamJobsDLQ, err)
	}
	return nil
}
