package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	atstypes "github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/kafka"
	"github.com/trushilpatel/jobradar/common/logger"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	scandto "github.com/trushilpatel/jobradar/service/scan/dto"
	"github.com/twmb/franz-go/pkg/kgo"
)

// fakeSettingsReader is a test double for ScanSettingsReader.
type fakeSettingsReader struct {
	settings *companymodel.CompanyScanSettings
	err      error
	calls    int
}

func (f *fakeSettingsReader) Get(_ context.Context, _ string) (*companymodel.CompanyScanSettings, error) {
	f.calls++
	return f.settings, f.err
}

// fakeLedger satisfies PipelineLedger but no method should be reached when
// the engine takes the graceful-skip path.
type fakeLedger struct {
	mu                  sync.Mutex
	called              bool
	activeIDs           []string
	activeErr           error
	seenMap             map[string]bool
	seenErr             error
	markSeenErr         error
	markRemovedErr      error
	markSeenCalls       [][]string
	markRemovedCalls    [][]string
	hasSeenBatchCalls   int
	markSeenLastReason  string
	markSeenLastCompany string
}

func (f *fakeLedger) HasSeenBatch(_ context.Context, ids []string) (map[string]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = true
	f.hasSeenBatchCalls++
	if f.seenErr != nil {
		return nil, f.seenErr
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = f.seenMap[id]
	}
	return out, nil
}
func (f *fakeLedger) MarkSeenBatch(_ context.Context, ids []string, companyID, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = true
	f.markSeenCalls = append(f.markSeenCalls, append([]string(nil), ids...))
	f.markSeenLastReason = reason
	f.markSeenLastCompany = companyID
	return f.markSeenErr
}
func (f *fakeLedger) GetActiveJobIDsByCompany(_ context.Context, _ string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = true
	if f.activeErr != nil {
		return nil, f.activeErr
	}
	return f.activeIDs, nil
}
func (f *fakeLedger) MarkRemovedBatch(_ context.Context, ids []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = true
	f.markRemovedCalls = append(f.markRemovedCalls, append([]string(nil), ids...))
	return f.markRemovedErr
}

// fakeRecorder satisfies ScanRecorder; tracks whether RecordScanResult fired.
type fakeRecorder struct {
	mu      sync.Mutex
	called  bool
	results []scandto.ScanResult
	err     error
}

func (f *fakeRecorder) RecordScanResult(_ context.Context, _ string, result scandto.ScanResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = true
	f.results = append(f.results, result)
	return f.err
}

// fakePublisher satisfies kafka.Publisher; tracks whether Publish fired.
type fakePublisher struct {
	mu       sync.Mutex
	called   bool
	messages [][]byte
	topic    string
	err      error
	// failOn allows the publisher to fail for specific job IDs (matched by
	// JSON substring on `"id":"<value>"`).
	failOn map[string]error
}

func (f *fakePublisher) Publish(_ context.Context, topic string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = true
	f.topic = topic
	// Per-id failure simulation — used to verify the engine retries the next
	// job after a single publish failure without bubbling the error up.
	if f.failOn != nil {
		var raw map[string]any
		_ = json.Unmarshal(payload, &raw)
		if id, ok := raw["id"].(string); ok {
			if e, exists := f.failOn[id]; exists {
				return e
			}
		}
	}
	if f.err != nil {
		return f.err
	}
	f.messages = append(f.messages, append([]byte(nil), payload...))
	return nil
}

func (f *fakePublisher) PublishWithHeaders(_ context.Context, _ string, _ []byte, _ []kgo.RecordHeader) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = true
	return nil
}

var _ kafka.Publisher = (*fakePublisher)(nil)

func newTestEngine(settings *fakeSettingsReader, ledger *fakeLedger, rec *fakeRecorder, pub *fakePublisher) *Engine {
	return NewEngine(settings, ledger, rec, pub, google.Config{}, logger.NewNoop())
}

// ── Stub adapters for engine-level tests ─────────────────────────────────────

// stubAdapter is the minimal ats.Adapter — returns a fixed slice of items.
type stubAdapter struct {
	jobs []ats.FetchJobsItem
	err  error
	// observed contains what FetchJobs received — used to verify routing.
	observedCompanyID string
}

func (s *stubAdapter) FetchJobs(_ context.Context, companyID string) (ats.FetchJobsResponse, error) {
	s.observedCompanyID = companyID
	if s.err != nil {
		return ats.FetchJobsResponse{}, s.err
	}
	return ats.FetchJobsResponse{Jobs: s.jobs}, nil
}

// streamingStubAdapter additionally implements StreamPublisher + SeenChecker —
// it publishes its jobs through the injected stream callback rather than
// returning them from FetchJobs.
type streamingStubAdapter struct {
	jobs          []ats.FetchJobsItem
	streamPublish func([]ats.FetchJobsItem)
	seenChecker   func(ctx context.Context, ids []string) (map[string]bool, error)
	known         map[string]struct{}
}

func (s *streamingStubAdapter) FetchJobs(ctx context.Context, _ string) (ats.FetchJobsResponse, error) {
	if s.streamPublish != nil {
		s.streamPublish(s.jobs)
	}
	if s.seenChecker != nil {
		// Touch the seen-checker so the engine code path that calls
		// SetSeenChecker is exercised.
		ids := make([]string, len(s.jobs))
		for i, j := range s.jobs {
			ids[i] = j.ID
		}
		_, _ = s.seenChecker(ctx, ids)
	}
	return ats.FetchJobsResponse{Jobs: s.jobs}, nil
}

func (s *streamingStubAdapter) SetStreamPublish(fn func([]ats.FetchJobsItem)) {
	s.streamPublish = fn
}
func (s *streamingStubAdapter) SetSeenChecker(fn func(ctx context.Context, ids []string) (map[string]bool, error)) {
	s.seenChecker = fn
}
func (s *streamingStubAdapter) SetKnownJobIDs(ids map[string]struct{}) {
	s.known = ids
}

// TestEngine_RunForCompany_NoScanSettings verifies the engine skips a company
// whose scan_settings row is missing (post-cutover un-onboarded company) without
// invoking the ledger, recorder, or publisher. Crawler workers complete the
// queue entry for skipped rows so the queue never loops.
func TestEngine_RunForCompany_NoScanSettings(t *testing.T) {
	settings := &fakeSettingsReader{err: apperr.ErrNotFound.WithMessage("scan settings not found")}
	ledger := &fakeLedger{}
	rec := &fakeRecorder{}
	pub := &fakePublisher{}

	e := newTestEngine(settings, ledger, rec, pub)
	e.RunForCompany(context.Background(), "stripe")

	if settings.calls != 1 {
		t.Errorf("expected 1 settings.Get call, got %d", settings.calls)
	}
	if ledger.called {
		t.Error("ledger must not be called when scan_settings missing")
	}
	if rec.called {
		t.Error("recorder must not be called when scan_settings missing")
	}
	if pub.called {
		t.Error("publisher must not be called when scan_settings missing")
	}
}

// TestEngine_RunForCompany_Inactive verifies the engine skips a company with
// active=false on its scan_settings.
func TestEngine_RunForCompany_Inactive(t *testing.T) {
	settings := &fakeSettingsReader{
		settings: &companymodel.CompanyScanSettings{CompanyID: "stripe", AdapterType: "greenhouse", Active: false},
	}
	ledger := &fakeLedger{}
	rec := &fakeRecorder{}
	pub := &fakePublisher{}

	e := newTestEngine(settings, ledger, rec, pub)
	e.RunForCompany(context.Background(), "stripe")

	if ledger.called || rec.called || pub.called {
		t.Error("inactive companies must not invoke ledger / recorder / publisher")
	}
}

// TestEngine_RunForCompany_LookupError verifies the engine logs and returns
// without invoking adapters on a generic lookup error.
func TestEngine_RunForCompany_LookupError(t *testing.T) {
	settings := &fakeSettingsReader{err: errors.New("db down")}
	ledger := &fakeLedger{}
	rec := &fakeRecorder{}
	pub := &fakePublisher{}

	e := newTestEngine(settings, ledger, rec, pub)
	e.RunForCompany(context.Background(), "stripe")

	if ledger.called || rec.called || pub.called {
		t.Error("lookup error must not trigger downstream calls")
	}
}

// ── toRawJob — pure conversion ───────────────────────────────────────────────

// TestToRawJob_PropagatesAllFields verifies the engine's listing→RawJob mapper
// carries every field through unchanged. The downstream filter assumes RawJob
// fields are populated; any drop here surfaces as missing data in Kafka.
func TestToRawJob_PropagatesAllFields(t *testing.T) {
	full := &atstypes.FullJobContent{Description: "desc"}
	item := ats.FetchJobsItem{
		ID:             "greenhouse-1",
		Title:          "SWE",
		Location:       "Remote",
		URL:            "https://example.com/jobs/1",
		EmploymentType: "full_time",
		FullContent:    full,
	}
	raw := toRawJob(item, "company-uuid", "greenhouse")
	if raw.ID != "greenhouse-1" || raw.CompanyID != "company-uuid" || raw.Adapter != "greenhouse" {
		t.Errorf("identifier fields wrong: %+v", raw)
	}
	if raw.Title != "SWE" || raw.Location != "Remote" || raw.URL != "https://example.com/jobs/1" {
		t.Errorf("metadata fields wrong: %+v", raw)
	}
	if raw.EmployType != "full_time" {
		t.Errorf("EmployType: got %q, want full_time", raw.EmployType)
	}
	if raw.FullContent != full {
		t.Errorf("FullContent must be carried by reference (pointer identity)")
	}
}

// TestToRawJob_NilFullContent verifies the thin-adapter path: FullContent
// stays nil, matching the omitempty wire contract on RawJob.
func TestToRawJob_NilFullContent(t *testing.T) {
	item := ats.FetchJobsItem{ID: "lever-1", FullContent: nil}
	raw := toRawJob(item, "co", "lever")
	if raw.FullContent != nil {
		t.Errorf("nil FullContent must stay nil; got %+v", raw.FullContent)
	}
}

// ── publish (private kafka wrapper) ──────────────────────────────────────────

// TestPublish_MarshalsAndSendsToRawTopic verifies publish marshals the RawJob
// and routes it to the canonical raw topic. Kafka topic constant lives in
// common/kafka; the engine must use exactly that — a typo would silently
// route to a dead topic.
func TestPublish_MarshalsAndSendsToRawTopic(t *testing.T) {
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, &fakeLedger{}, &fakeRecorder{}, pub)
	job := ats.RawJob{ID: "g-1", CompanyID: "co", Adapter: "greenhouse"}

	if err := e.publish(context.Background(), job); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if pub.topic != kafka.TopicJobListingsRaw {
		t.Errorf("topic: got %q, want %q", pub.topic, kafka.TopicJobListingsRaw)
	}
	if len(pub.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(pub.messages))
	}
	var decoded ats.RawJob
	if err := json.Unmarshal(pub.messages[0], &decoded); err != nil {
		t.Fatalf("decode publish payload: %v", err)
	}
	if decoded.ID != "g-1" || decoded.CompanyID != "co" {
		t.Errorf("decoded payload mismatch: %+v", decoded)
	}
}

// TestPublish_PropagatesPublisherError verifies publish surfaces the kafka
// publisher's error verbatim — wrappers higher up rely on the underlying
// error to drive retries / metrics.
func TestPublish_PropagatesPublisherError(t *testing.T) {
	pub := &fakePublisher{err: errors.New("broker down")}
	e := newTestEngine(&fakeSettingsReader{}, &fakeLedger{}, &fakeRecorder{}, pub)
	err := e.publish(context.Background(), ats.RawJob{ID: "g-1"})
	if err == nil {
		t.Fatal("expected error from publish when broker fails")
	}
}

// ── publishNewJobs — dedup, partial-failure, mark-seen-after-publish ────────

// TestPublishNewJobs_EmptyInput verifies the no-op early return — saves a DB
// round-trip on empty pages (Google's last page often returns 0 cards).
func TestPublishNewJobs_EmptyInput(t *testing.T) {
	pub := &fakePublisher{}
	ledger := &fakeLedger{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), nil, "co", "greenhouse", logger.NewNoop())
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
	if pub.called {
		t.Error("publisher must not be called for empty input")
	}
	if ledger.called {
		t.Error("ledger must not be called for empty input")
	}
}

// TestPublishNewJobs_SkipsSeenJobs verifies seen-batch result is honoured —
// already-known jobs do not re-enter Kafka or re-mark the ledger.
func TestPublishNewJobs_SkipsSeenJobs(t *testing.T) {
	jobs := []ats.FetchJobsItem{
		{ID: "g-1"}, {ID: "g-2"}, {ID: "g-3"},
	}
	ledger := &fakeLedger{seenMap: map[string]bool{"g-1": true, "g-2": false, "g-3": true}}
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), jobs, "co", "greenhouse", logger.NewNoop())
	if n != 1 {
		t.Fatalf("expected 1 new job, got %d", n)
	}
	if len(pub.messages) != 1 {
		t.Errorf("expected 1 publish, got %d", len(pub.messages))
	}
	if len(ledger.markSeenCalls) != 1 || len(ledger.markSeenCalls[0]) != 1 || ledger.markSeenCalls[0][0] != "g-2" {
		t.Errorf("expected MarkSeenBatch with [g-2], got %v", ledger.markSeenCalls)
	}
	if ledger.markSeenLastReason != "fetch" {
		t.Errorf("MarkSeenBatch reason: got %q, want fetch", ledger.markSeenLastReason)
	}
}

// TestPublishNewJobs_AllSeen verifies the early return when seen-check
// returns no new jobs — no publish, no mark-seen.
func TestPublishNewJobs_AllSeen(t *testing.T) {
	jobs := []ats.FetchJobsItem{{ID: "g-1"}, {ID: "g-2"}}
	ledger := &fakeLedger{seenMap: map[string]bool{"g-1": true, "g-2": true}}
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), jobs, "co", "greenhouse", logger.NewNoop())
	if n != 0 {
		t.Errorf("expected 0 new, got %d", n)
	}
	if pub.called {
		t.Error("publisher must not be called when all jobs already seen")
	}
	if len(ledger.markSeenCalls) != 0 {
		t.Error("MarkSeenBatch must not be called when no new jobs")
	}
}

// TestPublishNewJobs_HasSeenError verifies the engine logs and returns 0
// when the seen-batch DB call fails. Critical: a flapping DB must not flood
// Kafka with re-publishes.
func TestPublishNewJobs_HasSeenError(t *testing.T) {
	jobs := []ats.FetchJobsItem{{ID: "g-1"}}
	ledger := &fakeLedger{seenErr: errors.New("db down")}
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), jobs, "co", "greenhouse", logger.NewNoop())
	if n != 0 {
		t.Errorf("expected 0 on seen-batch error, got %d", n)
	}
	if pub.called {
		t.Error("publisher must not be called when seen-batch errors")
	}
}

// TestPublishNewJobs_ContinuesPastPublishFailure verifies that a single job
// publish failure does not abort the batch — the next job still ships, and
// only successfully-published IDs make it into MarkSeenBatch.
func TestPublishNewJobs_ContinuesPastPublishFailure(t *testing.T) {
	jobs := []ats.FetchJobsItem{{ID: "g-1"}, {ID: "g-2"}, {ID: "g-3"}}
	ledger := &fakeLedger{seenMap: map[string]bool{}}
	pub := &fakePublisher{failOn: map[string]error{"g-2": errors.New("transient")}}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), jobs, "co", "greenhouse", logger.NewNoop())
	if n != 2 {
		t.Errorf("expected 2 published (g-1, g-3), got %d", n)
	}
	if len(ledger.markSeenCalls) != 1 {
		t.Fatalf("expected one MarkSeenBatch call, got %d", len(ledger.markSeenCalls))
	}
	got := ledger.markSeenCalls[0]
	if len(got) != 2 || got[0] != "g-1" || got[1] != "g-3" {
		t.Errorf("MarkSeenBatch IDs: got %v, want [g-1 g-3]", got)
	}
}

// TestPublishNewJobs_AllPublishesFail verifies that when every publish
// fails, the engine does not call MarkSeenBatch — the next scan will
// retry these jobs verbatim.
func TestPublishNewJobs_AllPublishesFail(t *testing.T) {
	jobs := []ats.FetchJobsItem{{ID: "g-1"}, {ID: "g-2"}}
	ledger := &fakeLedger{seenMap: map[string]bool{}}
	pub := &fakePublisher{err: errors.New("broker down")}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), jobs, "co", "greenhouse", logger.NewNoop())
	if n != 0 {
		t.Errorf("expected 0 when all publishes fail, got %d", n)
	}
	if len(ledger.markSeenCalls) != 0 {
		t.Errorf("MarkSeenBatch must not be called when no publish succeeded; got %v", ledger.markSeenCalls)
	}
}

// TestPublishNewJobs_MarkSeenErrorDoesNotFail verifies the engine treats a
// MarkSeenBatch failure as a soft error — jobs are in Kafka, the next scan
// will republish, the downstream filter dedupes by ID.
func TestPublishNewJobs_MarkSeenErrorDoesNotFail(t *testing.T) {
	jobs := []ats.FetchJobsItem{{ID: "g-1"}}
	ledger := &fakeLedger{seenMap: map[string]bool{}, markSeenErr: errors.New("ledger flaky")}
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), jobs, "co", "greenhouse", logger.NewNoop())
	// Engine returns the number of published IDs even when the post-publish
	// mark-seen call fails — the publish itself succeeded.
	if n != 1 {
		t.Errorf("expected 1 (jobs reached Kafka), got %d", n)
	}
}

// ── detectRemovedJobs ────────────────────────────────────────────────────────

// TestDetectRemovedJobs_NoPrevious verifies first-scan behaviour: when there
// are no previously-known IDs, nothing is marked removed.
func TestDetectRemovedJobs_NoPrevious(t *testing.T) {
	ledger := &fakeLedger{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, &fakePublisher{})
	e.detectRemovedJobs(context.Background(), []ats.FetchJobsItem{{ID: "g-1"}}, nil, logger.NewNoop())
	if len(ledger.markRemovedCalls) != 0 {
		t.Errorf("MarkRemovedBatch must not be called when previousIDs is nil")
	}
}

// TestDetectRemovedJobs_AllStillActive verifies no-op when current scan
// covers every previously-known ID.
func TestDetectRemovedJobs_AllStillActive(t *testing.T) {
	ledger := &fakeLedger{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, &fakePublisher{})
	prev := []string{"g-1", "g-2"}
	curr := []ats.FetchJobsItem{{ID: "g-1"}, {ID: "g-2"}}
	e.detectRemovedJobs(context.Background(), curr, prev, logger.NewNoop())
	if len(ledger.markRemovedCalls) != 0 {
		t.Errorf("MarkRemovedBatch must not be called when nothing disappeared")
	}
}

// TestDetectRemovedJobs_MarksMissingIDs verifies the engine reports IDs that
// were in the ledger but absent from the current scan.
func TestDetectRemovedJobs_MarksMissingIDs(t *testing.T) {
	ledger := &fakeLedger{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, &fakePublisher{})
	prev := []string{"g-1", "g-2", "g-3"}
	curr := []ats.FetchJobsItem{{ID: "g-1"}}
	e.detectRemovedJobs(context.Background(), curr, prev, logger.NewNoop())
	if len(ledger.markRemovedCalls) != 1 {
		t.Fatalf("expected 1 MarkRemovedBatch call, got %d", len(ledger.markRemovedCalls))
	}
	removed := ledger.markRemovedCalls[0]
	if len(removed) != 2 {
		t.Fatalf("expected 2 removed IDs, got %d (%v)", len(removed), removed)
	}
	seen := map[string]bool{}
	for _, id := range removed {
		seen[id] = true
	}
	if !seen["g-2"] || !seen["g-3"] {
		t.Errorf("expected g-2 and g-3 marked removed, got %v", removed)
	}
}

// TestDetectRemovedJobs_LedgerErrorIsSwallowed verifies a flapping ledger
// does not panic or propagate — the engine logs and moves on (next scan
// will retry).
func TestDetectRemovedJobs_LedgerErrorIsSwallowed(t *testing.T) {
	ledger := &fakeLedger{markRemovedErr: errors.New("ledger down")}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, &fakePublisher{})
	prev := []string{"g-1", "g-2"}
	curr := []ats.FetchJobsItem{{ID: "g-1"}}
	// Should not panic.
	e.detectRemovedJobs(context.Background(), curr, prev, logger.NewNoop())
}

// ── recordResult ─────────────────────────────────────────────────────────────

// TestRecordResult_Success verifies the engine forwards counts + duration to
// the recorder and leaves ErrMsg empty.
func TestRecordResult_Success(t *testing.T) {
	rec := &fakeRecorder{}
	e := newTestEngine(&fakeSettingsReader{}, &fakeLedger{}, rec, &fakePublisher{})
	e.recordResult(context.Background(), "co", 5, 2, 100, nil)
	if len(rec.results) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rec.results))
	}
	r := rec.results[0]
	if r.FetchedCount != 5 || r.NewCount != 2 {
		t.Errorf("counts: %+v", r)
	}
	if r.ErrMsg != "" {
		t.Errorf("ErrMsg must be empty on success, got %q", r.ErrMsg)
	}
}

// TestRecordResult_ErrorPath verifies the error string is forwarded to the
// recorder when set.
func TestRecordResult_ErrorPath(t *testing.T) {
	rec := &fakeRecorder{}
	e := newTestEngine(&fakeSettingsReader{}, &fakeLedger{}, rec, &fakePublisher{})
	msg := "fetch failed"
	e.recordResult(context.Background(), "co", 0, 0, 100, &msg)
	if rec.results[0].ErrMsg != "fetch failed" {
		t.Errorf("ErrMsg: got %q, want %q", rec.results[0].ErrMsg, msg)
	}
}

// TestRecordResult_RecorderErrorIsSwallowed verifies the engine does not
// panic or propagate when the recorder fails — analytics is best-effort.
func TestRecordResult_RecorderErrorIsSwallowed(t *testing.T) {
	rec := &fakeRecorder{err: errors.New("record failed")}
	e := newTestEngine(&fakeSettingsReader{}, &fakeLedger{}, rec, &fakePublisher{})
	e.recordResult(context.Background(), "co", 0, 0, 100, nil)
}

// ── fetchJobs — via injected adapterFactory ──────────────────────────────────

// TestFetchJobs_UnknownAdapterReturnsError verifies the engine wraps the
// registry's unknown-type error in a clear context message — the scan-error
// recorder needs the wrapped message for ops.
func TestFetchJobs_UnknownAdapterReturnsError(t *testing.T) {
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, &fakeLedger{}, &fakeRecorder{}, pub)
	// Override the factory so we don't depend on the real registry's set of
	// known adapter types.
	e.adapterFactory = func(t string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return nil, errors.New("unknown adapter type: " + t)
	}
	_, _, err := e.fetchJobs(context.Background(), "co", "unknown_type", nil, logger.NewNoop())
	if err == nil {
		t.Fatal("expected error for unknown adapter type")
	}
}

// TestFetchJobs_PropagatesAdapterFetchError verifies the engine wraps the
// adapter's FetchJobs error (downstream sees the wrap context).
func TestFetchJobs_PropagatesAdapterFetchError(t *testing.T) {
	stub := &stubAdapter{err: errors.New("adapter boom")}
	e := newTestEngine(&fakeSettingsReader{}, &fakeLedger{}, &fakeRecorder{}, &fakePublisher{})
	e.adapterFactory = func(_ string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return stub, nil
	}
	_, _, err := e.fetchJobs(context.Background(), "co", "stub", nil, logger.NewNoop())
	if err == nil {
		t.Fatal("expected wrapped adapter error")
	}
}

// TestFetchJobs_StreamPublisherWiring verifies the engine sets the stream
// callback when the adapter implements StreamPublisher AND that the callback
// routes through publishNewJobs (i.e. respects the seen-batch + publish path).
func TestFetchJobs_StreamPublisherWiring(t *testing.T) {
	stub := &streamingStubAdapter{
		jobs: []ats.FetchJobsItem{{ID: "g-stream-1"}, {ID: "g-stream-2"}},
	}
	ledger := &fakeLedger{seenMap: map[string]bool{}}
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)
	e.adapterFactory = func(_ string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return stub, nil
	}
	_, streamed, err := e.fetchJobs(context.Background(), "co", "stub", nil, logger.NewNoop())
	if err != nil {
		t.Fatalf("fetchJobs: %v", err)
	}
	if !streamed {
		t.Error("expected streamed=true for StreamPublisher adapter")
	}
	if len(pub.messages) != 2 {
		t.Errorf("expected 2 streamed publishes, got %d", len(pub.messages))
	}
}

// TestFetchJobs_KnownJobsInjection verifies the engine passes knownIDs to
// adapters that implement KnownJobsProvider. Adapters use this to skip
// detail enrichment for already-tracked jobs.
func TestFetchJobs_KnownJobsInjection(t *testing.T) {
	stub := &streamingStubAdapter{}
	e := newTestEngine(&fakeSettingsReader{}, &fakeLedger{}, &fakeRecorder{}, &fakePublisher{})
	e.adapterFactory = func(_ string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return stub, nil
	}
	_, _, err := e.fetchJobs(context.Background(), "co", "stub", []string{"g-1", "g-2"}, logger.NewNoop())
	if err != nil {
		t.Fatalf("fetchJobs: %v", err)
	}
	if len(stub.known) != 2 {
		t.Errorf("expected 2 known IDs injected, got %d", len(stub.known))
	}
}

// TestFetchJobs_SeenCheckerInjection verifies the engine wires HasSeenBatch
// into adapters that need live per-page checks.
func TestFetchJobs_SeenCheckerInjection(t *testing.T) {
	stub := &streamingStubAdapter{
		jobs: []ats.FetchJobsItem{{ID: "g-1"}},
	}
	ledger := &fakeLedger{seenMap: map[string]bool{}}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, &fakePublisher{})
	e.adapterFactory = func(_ string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return stub, nil
	}
	_, _, err := e.fetchJobs(context.Background(), "co", "stub", nil, logger.NewNoop())
	if err != nil {
		t.Fatalf("fetchJobs: %v", err)
	}
	// streamingStubAdapter's FetchJobs invokes seenChecker if set — that's
	// the seen-batch call that reaches the fakeLedger.
	if ledger.hasSeenBatchCalls < 1 {
		t.Errorf("expected seenChecker to have been wired; ledger.hasSeenBatchCalls=%d", ledger.hasSeenBatchCalls)
	}
}

// ── scanCompany — full happy path through the engine ─────────────────────────

// TestScanCompany_HappyPath_NonStreaming verifies a complete scan: load known
// IDs → adapter returns jobs → publishNewJobs publishes new jobs → ledger marks
// them seen → recordResult writes the summary.
func TestScanCompany_HappyPath_NonStreaming(t *testing.T) {
	stub := &stubAdapter{jobs: []ats.FetchJobsItem{{ID: "g-1"}, {ID: "g-2"}}}
	ledger := &fakeLedger{seenMap: map[string]bool{}}
	rec := &fakeRecorder{}
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, rec, pub)
	e.adapterFactory = func(_ string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return stub, nil
	}

	settings := &companymodel.CompanyScanSettings{
		CompanyID: "co", AdapterType: "stub", Active: true,
	}
	e.scanCompany(context.Background(), "co", settings)

	if len(pub.messages) != 2 {
		t.Errorf("expected 2 publishes, got %d", len(pub.messages))
	}
	if !rec.called {
		t.Error("recorder must be called on success")
	}
	if rec.results[0].FetchedCount != 2 || rec.results[0].NewCount != 2 {
		t.Errorf("recorder counts: %+v", rec.results[0])
	}
	if rec.results[0].ErrMsg != "" {
		t.Errorf("ErrMsg must be empty on success; got %q", rec.results[0].ErrMsg)
	}
}

// TestScanCompany_FetchError verifies the failure path: adapter error →
// recorder gets ErrMsg, counts are zero, no publishes happen.
func TestScanCompany_FetchError(t *testing.T) {
	stub := &stubAdapter{err: errors.New("adapter died")}
	ledger := &fakeLedger{}
	rec := &fakeRecorder{}
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, rec, pub)
	e.adapterFactory = func(_ string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return stub, nil
	}

	settings := &companymodel.CompanyScanSettings{CompanyID: "co", AdapterType: "stub", Active: true}
	e.scanCompany(context.Background(), "co", settings)

	if pub.called {
		t.Error("publisher must not be called on fetch error")
	}
	if len(rec.results) != 1 {
		t.Fatalf("expected 1 recorder call, got %d", len(rec.results))
	}
	if rec.results[0].ErrMsg == "" {
		t.Error("recorder ErrMsg must be set on fetch failure")
	}
}

// TestScanCompany_GetActiveJobIDsError verifies the engine continues with
// nil knownIDs when the ledger call fails — enrichment runs for all jobs
// but the scan does not abort.
func TestScanCompany_GetActiveJobIDsError(t *testing.T) {
	stub := &stubAdapter{jobs: []ats.FetchJobsItem{{ID: "g-1"}}}
	ledger := &fakeLedger{
		activeErr: errors.New("ledger select failed"),
		seenMap:   map[string]bool{},
	}
	pub := &fakePublisher{}
	rec := &fakeRecorder{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, rec, pub)
	e.adapterFactory = func(_ string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return stub, nil
	}

	settings := &companymodel.CompanyScanSettings{CompanyID: "co", AdapterType: "stub", Active: true}
	e.scanCompany(context.Background(), "co", settings)

	if len(pub.messages) != 1 {
		t.Errorf("expected 1 publish despite knownIDs error, got %d", len(pub.messages))
	}
	if !rec.called {
		t.Error("recorder must still be called after knownIDs error")
	}
}

// TestScanCompany_DetectsRemovedJobs verifies the full pipeline: known IDs
// exist, adapter returns a subset, the missing IDs land in MarkRemovedBatch.
func TestScanCompany_DetectsRemovedJobs(t *testing.T) {
	stub := &stubAdapter{jobs: []ats.FetchJobsItem{{ID: "g-1"}}}
	ledger := &fakeLedger{
		activeIDs: []string{"g-1", "g-2", "g-3"},
		seenMap:   map[string]bool{"g-1": true},
	}
	pub := &fakePublisher{}
	rec := &fakeRecorder{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, rec, pub)
	e.adapterFactory = func(_ string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return stub, nil
	}

	settings := &companymodel.CompanyScanSettings{CompanyID: "co", AdapterType: "stub", Active: true}
	e.scanCompany(context.Background(), "co", settings)

	if len(ledger.markRemovedCalls) != 1 {
		t.Fatalf("expected MarkRemovedBatch call, got %d", len(ledger.markRemovedCalls))
	}
	removed := ledger.markRemovedCalls[0]
	if len(removed) != 2 {
		t.Errorf("expected 2 IDs removed (g-2,g-3), got %v", removed)
	}
}

// TestScanCompany_StreamingSkipsPostScanPublish verifies that when the
// adapter is a StreamPublisher, the post-scan publishNewJobs pass is
// skipped — streamed adapters publish per-page inline.
func TestScanCompany_StreamingSkipsPostScanPublish(t *testing.T) {
	jobs := []ats.FetchJobsItem{{ID: "g-1"}, {ID: "g-2"}}
	stub := &streamingStubAdapter{jobs: jobs}
	ledger := &fakeLedger{seenMap: map[string]bool{}}
	pub := &fakePublisher{}
	rec := &fakeRecorder{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, rec, pub)
	e.adapterFactory = func(_ string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return stub, nil
	}

	settings := &companymodel.CompanyScanSettings{CompanyID: "co", AdapterType: "stub", Active: true}
	e.scanCompany(context.Background(), "co", settings)

	// streaming adapter publishes 2 jobs via streamPublish callback (which
	// routes through publishNewJobs once). The non-streamed path would have
	// caused a second publishNewJobs invocation — verify we didn't double-call.
	if len(pub.messages) != 2 {
		t.Errorf("expected 2 streamed publishes (no double-publish), got %d", len(pub.messages))
	}
	if rec.results[0].NewCount != 2 {
		t.Errorf("recorder NewCount: got %d, want 2", rec.results[0].NewCount)
	}
}

// TestRunForCompany_FullSuccessPath threads the public entry point through
// the engine — verifies RunForCompany delegates correctly to scanCompany
// on the happy path.
func TestRunForCompany_FullSuccessPath(t *testing.T) {
	settings := &fakeSettingsReader{
		settings: &companymodel.CompanyScanSettings{
			CompanyID: "co", AdapterType: "stub", Active: true,
		},
	}
	stub := &stubAdapter{jobs: []ats.FetchJobsItem{{ID: "g-1"}}}
	ledger := &fakeLedger{seenMap: map[string]bool{}}
	pub := &fakePublisher{}
	rec := &fakeRecorder{}
	e := newTestEngine(settings, ledger, rec, pub)
	e.adapterFactory = func(_ string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return stub, nil
	}

	e.RunForCompany(context.Background(), "co")

	if !rec.called {
		t.Error("recorder must be called on full RunForCompany success")
	}
	if len(pub.messages) != 1 {
		t.Errorf("expected 1 publish, got %d", len(pub.messages))
	}
	if stub.observedCompanyID != "co" {
		t.Errorf("adapter must receive companyID; got %q", stub.observedCompanyID)
	}
}
