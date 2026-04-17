package application

import (
	"context"
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	jobsvc "github.com/trushilpatel/jobradar/service/job"
)

// fakeAppStore is a test double for Store.
type fakeAppStore struct {
	apps         []model.JobApplication
	appsWithJob  []ApplicationWithJob
	total        int64
	err          error
	insertCalled bool
}

func (f *fakeAppStore) GetApplicationsByJobID(jobResultID string) ([]model.JobApplication, error) {
	return f.apps, f.err
}
func (f *fakeAppStore) GetApplicationsByJobIDPaged(jobResultID string, page, limit int) ([]model.JobApplication, int64, error) {
	return f.apps, f.total, f.err
}
func (f *fakeAppStore) InsertApplication(jobResultID, stage, note string) error {
	f.insertCalled = true
	return f.err
}
func (f *fakeAppStore) GetLatestApplicationsWithJob() ([]ApplicationWithJob, error) {
	return f.appsWithJob, f.err
}
func (f *fakeAppStore) GetLatestApplicationsWithJobPaged(page, limit int) ([]ApplicationWithJob, int64, error) {
	return f.appsWithJob, f.total, f.err
}
func (f *fakeAppStore) UpdateApplicationStage(id int64, stage string) error { return f.err }
func (f *fakeAppStore) UpdateApplicationNote(id int64, note string) error   { return f.err }

// fakeJobService is a test double for job.Service.
type fakeJobService struct {
	job *model.Job
	err error

	setStatusCalled bool
}

func (f *fakeJobService) List(_ context.Context, _ jobsvc.JobFilter, _, _ int) ([]model.Job, int, error) {
	return nil, 0, f.err
}
func (f *fakeJobService) Get(_ context.Context, _ string) (*model.Job, error) { return f.job, f.err }
func (f *fakeJobService) Dismiss(_ context.Context, _ string, _ jobsvc.DismissInput) error {
	return f.err
}
func (f *fakeJobService) SetSaved(_ context.Context, _ string, _ bool) error { return f.err }
func (f *fakeJobService) SetStatus(_ context.Context, _, _ string) error {
	f.setStatusCalled = true
	return f.err
}
func (f *fakeJobService) GetCounts(_ context.Context) (*jobsvc.JobCounts, error) {
	return nil, f.err
}
func (f *fakeJobService) SetNotes(_ context.Context, _, _ string) error { return f.err }

func TestCreate_JobNotFound(t *testing.T) {
	svc := NewService(&fakeAppStore{}, &fakeJobService{
		err: domain.ErrNotFound.WithMessage("job not found"),
	})

	_, err := svc.Create(context.Background(), "missing", CreateApplicationInput{Stage: model.StageApplied})
	if err == nil {
		t.Fatal("expected error for missing job")
	}
	var sErr *domain.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND error, got %v", err)
	}
}

func TestCreate_OK(t *testing.T) {
	fs := &fakeAppStore{}
	svc := NewService(fs, &fakeJobService{job: &model.Job{ID: "j1"}})

	app, err := svc.Create(context.Background(), "j1", CreateApplicationInput{Stage: model.StagePhoneScreen})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !fs.insertCalled {
		t.Error("expected InsertApplication to be called")
	}
	if app.Stage != model.StagePhoneScreen {
		t.Errorf("expected stage %q, got %q", model.StagePhoneScreen, app.Stage)
	}
}

func TestCreate_SetsJobApplied(t *testing.T) {
	fs := &fakeAppStore{}
	jobSvc := &fakeJobService{job: &model.Job{ID: "j1"}}
	svc := NewService(fs, jobSvc)

	_, err := svc.Create(context.Background(), "j1", CreateApplicationInput{Stage: model.StageApplied})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !jobSvc.setStatusCalled {
		t.Error("expected SetStatus to be called when stage is 'applied'")
	}
}

func TestListAll_Paged(t *testing.T) {
	appsWithJob := []ApplicationWithJob{{JobTitle: "Backend Eng"}}
	svc := NewService(&fakeAppStore{appsWithJob: appsWithJob, total: 1}, &fakeJobService{})

	result, total, err := svc.ListAll(1, 20)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 app, got %d", len(result))
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
}

// Ensure test doubles implement interfaces.
var _ Store = (*fakeAppStore)(nil)
var _ jobsvc.Service = (*fakeJobService)(nil)
