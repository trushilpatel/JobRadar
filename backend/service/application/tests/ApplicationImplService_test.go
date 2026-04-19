package application

import (
	"context"
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/common/domain"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	svc "github.com/trushilpatel/jobradar/service/application/services"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// fakeApplicationService is a test double for ApplicationService (used as "next" for ImplService).
type fakeApplicationService struct {
	apps         []appmodel.JobApplication
	appsWithJob  []svc.ApplicationWithJob
	app          *appmodel.JobApplication
	total        int64
	err          error
	insertCalled bool
}

func (f *fakeApplicationService) ListByJob(_ string) ([]appmodel.JobApplication, error) {
	return f.apps, f.err
}
func (f *fakeApplicationService) ListAll(_, _ int) ([]svc.ApplicationWithJob, int, error) {
	return f.appsWithJob, int(f.total), f.err
}
func (f *fakeApplicationService) ListActive() ([]svc.ApplicationWithJob, error) {
	return f.appsWithJob, f.err
}
func (f *fakeApplicationService) Create(_ context.Context, _ string, _ dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	f.insertCalled = true
	return f.app, f.err
}
func (f *fakeApplicationService) UpdateStage(_, _ string, _ dto.UpdateStageInput) error { return f.err }
func (f *fakeApplicationService) UpdateNote(_, _ string, _ dto.UpdateNoteInput) error   { return f.err }

// fakeJobService is a test double for job.Service.
type fakeJobService struct {
	job *jobmodel.Job
	err error

	markAppliedCalled bool
}

func (f *fakeJobService) List(_ context.Context, _ jobdto.JobFilter, _, _ int) ([]jobmodel.Job, int, error) {
	return nil, 0, f.err
}
func (f *fakeJobService) Get(_ context.Context, _ string) (*jobmodel.Job, error) { return f.job, f.err }
func (f *fakeJobService) Dismiss(_ context.Context, _ string, _ jobdto.DismissInput) error {
	return f.err
}
func (f *fakeJobService) SetSaved(_ context.Context, _ string, _ bool) error { return f.err }
func (f *fakeJobService) SetStatus(_ context.Context, _, _ string) error     { return f.err }
func (f *fakeJobService) GetCounts(_ context.Context) (*jobdto.JobCounts, error) {
	return nil, f.err
}
func (f *fakeJobService) SetNotes(_ context.Context, _, _ string) error { return f.err }
func (f *fakeJobService) MarkApplied(_ context.Context, _ string) error {
	f.markAppliedCalled = true
	return f.err
}

func TestCreate_JobNotFound(t *testing.T) {
	next := &fakeApplicationService{}
	service := svc.NewApplicationImplService(next, &fakeJobService{
		err: domain.ErrNotFound.WithMessage("job not found"),
	})

	_, err := service.Create(context.Background(), "missing", dto.CreateApplicationInput{Stage: appcommon.StageApplied})
	if err == nil {
		t.Fatal("expected error for missing job")
	}
	var sErr *domain.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND error, got %v", err)
	}
}

func TestCreate_OK(t *testing.T) {
	next := &fakeApplicationService{
		app: &appmodel.JobApplication{JobResultID: "j1", Stage: appcommon.StagePhoneScreen},
	}
	service := svc.NewApplicationImplService(next, &fakeJobService{job: &jobmodel.Job{ID: "j1"}})

	app, err := service.Create(context.Background(), "j1", dto.CreateApplicationInput{Stage: appcommon.StagePhoneScreen})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !next.insertCalled {
		t.Error("expected Create to be called on next")
	}
	if app.Stage != appcommon.StagePhoneScreen {
		t.Errorf("expected stage %q, got %q", appcommon.StagePhoneScreen, app.Stage)
	}
}

func TestCreate_SetsJobApplied(t *testing.T) {
	next := &fakeApplicationService{
		app: &appmodel.JobApplication{JobResultID: "j1", Stage: appcommon.StageApplied},
	}
	jobService := &fakeJobService{job: &jobmodel.Job{ID: "j1"}}
	service := svc.NewApplicationImplService(next, jobService)

	_, err := service.Create(context.Background(), "j1", dto.CreateApplicationInput{Stage: appcommon.StageApplied})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !jobService.markAppliedCalled {
		t.Error("expected MarkApplied to be called when stage is 'applied'")
	}
}

func TestListAll_Paged(t *testing.T) {
	appsWithJob := []svc.ApplicationWithJob{{JobTitle: "Backend Eng"}}
	next := &fakeApplicationService{appsWithJob: appsWithJob, total: 1}
	service := svc.NewApplicationImplService(next, &fakeJobService{})

	result, total, err := service.ListAll(1, 20)
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
var _ svc.ApplicationService = (*fakeApplicationService)(nil)
var _ jobsvc.Service = (*fakeJobService)(nil)
