package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	"github.com/trushilpatel/jobradar/common/kafka"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

// makeJobAt is like makeJob but lets the test set Location.
func makeJobAt(id, title, loc string) ats.RawJob {
	return ats.RawJob{ID: id, Title: title, Location: loc}
}

func chunkOf(jobs ...ats.RawJob) []chunkItem {
	out := make([]chunkItem, len(jobs))
	for i, j := range jobs {
		out[i] = chunkItem{
			job:    jobWithCtx{ctx: context.Background(), job: j},
			record: &kgo.Record{Partition: 0, Offset: int64(i)},
		}
	}
	return out
}

// Non-USA jobs are rejected before the LLM scorer is called. The ledger
// receives reason='non_usa_location' for those IDs and the filtered topic
// receives no record.
func TestProcessChunk_NonUSALocation_RejectedBeforeLLM(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"us-1"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.processChunk(context.Background(), chunkOf(
		makeJobAt("us-1", "Software Engineer", "San Francisco, CA"),
		makeJobAt("uk-1", "Software Engineer", "London, United Kingdom"),
		makeJobAt("in-1", "Software Engineer", "Bangalore, India"),
	), "fetch-1", 0, 1)

	// Only one job (us-1) should have been scored.
	if scorer.calls != 1 {
		t.Errorf("expected scorer called once for the USA job, got %d calls", scorer.calls)
	}

	// Only one publish (us-1) on the filtered topic.
	filtered := pub.recordsOnTopic(kafka.TopicJobListingsFiltered)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 publish on filtered topic, got %d", len(filtered))
	}
	var pubJob ats.RawJob
	if err := json.Unmarshal(filtered[0].value, &pubJob); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pubJob.ID != "us-1" {
		t.Errorf("expected filtered publish for us-1, got %s", pubJob.ID)
	}

	// One ledger call with reason='non_usa_location' covering both non-USA IDs.
	var sawNonUSA bool
	for _, c := range ledger.calls {
		if c.reason == jobcommon.SeenReasonNonUSA {
			sawNonUSA = true
			if len(c.ids) != 2 {
				t.Errorf("expected 2 non-USA IDs marked, got %d (%v)", len(c.ids), c.ids)
			}
			got := map[string]bool{}
			for _, id := range c.ids {
				got[id] = true
			}
			if !got["uk-1"] || !got["in-1"] {
				t.Errorf("expected uk-1 and in-1 marked, got %v", c.ids)
			}
		}
	}
	if !sawNonUSA {
		t.Errorf("expected a ledger call with reason=%q, calls=%+v", jobcommon.SeenReasonNonUSA, ledger.calls)
	}
}

// A chunk where every job is non-USA: scorer must not be called at all, the
// filtered topic stays empty, ledger gets exactly one non_usa_location batch.
func TestProcessChunk_AllNonUSA_SkipsLLMEntirely(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.processChunk(context.Background(), chunkOf(
		makeJobAt("uk-1", "SWE", "London"),
		makeJobAt("ca-1", "SWE", "Toronto, Canada"),
		makeJobAt("de-1", "SWE", "Berlin, Germany"),
	), "fetch-1", 0, 1)

	if scorer.calls != 0 {
		t.Errorf("expected scorer NOT called when all jobs are non-USA, got %d calls", scorer.calls)
	}
	if len(pub.records) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.records))
	}
	if len(ledger.calls) != 1 {
		t.Fatalf("expected exactly 1 ledger call, got %d (%+v)", len(ledger.calls), ledger.calls)
	}
	if ledger.calls[0].reason != jobcommon.SeenReasonNonUSA {
		t.Errorf("expected reason=%q, got %q", jobcommon.SeenReasonNonUSA, ledger.calls[0].reason)
	}
	if len(ledger.calls[0].ids) != 3 {
		t.Errorf("expected 3 IDs in non-USA batch, got %d", len(ledger.calls[0].ids))
	}
}

// Unknown / empty locations are NOT rejected — they go to the LLM scorer.
// This preserves the "never reject on missing signal" invariant.
func TestProcessChunk_UnknownLocation_ReachesLLM(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"u-1", "u-2", "u-3"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.processChunk(context.Background(), chunkOf(
		makeJobAt("u-1", "SWE", ""),
		makeJobAt("u-2", "SWE", "Remote"),
		makeJobAt("u-3", "SWE", "Worldwide"),
	), "fetch-1", 0, 1)

	if scorer.calls != 1 {
		t.Errorf("expected one ScoreBatch call covering the unknown-location jobs, got %d", scorer.calls)
	}
	if len(pub.recordsOnTopic(kafka.TopicJobListingsFiltered)) != 3 {
		t.Errorf("expected 3 jobs published to filtered topic, got %d",
			len(pub.recordsOnTopic(kafka.TopicJobListingsFiltered)))
	}
	for _, c := range ledger.calls {
		if c.reason == jobcommon.SeenReasonNonUSA {
			t.Errorf("did NOT expect non-USA ledger entry for unknown locations, got: %+v", c)
		}
	}
}

// Multi-location strings with at least one USA part keep the whole job; they
// must reach the LLM scorer, not the non-USA reject path.
func TestProcessChunk_MultiLocationWithUSA_NotRejected(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"m-1"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.processChunk(context.Background(), chunkOf(
		makeJobAt("m-1", "SWE", "London, UK / New York, NY"),
	), "fetch-1", 0, 1)

	for _, c := range ledger.calls {
		if c.reason == jobcommon.SeenReasonNonUSA {
			t.Errorf("multi-location with USA part should NOT trigger non_usa_location: %+v", c)
		}
	}
	if scorer.calls != 1 {
		t.Errorf("expected ScoreBatch to be called for multi-location USA job, got %d", scorer.calls)
	}
	if len(pub.recordsOnTopic(kafka.TopicJobListingsFiltered)) != 1 {
		t.Errorf("expected 1 publish on filtered, got %d", len(pub.recordsOnTopic(kafka.TopicJobListingsFiltered)))
	}
}

// A non-USA job WITHOUT a title still gets rejected at the partition step —
// the non-USA check runs before the "no title → passthrough" check, so we
// don't waste an enricher fetch on a clearly non-USA job.
func TestProcessChunk_NonUSAWithoutTitle_RejectedNotPassedThrough(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.processChunk(context.Background(), chunkOf(
		makeJobAt("uk-x", "", "London, UK"),
	), "fetch-1", 0, 1)

	// Filtered topic must be empty — the job was rejected, not passed through.
	if got := len(pub.recordsOnTopic(kafka.TopicJobListingsFiltered)); got != 0 {
		t.Errorf("expected no filtered publishes for non-USA no-title job, got %d", got)
	}
	if len(ledger.calls) != 1 || ledger.calls[0].reason != jobcommon.SeenReasonNonUSA {
		t.Errorf("expected ledger marked non_usa_location, got %+v", ledger.calls)
	}
	if scorer.calls != 0 {
		t.Errorf("expected scorer not called, got %d", scorer.calls)
	}
}

// A no-title job with an *unknown* (non-rejecting) location still goes
// through the passthrough path — regression guard for the partition order.
func TestProcessChunk_NoTitleUnknownLocation_PassesThrough(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.processChunk(context.Background(), chunkOf(
		makeJobAt("pt-1", "", "Remote"),
	), "fetch-1", 0, 1)

	if got := len(pub.recordsOnTopic(kafka.TopicJobListingsFiltered)); got != 1 {
		t.Errorf("expected 1 passthrough publish, got %d", got)
	}
	for _, c := range ledger.calls {
		if c.reason == jobcommon.SeenReasonNonUSA {
			t.Errorf("did NOT expect non-USA ledger entry for unknown-location passthrough: %+v", c)
		}
	}
	if scorer.calls != 0 {
		t.Errorf("expected scorer not called (no scorable jobs), got %d", scorer.calls)
	}
}
