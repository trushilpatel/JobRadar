package handler

import (
	"context"
	"errors"
	"time"

	"github.com/trushilpatel/jobradar/internal/model"
	"github.com/trushilpatel/jobradar/internal/store"
	"gorm.io/gorm"
)

// ── fakeJobStore ──────────────────────────────────────────────────────────────

type fakeJobStore struct {
	jobs   map[string]*model.Job
	err    error
	status string // last UpdateJobStatus call
	notes  string // last UpdateJobNotes call
}

func newFakeJobStore(jobs ...*model.Job) *fakeJobStore {
	m := map[string]*model.Job{}
	for _, j := range jobs {
		m[j.ID] = j
	}
	return &fakeJobStore{jobs: m}
}

func (f *fakeJobStore) GetJobsByStatus(status string) ([]model.Job, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []model.Job
	for _, j := range f.jobs {
		if j.Status == status {
			out = append(out, *j)
		}
	}
	return out, nil
}

func (f *fakeJobStore) GetJobByID(id string) (*model.Job, error) {
	if f.err != nil {
		return nil, f.err
	}
	j, ok := f.jobs[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return j, nil
}

func (f *fakeJobStore) GetJobCounts() (newC, savedC, appliedC int64, err error) {
	if f.err != nil {
		return 0, 0, 0, f.err
	}
	for _, j := range f.jobs {
		switch j.Status {
		case model.JobStatusNew:
			newC++
		case model.JobStatusSaved:
			savedC++
		case model.JobStatusApplied:
			appliedC++
		}
	}
	return
}

func (f *fakeJobStore) GetJobsAddedByDay(_ int) ([]store.DailyJobCount, error) {
	return nil, f.err
}

func (f *fakeJobStore) InsertJob(j *model.Job) error        { return f.err }
func (f *fakeJobStore) DeleteJob(_ string) error            { return f.err }
func (f *fakeJobStore) UpdateJobSaved(_ string, _ bool) error { return f.err }
func (f *fakeJobStore) UpdateJobNotes(_ string, notes string) error {
	f.notes = notes
	return f.err
}
func (f *fakeJobStore) UpdateJobStatus(_ string, status string) error {
	f.status = status
	return f.err
}

// ── fakeLedger ────────────────────────────────────────────────────────────────

type fakeLedgerWriter struct {
	lastReason string
}

func (f *fakeLedgerWriter) MarkSeen(_, _, _ string) error { return nil }
func (f *fakeLedgerWriter) UpdateSeenReason(_ string, reason string) error {
	f.lastReason = reason
	return nil
}
func (f *fakeLedgerWriter) UpdateSeenReasonBatch(_ []string, _ string) error { return nil }

// ── fakeDiscardStore ──────────────────────────────────────────────────────────

type fakeDiscardStore struct{ called bool }

func (f *fakeDiscardStore) InsertDiscard(_, _, _ string) error {
	f.called = true
	return nil
}

// ── fakeSnapshotWriter ────────────────────────────────────────────────────────

type fakeSnapshotWriter struct{ lastAction string }

func (f *fakeSnapshotWriter) InsertSnapshot(_ store.SnapshotInput) error { return nil }
func (f *fakeSnapshotWriter) UpdateSnapshotAction(_ string, action string) error {
	f.lastAction = action
	return nil
}

// ── fakeApplicationStore ──────────────────────────────────────────────────────

type fakeApplicationStore struct {
	lastStage string
	lastNote  string
	err       error
}

func (f *fakeApplicationStore) GetApplicationsByJobID(_ string) ([]model.JobApplication, error) {
	return nil, f.err
}
func (f *fakeApplicationStore) InsertApplication(_, stage, _ string) error {
	f.lastStage = stage
	return f.err
}
func (f *fakeApplicationStore) GetLatestApplicationsWithJob() ([]store.ApplicationWithJob, error) {
	return nil, f.err
}
func (f *fakeApplicationStore) UpdateApplicationStage(_ int64, stage string) error {
	f.lastStage = stage
	return f.err
}
func (f *fakeApplicationStore) UpdateApplicationNote(_ int64, note string) error {
	f.lastNote = note
	return f.err
}

// ── fakeCompanyStore ──────────────────────────────────────────────────────────

type fakeCompanyStore struct {
	companies map[string]model.Company
	err       error
}

func newFakeCompanyStore(companies ...model.Company) *fakeCompanyStore {
	m := map[string]model.Company{}
	for _, c := range companies {
		m[c.ID] = c
	}
	return &fakeCompanyStore{companies: m}
}

func (f *fakeCompanyStore) GetAllCompanies() ([]model.Company, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]model.Company, 0, len(f.companies))
	for _, c := range f.companies {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeCompanyStore) GetActiveCompanies() ([]model.Company, error) {
	return f.GetAllCompanies()
}

func (f *fakeCompanyStore) GetCompanyByID(id string) (model.Company, error) {
	if f.err != nil {
		return model.Company{}, f.err
	}
	c, ok := f.companies[id]
	if !ok {
		return model.Company{}, gorm.ErrRecordNotFound
	}
	return c, nil
}

func (f *fakeCompanyStore) GetCompaniesDueForScan(_ int) ([]model.Company, error) {
	return nil, f.err
}

func (f *fakeCompanyStore) CreateCompany(c model.Company) error {
	if f.err != nil {
		return f.err
	}
	f.companies[c.ID] = c
	return nil
}

func (f *fakeCompanyStore) UpdateCompany(id string, fields map[string]any) error {
	return f.err
}

func (f *fakeCompanyStore) DeleteCompany(id string) error {
	delete(f.companies, id)
	return f.err
}

// ── fakeScanLogReader ─────────────────────────────────────────────────────────

type fakeScanLogReader struct {
	scannedToday int64
	err          error
}

func (f *fakeScanLogReader) GetScanStats(_ string) (store.ScanStats, error) {
	return store.ScanStats{}, f.err
}
func (f *fakeScanLogReader) GetJobsScannedToday() (int64, error) {
	return f.scannedToday, f.err
}
func (f *fakeScanLogReader) GetScansByDay(_ int) ([]store.DailyScanCount, error) {
	return []store.DailyScanCount{}, f.err
}

// ── fakeInsightsReader ────────────────────────────────────────────────────────

type fakeInsightsReader struct {
	techLimit     int
	velocityLimit int
	err           error
}

func (f *fakeInsightsReader) GetTechStackTrends(limit int) ([]store.TechCount, error) {
	f.techLimit = limit
	return []store.TechCount{{Tech: "Go", Count: 10}}, f.err
}
func (f *fakeInsightsReader) GetCompanyActivity() ([]store.CompanyActivityItem, error) {
	return nil, f.err
}
func (f *fakeInsightsReader) GetPipelineFunnel() ([]store.FunnelStep, error) {
	return []store.FunnelStep{{Stage: "enriched", Count: 5}}, f.err
}
func (f *fakeInsightsReader) GetLocationBreakdown() ([]store.LocationCount, error) {
	return nil, f.err
}
func (f *fakeInsightsReader) GetUSAStateBreakdown() ([]store.StateCount, error) {
	return nil, f.err
}
func (f *fakeInsightsReader) GetSponsorshipTrend(_ int) ([]store.SponsorshipDay, error) {
	return nil, f.err
}
func (f *fakeInsightsReader) GetCompanyVelocity(limit int) ([]store.CompanyVelocity, error) {
	f.velocityLimit = limit
	return nil, f.err
}
func (f *fakeInsightsReader) GetRoleCategoryBreakdown() ([]store.RoleCategoryCount, error) {
	return nil, f.err
}
func (f *fakeInsightsReader) GetPipelineCounts() (store.PipelineCounts, error) {
	return store.PipelineCounts{Pending: 3, Errors: 1}, f.err
}

// ── fakeScanQueueStore ────────────────────────────────────────────────────────

type fakeScanQueueStore struct {
	items []store.ScanQueueItem
	err   error
}

func (f *fakeScanQueueStore) Enqueue(companyID, source string) error {
	if f.err != nil {
		return f.err
	}
	f.items = append(f.items, store.ScanQueueItem{
		ID:        "fake-id",
		CompanyID: companyID,
		Source:    source,
		Status:    model.QueueStatusQueued,
		QueuedAt:  time.Now(),
	})
	return nil
}
func (f *fakeScanQueueStore) ClaimNext(_ context.Context) (*store.ScanQueueItem, error) {
	return nil, f.err
}
func (f *fakeScanQueueStore) Complete(_ string) error           { return f.err }
func (f *fakeScanQueueStore) ResetStuck(_ time.Duration) error { return f.err }
func (f *fakeScanQueueStore) ListQueue() ([]store.ScanQueueItem, error) {
	return f.items, f.err
}

// ── sentinel ──────────────────────────────────────────────────────────────────

var errStore = errors.New("store error")
