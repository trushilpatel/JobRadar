package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/common/ats/core"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
)

// makeJobAt is like makeJob but lets the test set Location.
// LocationKnown is false (zero value) — use makeJobKnownAt when the location
// should be treated as structured/trusted for non-USA gating.
func makeJobAt(id, title, loc string) core.JobSummary {
	return core.JobSummary{ID: id, Title: title, Location: loc}
}

// makeJobKnownAt creates a core.JobSummary with LocationKnown=true and AllLocations set
// to the single supplied location string. Use this whenever the test intends
// the non-USA gate to evaluate the location.
func makeJobKnownAt(id, title, loc string) core.JobSummary {
	return core.JobSummary{
		ID:            id,
		Title:         title,
		Location:      loc,
		LocationKnown: true,
		AllLocations:  []string{loc},
	}
}

// makeJobKnownMultiAt creates a core.JobSummary with LocationKnown=true and multiple
// AllLocations. Use to test the "any USA wins" rule across multi-location jobs.
func makeJobKnownMultiAt(id, title string, locs ...string) core.JobSummary {
	loc := locs[0]
	if len(locs) > 1 {
		loc = strings.Join(locs, "; ")
	}
	return core.JobSummary{
		ID:            id,
		Title:         title,
		Location:      loc,
		LocationKnown: true,
		AllLocations:  locs,
	}
}

func chunkOf(jobs ...core.JobSummary) []chunkItem {
	out := make([]chunkItem, len(jobs))
	for i, j := range jobs {
		msg, _ := js.NewTestMsg([]byte("{}"), 1)
		out[i] = chunkItem{
			job: jobWithCtx{ctx: context.Background(), job: j},
			msg: msg,
		}
	}
	return out
}

// Non-USA jobs are rejected before the LLM scorer is called. The ledger
// receives reason='non_usa_location' for those IDs and the filtered subject
// receives no record.
func TestProcessChunk_NonUSALocation_RejectedBeforeLLM(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"us-1"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	_ = w.processChunk(context.Background(), chunkOf(
		makeJobKnownAt("us-1", "Software Engineer", "San Francisco, CA"),
		makeJobKnownAt("uk-1", "Software Engineer", "London, United Kingdom"),
		makeJobKnownAt("in-1", "Software Engineer", "Bangalore, India"),
	), "fetch-1", 0, 1)

	// Only one job (us-1) should have been scored.
	if scorer.calls != 1 {
		t.Errorf("expected scorer called once for the USA job, got %d calls", scorer.calls)
	}

	// Only one publish (us-1) on the filtered subject.
	filtered := pub.recordsOnSubject("jobs.filtered.")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 publish on filtered subject, got %d", len(filtered))
	}
	var pubJob core.JobSummary
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
// filtered subject stays empty, ledger gets exactly one non_usa_location batch.
func TestProcessChunk_AllNonUSA_SkipsLLMEntirely(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	_ = w.processChunk(context.Background(), chunkOf(
		makeJobKnownAt("uk-1", "SWE", "London"),
		makeJobKnownAt("ca-1", "SWE", "Toronto, Canada"),
		makeJobKnownAt("de-1", "SWE", "Berlin, Germany"),
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
	_ = w.processChunk(context.Background(), chunkOf(
		makeJobAt("u-1", "SWE", ""),
		makeJobAt("u-2", "SWE", "Remote"),
		makeJobAt("u-3", "SWE", "Worldwide"),
	), "fetch-1", 0, 1)

	if scorer.calls != 1 {
		t.Errorf("expected one ScoreBatch call covering the unknown-location jobs, got %d", scorer.calls)
	}
	if len(pub.recordsOnSubject("jobs.filtered.")) != 3 {
		t.Errorf("expected 3 jobs published to filtered subject, got %d",
			len(pub.recordsOnSubject("jobs.filtered.")))
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
	_ = w.processChunk(context.Background(), chunkOf(
		makeJobKnownAt("m-1", "SWE", "London, UK / New York, NY"),
	), "fetch-1", 0, 1)

	for _, c := range ledger.calls {
		if c.reason == jobcommon.SeenReasonNonUSA {
			t.Errorf("multi-location with USA part should NOT trigger non_usa_location: %+v", c)
		}
	}
	if scorer.calls != 1 {
		t.Errorf("expected ScoreBatch to be called for multi-location USA job, got %d", scorer.calls)
	}
	if len(pub.recordsOnSubject("jobs.filtered.")) != 1 {
		t.Errorf("expected 1 publish on filtered, got %d", len(pub.recordsOnSubject("jobs.filtered.")))
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
	_ = w.processChunk(context.Background(), chunkOf(
		makeJobKnownAt("uk-x", "", "London, UK"),
	), "fetch-1", 0, 1)

	// Filtered subject must be empty — the job was rejected, not passed through.
	if got := len(pub.recordsOnSubject("jobs.filtered.")); got != 0 {
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
	_ = w.processChunk(context.Background(), chunkOf(
		makeJobAt("pt-1", "", "Remote"),
	), "fetch-1", 0, 1)

	if got := len(pub.recordsOnSubject("jobs.filtered.")); got != 1 {
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

// ── AC-5: LocationKnown gate ─────────────────────────────────────────────────

// Jobs with LocationKnown=false bypass the non-USA gate entirely and reach
// the scorer even when Location contains a clearly non-USA string. This is the
// Workday/Meta thin-listing path where location is resolved at analyzer time.
func TestProcessChunk_LocationKnownFalse_BypassesNonUSAGate(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"wd-1"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	// LocationKnown=false (zero value via makeJobAt) + an explicitly non-USA string.
	_ = w.processChunk(context.Background(), chunkOf(
		makeJobAt("wd-1", "Software Engineer", "London, United Kingdom"),
	), "fetch-1", 0, 1)

	// Must NOT be rejected as non-USA.
	for _, c := range ledger.calls {
		if c.reason == jobcommon.SeenReasonNonUSA {
			t.Errorf("LocationKnown=false job must not be rejected as non-USA; ledger=%+v", ledger.calls)
		}
	}
	// Must reach the scorer.
	if scorer.calls != 1 {
		t.Errorf("expected scorer called once for LocationKnown=false job, got %d", scorer.calls)
	}
	// Must be published to filtered subject.
	if len(pub.recordsOnSubject("jobs.filtered.")) != 1 {
		t.Errorf("expected 1 filtered publish for LocationKnown=false job, got %d",
			len(pub.recordsOnSubject("jobs.filtered.")))
	}
}

// AllLocations slice is used for non-USA evaluation when LocationKnown=true.
// When AllLocations contains mixed USA+non-USA entries, "any USA wins" → keep.
func TestProcessChunk_AllLocations_AnyUSAWins(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"mixed-1"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	_ = w.processChunk(context.Background(), chunkOf(
		makeJobKnownMultiAt("mixed-1", "SWE", "London, UK", "New York, NY"),
	), "fetch-1", 0, 1)

	for _, c := range ledger.calls {
		if c.reason == jobcommon.SeenReasonNonUSA {
			t.Errorf("mixed USA+non-USA AllLocations must NOT be rejected; ledger=%+v", ledger.calls)
		}
	}
	if scorer.calls != 1 {
		t.Errorf("expected scorer called; got %d", scorer.calls)
	}
}

// When AllLocations is set with multiple non-USA entries and LocationKnown=true,
// AnyNonUSA returns true → rejected.
func TestProcessChunk_AllLocations_AllNonUSA_Rejected(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	_ = w.processChunk(context.Background(), chunkOf(
		makeJobKnownMultiAt("multi-uk-1", "SWE", "London, UK", "Berlin, Germany"),
	), "fetch-1", 0, 1)

	var sawNonUSA bool
	for _, c := range ledger.calls {
		if c.reason == jobcommon.SeenReasonNonUSA {
			sawNonUSA = true
		}
	}
	if !sawNonUSA {
		t.Errorf("expected non_usa_location for all-non-USA AllLocations job; ledger=%+v", ledger.calls)
	}
	if scorer.calls != 0 {
		t.Errorf("expected scorer NOT called; got %d", scorer.calls)
	}
}
