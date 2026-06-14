package main

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/trushilpatel/jobradar/common/ats/core"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

// TestProcessFetch_CoreJobSummary_DeserializeAndRepublish verifies the filter
// consumes the canonical core.JobSummary off jobs.raw.* and republishes it to
// jobs.filtered.* with job_content (and every other field) intact. This is the
// crawler-v2 contract: the analyzer downstream depends on job_content surviving.
func TestProcessFetch_CoreJobSummary_DeserializeAndRepublish(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"greenhouse-1"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	w := makeWorker(scorer, ledger, pub)

	in := core.JobSummary{
		ID:            "greenhouse-1",
		CompanyID:     "c-123",
		Adapter:       "greenhouse",
		Title:         "Software Engineer",
		Location:      "San Francisco, CA",
		URL:           "https://example.com/jobs/1",
		EmployType:    "full_time",
		LocationKnown: true,
		AllLocations:  []string{"San Francisco, CA"},
		JobContent: &core.JobContent{
			Description:   "Build things",
			Requirements:  "Go experience",
			Compensation:  "$200k",
			WorkplaceType: "remote",
			Department:    "Engineering",
			PublishedDate: "2026-06-01",
		},
	}
	payload, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	msg, state := js.NewTestMsg(payload, 1)

	if err := w.processFetch(context.Background(), []*js.Msg{msg}); err != nil {
		t.Fatalf("processFetch: %v", err)
	}
	if !state.Acked {
		t.Fatalf("expected message Ack'd after successful processing")
	}

	filtered := pub.recordsOnSubject("jobs.filtered.")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 publish on jobs.filtered.*, got %d", len(filtered))
	}
	if filtered[0].subject != "jobs.filtered.greenhouse" {
		t.Errorf("expected subject jobs.filtered.greenhouse, got %q", filtered[0].subject)
	}

	var out core.JobSummary
	if err := json.Unmarshal(filtered[0].value, &out); err != nil {
		t.Fatalf("unmarshal republished payload: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("republished JobSummary differs from input.\n got: %+v\nwant: %+v", out, in)
	}
	if out.JobContent == nil {
		t.Fatalf("job_content was dropped during republish")
	}
	if out.JobContent.Description != "Build things" {
		t.Errorf("job_content.description not preserved, got %q", out.JobContent.Description)
	}
}

// TestProcessChunk_WorkdayLocationKnownFalse_NotRejected is the crawler-v2 parity
// guard: a Workday job with LocationKnown=false and a clearly non-USA Location
// (Bengaluru, India) must NOT be routed to the non-USA reject path at the filter
// stage — the gate short-circuits on LocationKnown==false, so the job flows
// through to jobs.filtered.* and the analyzer decides later via country code.
func TestProcessChunk_WorkdayLocationKnownFalse_NotRejected(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"workday-1"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	w := makeWorker(scorer, ledger, pub)

	job := core.JobSummary{
		ID:            "workday-1",
		Adapter:       "workday",
		Title:         "Software Engineer",
		Location:      "Bengaluru, India",
		LocationKnown: false,
	}
	_ = w.processChunk(context.Background(), chunkOf(job), "fetch-1", 0, 1)

	for _, c := range ledger.calls {
		if c.reason == jobcommon.SeenReasonNonUSA {
			t.Fatalf("LocationKnown=false Workday job must not be rejected as non-USA; ledger=%+v", ledger.calls)
		}
	}
	filtered := pub.recordsOnSubject("jobs.filtered.")
	if len(filtered) != 1 {
		t.Fatalf("expected Workday job to pass through to jobs.filtered.*, got %d publishes", len(filtered))
	}
	if filtered[0].subject != "jobs.filtered.workday" {
		t.Errorf("expected subject jobs.filtered.workday, got %q", filtered[0].subject)
	}
}

// TestProcessChunk_LocationKnownTrue_NonUSA_Rejected is the inverse parity guard:
// when LocationKnown=true and AllLocations is entirely non-USA, the gate rejects.
func TestProcessChunk_LocationKnownTrue_NonUSA_Rejected(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}
	w := makeWorker(scorer, ledger, pub)

	job := core.JobSummary{
		ID:            "gh-uk-1",
		Adapter:       "greenhouse",
		Title:         "Software Engineer",
		Location:      "London, UK",
		LocationKnown: true,
		AllLocations:  []string{"London, UK"},
	}
	_ = w.processChunk(context.Background(), chunkOf(job), "fetch-1", 0, 1)

	if scorer.calls != 0 {
		t.Errorf("expected scorer NOT called for rejected non-USA job, got %d", scorer.calls)
	}
	if len(pub.recordsOnSubject("jobs.filtered.")) != 0 {
		t.Errorf("expected no filtered publishes for rejected non-USA job")
	}
	if len(ledger.calls) != 1 || ledger.calls[0].reason != jobcommon.SeenReasonNonUSA {
		t.Fatalf("expected exactly one non_usa_location ledger call, got %+v", ledger.calls)
	}
}
