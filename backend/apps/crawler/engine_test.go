package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/trushilpatel/jobradar/apps/crawler/ats"
	"github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	atstypes "github.com/trushilpatel/jobradar/apps/crawler/ats/types"
	apperr "github.com/trushilpatel/jobradar/common/errors"
	js "github.com/trushilpatel/jobradar/common/jetstream"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/metrics"
	companymodel "github.com/trushilpatel/jobradar/service/company/models"
	jobcommon "github.com/trushilpatel/jobradar/service/job/common"
	scandto "github.com/trushilpatel/jobradar/service/scan/dto"
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
	// markSeenReasons records the reason for each MarkSeenBatch call in order.
	markSeenReasons []string
	// markSeenErrForReason allows injecting a per-reason error: if set and the
	// call's reason matches a key, that error is returned for that call.
	markSeenErrForReason map[string]error
	// order, when set, records "mark_seen" / "publish" events across both the
	// ledger and publisher fakes so tests can assert call ordering.
	order   *[]string
	orderMu *sync.Mutex
}

// recordOrder appends an event to the shared order log under its mutex.
func recordOrder(order *[]string, mu *sync.Mutex, event string) {
	if order == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	*order = append(*order, event)
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
	f.markSeenReasons = append(f.markSeenReasons, reason)
	if reason == jobcommon.SeenReasonFetch {
		recordOrder(f.order, f.orderMu, "mark_seen")
	}
	if f.markSeenErrForReason != nil {
		if err, ok := f.markSeenErrForReason[reason]; ok {
			return err
		}
	}
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

// fakePublisher satisfies js.Publisher; tracks whether Publish fired.
type fakePublisher struct {
	mu       sync.Mutex
	called   bool
	messages [][]byte
	subject  string
	err      error
	// failOn allows the publisher to fail for specific job IDs (matched by
	// JSON field `"id":"<value>"`).
	failOn map[string]error
	// order, when set, records "publish" events so ordering tests can assert
	// that MarkSeenBatch ran before any publish.
	order   *[]string
	orderMu *sync.Mutex
}

func (f *fakePublisher) doPublish(subject string, payload []byte) error {
	f.called = true
	f.subject = subject
	recordOrder(f.order, f.orderMu, "publish")
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

func (f *fakePublisher) Publish(_ context.Context, subject string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.doPublish(subject, payload)
}

func (f *fakePublisher) PublishWithMsgID(_ context.Context, subject string, payload []byte, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.doPublish(subject, payload)
}

var _ js.Publisher = (*fakePublisher)(nil)

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

// TestToRawJob_PropagatesLocationFields verifies LocationKnown and AllLocations
// are propagated from FetchJobsItem to RawJob by toRawJob.
func TestToRawJob_PropagatesLocationFields(t *testing.T) {
	item := ats.FetchJobsItem{
		ID:            "lever-2",
		LocationKnown: true,
		AllLocations:  []string{"San Francisco, CA", "New York, NY"},
	}
	raw := toRawJob(item, "co", "lever")
	if !raw.LocationKnown {
		t.Errorf("LocationKnown must be propagated; got false")
	}
	if len(raw.AllLocations) != 2 || raw.AllLocations[0] != "San Francisco, CA" {
		t.Errorf("AllLocations must be propagated; got %v", raw.AllLocations)
	}
}

// TestPublishNewJobs_NonUSADroppedBeforePublish verifies that jobs with
// LocationKnown=true and a non-USA AllLocations are dropped via MarkSeenBatch
// (reason=non_usa_location) before publishing to NATS. The publisher must not
// see those jobs.
func TestPublishNewJobs_NonUSADroppedBeforePublish(t *testing.T) {
	jobs := []ats.FetchJobsItem{
		{ID: "g-1", LocationKnown: true, AllLocations: []string{"San Francisco, CA"}},
		{ID: "g-2", LocationKnown: true, AllLocations: []string{"London, UK"}},
		{ID: "g-3", LocationKnown: false, AllLocations: nil}, // unknown — keep
	}
	ledger := &fakeLedger{seenMap: map[string]bool{}}
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), jobs, "co", "greenhouse", logger.NewNoop())

	// g-1 (USA) + g-3 (unknown) should publish; g-2 (non-USA) should be dropped.
	if n != 2 {
		t.Errorf("expected 2 published (g-1, g-3), got %d", n)
	}
	if len(pub.messages) != 2 {
		t.Errorf("expected 2 NATS publishes, got %d", len(pub.messages))
	}
	// Verify the non-USA job was marked seen with the non_usa_location reason.
	var foundNonUSA bool
	for _, call := range ledger.markSeenCalls {
		for _, id := range call {
			if id == "g-2" {
				foundNonUSA = true
			}
		}
	}
	if !foundNonUSA {
		t.Errorf("expected g-2 to be marked seen via MarkSeenBatch; calls=%v", ledger.markSeenCalls)
	}
	// Verify at least one MarkSeenBatch call used the non_usa_location reason.
	var foundReason bool
	for _, r := range ledger.markSeenReasons {
		if r == jobcommon.SeenReasonNonUSA {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Errorf("expected a MarkSeenBatch call with reason %q; reasons seen=%v", jobcommon.SeenReasonNonUSA, ledger.markSeenReasons)
	}
}

// TestPublishNewJobs_NonUSALedgerFailureIsolated verifies that a MarkSeenBatch
// failure for the non-USA gate does not stop other jobs from being published.
func TestPublishNewJobs_NonUSALedgerFailureIsolated(t *testing.T) {
	jobs := []ats.FetchJobsItem{
		{ID: "g-1", LocationKnown: true, AllLocations: []string{"London, UK"}},
		{ID: "g-2", LocationKnown: true, AllLocations: []string{"San Francisco, CA"}},
	}
	ledger := &fakeLedger{
		seenMap:              map[string]bool{},
		markSeenErrForReason: map[string]error{jobcommon.SeenReasonNonUSA: errors.New("ledger flaky")},
	}
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), jobs, "co", "greenhouse", logger.NewNoop())
	// g-2 (USA) must still publish even though the non-USA mark for g-1 failed.
	if n != 1 {
		t.Errorf("expected 1 published (g-2), got %d", n)
	}
}

// TestPublishNewJobs_LocationKnownFalsePassesThrough verifies that jobs with
// LocationKnown=false bypass the non-USA gate entirely and proceed to publish.
func TestPublishNewJobs_LocationKnownFalsePassesThrough(t *testing.T) {
	jobs := []ats.FetchJobsItem{
		{ID: "meta-1", LocationKnown: false, AllLocations: nil},
	}
	ledger := &fakeLedger{seenMap: map[string]bool{}}
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), jobs, "co", "meta", logger.NewNoop())
	if n != 1 {
		t.Errorf("expected 1 published for LocationKnown=false job, got %d", n)
	}
}

// ── publish (private NATS wrapper) ───────────────────────────────────────────

// TestPublish_MarshalsAndSendsToRawSubject verifies publish marshals the RawJob
// and routes it to the adapter-scoped NATS subject (jobs.raw.<adapter>).
func TestPublish_MarshalsAndSendsToRawSubject(t *testing.T) {
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, &fakeLedger{}, &fakeRecorder{}, pub)
	job := ats.RawJob{ID: "g-1", CompanyID: "co", Adapter: "greenhouse"}

	if err := e.publish(context.Background(), job); err != nil {
		t.Fatalf("publish: %v", err)
	}
	want := fmt.Sprintf(js.SubjectJobsRawFmt, "greenhouse")
	if pub.subject != want {
		t.Errorf("subject: got %q, want %q", pub.subject, want)
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
// publish failure does not abort the batch — the next job still ships. With
// the mark-seen-first ordering, ALL new IDs are recorded in the ledger
// (reason='fetch') before any publish, so a failed publish leaves its row at
// 'fetch' for the next scan to recover.
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
	// Mark-seen-first: every new job is recorded before publish, including g-2.
	got := ledger.markSeenCalls[0]
	if len(got) != 3 || got[0] != "g-1" || got[1] != "g-2" || got[2] != "g-3" {
		t.Errorf("MarkSeenBatch IDs: got %v, want [g-1 g-2 g-3]", got)
	}
}

// TestPublishNewJobs_AllPublishesFail verifies that when every publish fails,
// the ledger rows still exist at reason='fetch' (recorded before publish), and
// the returned count is 0. The next scan re-publishes via HasSeenBatch.
func TestPublishNewJobs_AllPublishesFail(t *testing.T) {
	jobs := []ats.FetchJobsItem{{ID: "g-1"}, {ID: "g-2"}}
	ledger := &fakeLedger{seenMap: map[string]bool{}}
	pub := &fakePublisher{err: errors.New("broker down")}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), jobs, "co", "greenhouse", logger.NewNoop())
	if n != 0 {
		t.Errorf("expected 0 when all publishes fail, got %d", n)
	}
	// The ledger rows are written before publish, so the call IS made.
	if len(ledger.markSeenCalls) != 1 {
		t.Fatalf("expected one MarkSeenBatch (mark-seen-first), got %d", len(ledger.markSeenCalls))
	}
	if ledger.markSeenLastReason != jobcommon.SeenReasonFetch {
		t.Errorf("MarkSeenBatch reason: got %q, want fetch", ledger.markSeenLastReason)
	}
}

// TestPublishNewJobs_MarkSeenFails_SkipsPublish verifies that if the pre-publish
// ledger write fails, the engine does NOT publish — publishing without a ledger
// row would re-open the crawler→filter race. The next scan re-discovers the job.
func TestPublishNewJobs_MarkSeenFails_SkipsPublish(t *testing.T) {
	jobs := []ats.FetchJobsItem{{ID: "g-1"}}
	ledger := &fakeLedger{seenMap: map[string]bool{}, markSeenErr: errors.New("ledger flaky")}
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), jobs, "co", "greenhouse", logger.NewNoop())
	if n != 0 {
		t.Errorf("expected 0 when pre-publish ledger write fails, got %d", n)
	}
	if pub.called {
		t.Error("publisher must not be called when the pre-publish ledger write failed")
	}
}

// TestPublishNewJobs_LedgerInsertBeforePublish (#15) — asserts MarkSeenBatch
// (reason='fetch') happens before any publish, closing the crawler→filter race.
func TestPublishNewJobs_LedgerInsertBeforePublish(t *testing.T) {
	var order []string
	mu := &sync.Mutex{}
	jobs := []ats.FetchJobsItem{{ID: "g-1"}, {ID: "g-2"}}
	ledger := &fakeLedger{seenMap: map[string]bool{}, order: &order, orderMu: mu}
	pub := &fakePublisher{order: &order, orderMu: mu}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), jobs, "co", "greenhouse", logger.NewNoop())
	if n != 2 {
		t.Fatalf("expected 2 published, got %d", n)
	}
	if len(order) == 0 || order[0] != "mark_seen" {
		t.Fatalf("expected first event to be mark_seen, got %v", order)
	}
	for i, ev := range order {
		if ev == "publish" {
			continue
		}
		if ev == "mark_seen" && i != 0 {
			t.Fatalf("mark_seen must be the only pre-publish event; order=%v", order)
		}
	}
	// Every publish must come after the single mark_seen.
	firstPublish := -1
	for i, ev := range order {
		if ev == "publish" {
			firstPublish = i
			break
		}
	}
	if firstPublish <= 0 {
		t.Fatalf("expected publishes to follow mark_seen; order=%v", order)
	}
}

// TestPublishNewJobs_PublishFailure_LedgerRemainsAtFetch (#16) — a per-job
// publish failure increments crawler_publish_failures_total and leaves the
// ledger row at reason='fetch' (it was written before publish).
func TestPublishNewJobs_PublishFailure_LedgerRemainsAtFetch(t *testing.T) {
	jobs := []ats.FetchJobsItem{{ID: "g-1"}, {ID: "g-2"}}
	ledger := &fakeLedger{seenMap: map[string]bool{}}
	pub := &fakePublisher{failOn: map[string]error{"g-2": errors.New("transient")}}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	before := promtestutil.ToFloat64(metrics.CrawlerPublishFailures.WithLabelValues("greenhouse"))
	n := e.publishNewJobs(context.Background(), jobs, "co", "greenhouse", logger.NewNoop())
	after := promtestutil.ToFloat64(metrics.CrawlerPublishFailures.WithLabelValues("greenhouse"))

	if n != 1 {
		t.Errorf("expected 1 published (g-1), got %d", n)
	}
	if after-before != 1 {
		t.Errorf("expected crawler_publish_failures_total +1, got +%v", after-before)
	}
	// The ledger was written (reason='fetch') for both before publish.
	if len(ledger.markSeenCalls) != 1 || len(ledger.markSeenCalls[0]) != 2 {
		t.Errorf("expected one MarkSeenBatch with both IDs, got %v", ledger.markSeenCalls)
	}
	if ledger.markSeenLastReason != jobcommon.SeenReasonFetch {
		t.Errorf("MarkSeenBatch reason: got %q, want fetch", ledger.markSeenLastReason)
	}
}

// TestPublishNewJobs_NoNewJobs_NoLedgerWrite (#17) — when all jobs are already
// in the ledger, there is no ledger insert and no publish.
func TestPublishNewJobs_NoNewJobs_NoLedgerWrite(t *testing.T) {
	jobs := []ats.FetchJobsItem{{ID: "g-1"}, {ID: "g-2"}}
	ledger := &fakeLedger{seenMap: map[string]bool{"g-1": true, "g-2": true}}
	pub := &fakePublisher{}
	e := newTestEngine(&fakeSettingsReader{}, ledger, &fakeRecorder{}, pub)

	n := e.publishNewJobs(context.Background(), jobs, "co", "greenhouse", logger.NewNoop())
	if n != 0 {
		t.Errorf("expected 0 new, got %d", n)
	}
	if len(ledger.markSeenCalls) != 0 {
		t.Errorf("MarkSeenBatch must not be called when no new jobs; got %v", ledger.markSeenCalls)
	}
	if pub.called {
		t.Error("publisher must not be called when no new jobs")
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
	_, _, err := e.fetchJobs(context.Background(), "co", "unknown_type", nil, nil, logger.NewNoop())
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
	_, _, err := e.fetchJobs(context.Background(), "co", "stub", nil, nil, logger.NewNoop())
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
	_, streamed, err := e.fetchJobs(context.Background(), "co", "stub", nil, nil, logger.NewNoop())
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
	_, _, err := e.fetchJobs(context.Background(), "co", "stub", nil, []string{"g-1", "g-2"}, logger.NewNoop())
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
	_, _, err := e.fetchJobs(context.Background(), "co", "stub", nil, nil, logger.NewNoop())
	if err != nil {
		t.Fatalf("fetchJobs: %v", err)
	}
	// streamingStubAdapter's FetchJobs invokes seenChecker if set — that's
	// the seen-batch call that reaches the fakeLedger.
	if ledger.hasSeenBatchCalls < 1 {
		t.Errorf("expected seenChecker to have been wired; ledger.hasSeenBatchCalls=%d", ledger.hasSeenBatchCalls)
	}
}

// TestFetchJobs_ConfigurableAdapter_UsesFetchJobsWithConfig verifies that when
// an adapter implements ConfigurableAdapter AND scan_settings carries a
// non-empty adapter_config, the engine calls FetchJobsWithConfig with the raw
// config bytes — NOT plain FetchJobs.
func TestFetchJobs_ConfigurableAdapter_UsesFetchJobsWithConfig(t *testing.T) {
	stub := &configurableStubAdapter{
		jobs: []ats.FetchJobsItem{{ID: "wd-1"}},
	}
	e := newTestEngine(&fakeSettingsReader{}, &fakeLedger{}, &fakeRecorder{}, &fakePublisher{})
	e.adapterFactory = func(_ string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return stub, nil
	}

	cfg := json.RawMessage(`{"host":"nvidia.wd5.myworkdayjobs.com","site":"NVIDIA"}`)
	_, _, err := e.fetchJobs(context.Background(), "co", "workday", cfg, nil, logger.NewNoop())
	if err != nil {
		t.Fatalf("fetchJobs: %v", err)
	}
	if !stub.configCalled {
		t.Error("expected FetchJobsWithConfig to be invoked")
	}
	if stub.plainCalled {
		t.Error("plain FetchJobs must NOT be invoked when ConfigurableAdapter applies")
	}
	if string(stub.gotConfig) != string(cfg) {
		t.Errorf("config passed = %q, want %q", stub.gotConfig, cfg)
	}
}

// TestFetchJobs_ConfigurableAdapter_FallsBackWhenConfigEmpty verifies that
// ConfigurableAdapter is NOT used when adapter_config is empty/nil — the
// engine falls back to plain FetchJobs, preserving compatibility with adapters
// that may opt-in to ConfigurableAdapter but be invoked from a company that
// doesn't carry config.
func TestFetchJobs_ConfigurableAdapter_FallsBackWhenConfigEmpty(t *testing.T) {
	stub := &configurableStubAdapter{
		jobs: []ats.FetchJobsItem{{ID: "wd-1"}},
	}
	e := newTestEngine(&fakeSettingsReader{}, &fakeLedger{}, &fakeRecorder{}, &fakePublisher{})
	e.adapterFactory = func(_ string, _ google.Config, _ logger.Logger) (ats.Adapter, error) {
		return stub, nil
	}

	_, _, err := e.fetchJobs(context.Background(), "co", "workday", nil, nil, logger.NewNoop())
	if err != nil {
		t.Fatalf("fetchJobs: %v", err)
	}
	if stub.configCalled {
		t.Error("FetchJobsWithConfig must NOT be invoked when config is empty")
	}
	if !stub.plainCalled {
		t.Error("expected plain FetchJobs fallback when config is empty")
	}
}

// configurableStubAdapter is a test double that records which entry point the
// engine chose. It implements both Adapter and ConfigurableAdapter.
type configurableStubAdapter struct {
	jobs         []ats.FetchJobsItem
	gotConfig    json.RawMessage
	configCalled bool
	plainCalled  bool
}

func (c *configurableStubAdapter) FetchJobs(_ context.Context, _ string) (ats.FetchJobsResponse, error) {
	c.plainCalled = true
	return ats.FetchJobsResponse{Jobs: c.jobs}, nil
}

func (c *configurableStubAdapter) FetchJobsWithConfig(_ context.Context, _ string, cfg json.RawMessage) (ats.FetchJobsResponse, error) {
	c.configCalled = true
	c.gotConfig = cfg
	return ats.FetchJobsResponse{Jobs: c.jobs}, nil
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
