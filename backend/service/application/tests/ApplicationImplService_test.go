package application

import (
	"context"
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/domain"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	jobmodel "github.com/trushilpatel/jobradar/service/job/models"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	svc "github.com/trushilpatel/jobradar/service/application/services"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

// fakeRepository is a test double for ApplicationRepositoryService.
type fakeRepository struct {
	apps         []appmodel.JobApplication
	appsWithJob  []svc.ApplicationWithJob
	total        int64
	err          error
	insertCalled bool
}

func (f *fakeRepository) GetApplicationsByJobID(jobResultID string) ([]appmodel.JobApplication, error) {
	return f.apps, f.err
}
func (f *fakeRepository) GetApplicationsByJobIDPaged(jobResultID string, page, limit int) ([]appmodel.JobApplication, int64, error) {
	return f.apps, f.total, f.err
}
func (f *fakeRepository) InsertApplication(jobResultID, stage, note string) error {
	f.insertCalled = true
	return f.err
}
func (f *fakeRepository) InsertAppliedApplication(_ string) error      { return f.err }
func (f *fakeRepository) InsertInterviewingApplication(_ string) error { return f.err }
func (f *fakeRepository) InsertOfferApplication(_ string) error        { return f.err }
func (f *fakeRepository) GetLatestApplicationsWithJob() ([]svc.ApplicationWithJob, error) {
	return f.appsWithJob, f.err
}
func (f *fakeRepository) GetLatestApplicationsWithJobPaged(page, limit int) ([]svc.ApplicationWithJob, int64, error) {
	return f.appsWithJob, f.total, f.err
}
func (f *fakeRepository) UpdateApplicationStage(id int64, stage string) error { return f.err }
func (f *fakeRepository) UpdateApplicationNote(id int64, note string) error   { return f.err }

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
	service := svc.NewApplicationImplService(&fakeRepository{}, &fakeJobService{
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
	fr := &fakeRepository{}
	service := svc.NewApplicationImplService(fr, &fakeJobService{job: &jobmodel.Job{ID: "j1"}})

	app, err := service.Create(context.Background(), "j1", dto.CreateApplicationInput{Stage: appcommon.StagePhoneScreen})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !fr.insertCalled {
		t.Error("expected InsertApplication to be called")
	}
	if app.Stage != appcommon.StagePhoneScreen {
		t.Errorf("expected stage %q, got %q", appcommon.StagePhoneScreen, app.Stage)
	}
}

func TestCreate_SetsJobApplied(t *testing.T) {
	fr := &fakeRepository{}
	jobService := &fakeJobService{job: &jobmodel.Job{ID: "j1"}}
	service := svc.NewApplicationImplService(fr, jobService)

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
	service := svc.NewApplicationImplService(&fakeRepository{appsWithJob: appsWithJob, total: 1}, &fakeJobService{})

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
var _ svc.ApplicationRepositoryService = (*fakeRepository)(nil)
var _ jobsvc.Service = (*fakeJobService)(nil)
