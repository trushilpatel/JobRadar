package main

import (
	"context"
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/apps/crawler/ats/google"
	apperr "github.com/trushilpatel/jobradar/common/errors"
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
	called bool
}

func (f *fakeLedger) HasSeenBatch(_ context.Context, _ []string) (map[string]bool, error) {
	f.called = true
	return nil, nil
}
func (f *fakeLedger) MarkSeenBatch(_ context.Context, _ []string, _, _ string) error {
	f.called = true
	return nil
}
func (f *fakeLedger) GetActiveJobIDsByCompany(_ context.Context, _ string) ([]string, error) {
	f.called = true
	return nil, nil
}
func (f *fakeLedger) MarkRemovedBatch(_ context.Context, _ []string) error {
	f.called = true
	return nil
}

// fakeRecorder satisfies ScanRecorder; tracks whether RecordScanResult fired.
type fakeRecorder struct {
	called bool
}

func (f *fakeRecorder) RecordScanResult(_ context.Context, _ string, _ scandto.ScanResult) error {
	f.called = true
	return nil
}

// fakePublisher satisfies kafka.Publisher; tracks whether Publish fired.
type fakePublisher struct {
	called bool
}

func (f *fakePublisher) Publish(_ context.Context, _ string, _ []byte) error {
	f.called = true
	return nil
}

func (f *fakePublisher) PublishWithHeaders(_ context.Context, _ string, _ []byte, _ []kgo.RecordHeader) error {
	f.called = true
	return nil
}

func newTestEngine(settings *fakeSettingsReader, ledger *fakeLedger, rec *fakeRecorder, pub *fakePublisher) *Engine {
	return NewEngine(settings, ledger, rec, pub, google.Config{}, logger.NewNoop())
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
