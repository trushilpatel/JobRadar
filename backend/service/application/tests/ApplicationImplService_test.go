package application

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	svc "github.com/trushilpatel/jobradar/service/application/services"
	jobdto "github.com/trushilpatel/jobradar/service/job/dto"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
)

const testUserID = "u-1"

// fakeRepo is a test double for ApplicationRepository.
type fakeRepo struct {
	apps        []appmodel.JobApplication
	appsWithJob []svc.ApplicationWithJob
	app         *appmodel.JobApplication
	total       int
	err         error

	gotUserID string
	calls     int
}

func (f *fakeRepo) ListByJob(_ context.Context, userID, _ string) ([]appmodel.JobApplication, error) {
	f.gotUserID = userID
	f.calls++
	return f.apps, f.err
}
func (f *fakeRepo) ListAll(_ context.Context, userID string, _, _ int) ([]svc.ApplicationWithJob, int, error) {
	f.gotUserID = userID
	f.calls++
	return f.appsWithJob, f.total, f.err
}
func (f *fakeRepo) ListActive(_ context.Context, userID string) ([]svc.ApplicationWithJob, error) {
	f.gotUserID = userID
	f.calls++
	return f.appsWithJob, f.err
}
func (f *fakeRepo) Create(_ context.Context, userID, _ string, _ dto.CreateApplicationInput) (*appmodel.JobApplication, error) {
	f.gotUserID = userID
	f.calls++
	return f.app, f.err
}
func (f *fakeRepo) UpdateStage(_ context.Context, userID, _ string, _ dto.UpdateStageInput) error {
	f.gotUserID = userID
	f.calls++
	return f.err
}
func (f *fakeRepo) UpdateNote(_ context.Context, userID, _ string, _ dto.UpdateNoteInput) error {
	f.gotUserID = userID
	f.calls++
	return f.err
}

// fakeJobService is a minimal test double for jobsvc.JobService — only Get
// is exercised by the application impl service.
type fakeJobService struct {
	view *jobdto.JobView
	err  error
}

func (f *fakeJobService) List(_ context.Context, _ jobdto.JobFilter, _ string, _ int) (jobdto.PageResult, error) {
	return jobdto.PageResult{}, f.err
}
func (f *fakeJobService) Get(_ context.Context, _ string) (*jobdto.JobView, error) {
	return f.view, f.err
}
func (f *fakeJobService) SetSaved(_ context.Context, _ string, _ bool) error { return f.err }
func (f *fakeJobService) SetStatus(_ context.Context, _, _ string) error     { return f.err }
func (f *fakeJobService) SetNotes(_ context.Context, _, _ string) error      { return f.err }
func (f *fakeJobService) Dismiss(_ context.Context, _, _, _ string) error    { return f.err }
func (f *fakeJobService) BulkDismiss(_ context.Context, _ []string, _, _ string) error {
	return f.err
}
func (f *fakeJobService) GetCounts(_ context.Context) (jobdto.Counts, error) {
	return jobdto.Counts{}, f.err
}

// ctxAs returns a ctx carrying a synthetic Identity for userID.
func ctxAs(userID string) context.Context {
	return identity.WithTestIdentity(context.Background(), userID, "user")
}

func TestImpl_Create_JobNotFound(t *testing.T) {
	repo := &fakeRepo{}
	jobs := &fakeJobService{err: apperr.ErrNotFound.WithMessage("job not found")}
	service := svc.NewApplicationImplService(repo, jobs)

	_, err := service.Create(ctxAs(testUserID), "j1", dto.CreateApplicationInput{Stage: appcommon.StageApplied})
	if err == nil {
		t.Fatal("expected error for missing job")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND error, got %v", err)
	}
	if repo.calls != 0 {
		t.Errorf("expected repo.Create not to be called when job missing, calls=%d", repo.calls)
	}
}

func TestImpl_Create_OK(t *testing.T) {
	repo := &fakeRepo{
		app: &appmodel.JobApplication{UserID: testUserID, JobResultID: "j1", Stage: appcommon.StagePhoneScreen},
	}
	service := svc.NewApplicationImplService(repo, &fakeJobService{view: &jobdto.JobView{}})

	app, err := service.Create(ctxAs(testUserID), "j1", dto.CreateApplicationInput{Stage: appcommon.StagePhoneScreen})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repo.gotUserID != testUserID {
		t.Errorf("expected repo to receive userID %q, got %q", testUserID, repo.gotUserID)
	}
	if app.Stage != appcommon.StagePhoneScreen {
		t.Errorf("expected stage %q, got %q", appcommon.StagePhoneScreen, app.Stage)
	}
}

func TestImpl_Create_NoIdentity(t *testing.T) {
	repo := &fakeRepo{}
	service := svc.NewApplicationImplService(repo, &fakeJobService{view: &jobdto.JobView{}})

	_, err := service.Create(context.Background(), "j1", dto.CreateApplicationInput{Stage: appcommon.StageApplied})
	if err == nil {
		t.Fatal("expected error for missing identity")
	}
	if repo.calls != 0 {
		t.Errorf("repo must not be called without identity, calls=%d", repo.calls)
	}
}

func TestImpl_ListAll_PassesUserID(t *testing.T) {
	repo := &fakeRepo{appsWithJob: []svc.ApplicationWithJob{{JobTitle: "Backend Eng"}}, total: 1}
	service := svc.NewApplicationImplService(repo, &fakeJobService{})

	result, total, err := service.ListAll(ctxAs(testUserID), 1, 20)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(result) != 1 || total != 1 {
		t.Errorf("unexpected result/total: %d/%d", len(result), total)
	}
	if repo.gotUserID != testUserID {
		t.Errorf("expected userID %q passed to repo, got %q", testUserID, repo.gotUserID)
	}
}

func TestImpl_ListByJob_NoIdentity(t *testing.T) {
	repo := &fakeRepo{}
	service := svc.NewApplicationImplService(repo, &fakeJobService{})

	_, err := service.ListByJob(context.Background(), "j1")
	if err == nil {
		t.Fatal("expected error for missing identity")
	}
}

// TestImpl_CrossUserIsolation verifies the impl service forwards each
// caller's own userID to the repo — user A's call cannot end up using
// user B's identity, and the repo receives them distinctly.
func TestImpl_CrossUserIsolation(t *testing.T) {
	repo := &fakeRepo{appsWithJob: []svc.ApplicationWithJob{}}
	service := svc.NewApplicationImplService(repo, &fakeJobService{})

	if _, _, err := service.ListAll(ctxAs("user-A"), 1, 20); err != nil {
		t.Fatalf("user A ListAll: %v", err)
	}
	if repo.gotUserID != "user-A" {
		t.Errorf("repo got %q for user A", repo.gotUserID)
	}

	if _, _, err := service.ListAll(ctxAs("user-B"), 1, 20); err != nil {
		t.Fatalf("user B ListAll: %v", err)
	}
	if repo.gotUserID != "user-B" {
		t.Errorf("repo got %q for user B", repo.gotUserID)
	}
}

// Ensure test doubles implement the production interfaces.
var _ svc.ApplicationRepository = (*fakeRepo)(nil)
var _ jobsvc.JobService = (*fakeJobService)(nil)
