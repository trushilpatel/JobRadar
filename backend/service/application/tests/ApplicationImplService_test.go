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
func (f *fakeJobService) GetCompanies(_ context.Context) ([]string, error) { return nil, nil }

// ctxAs returns a ctx carrying a synthetic Identity for userID.
func ctxAs(userID string) context.Context {
	return identity.WithTestIdentity(context.Background(), userID, "user")
}

func TestImpl_Create_JobNotFound(t *testing.T) {
	repo := &fakeRepo{}
	jobs := &fakeJobService{err: apperr.ErrNotFound.WithMessage("job not found")}
	service := svc.NewImplService(repo, jobs)

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
	service := svc.NewImplService(repo, &fakeJobService{view: &jobdto.JobView{}})

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
	service := svc.NewImplService(repo, &fakeJobService{view: &jobdto.JobView{}})

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
	service := svc.NewImplService(repo, &fakeJobService{})

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
	service := svc.NewImplService(repo, &fakeJobService{})

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
	service := svc.NewImplService(repo, &fakeJobService{})

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

// TestImpl_Create_RepoError_WrappedAsInternal — the impl service must NOT
// surface raw repo errors to callers; any failure after the job-exists check
// is mapped to ErrInternal.WithMessage("failed to create application") so
// handlers see a stable typed error.
func TestImpl_Create_RepoError_WrappedAsInternal(t *testing.T) {
	repo := &fakeRepo{err: errors.New("boom")}
	service := svc.NewImplService(repo, &fakeJobService{view: &jobdto.JobView{}})

	_, err := service.Create(ctxAs(testUserID), "j1", dto.CreateApplicationInput{Stage: appcommon.StageApplied})
	if err == nil {
		t.Fatal("expected error from repo")
	}
	var sErr *apperr.ServiceError
	if !errors.As(err, &sErr) {
		t.Fatalf("expected *ServiceError, got %T (%v)", err, err)
	}
	if sErr.Code != "INTERNAL" {
		t.Errorf("expected INTERNAL code, got %q", sErr.Code)
	}
	if repo.calls != 1 {
		t.Errorf("expected repo.Create to be called once, got %d", repo.calls)
	}
}

func TestImpl_ListAll_NoIdentity(t *testing.T) {
	repo := &fakeRepo{}
	service := svc.NewImplService(repo, &fakeJobService{})

	_, _, err := service.ListAll(context.Background(), 1, 20)
	if err == nil {
		t.Fatal("expected error for missing identity")
	}
	if repo.calls != 0 {
		t.Errorf("repo must not be called without identity, calls=%d", repo.calls)
	}
}

func TestImpl_ListByJob_OK(t *testing.T) {
	repo := &fakeRepo{apps: []appmodel.JobApplication{{UserID: testUserID, JobResultID: "j1"}}}
	service := svc.NewImplService(repo, &fakeJobService{})

	got, err := service.ListByJob(ctxAs(testUserID), "j1")
	if err != nil {
		t.Fatalf("ListByJob() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if repo.gotUserID != testUserID {
		t.Errorf("expected repo to receive userID %q, got %q", testUserID, repo.gotUserID)
	}
}

func TestImpl_ListByJob_RepoError(t *testing.T) {
	repo := &fakeRepo{err: errors.New("db down")}
	service := svc.NewImplService(repo, &fakeJobService{})

	_, err := service.ListByJob(ctxAs(testUserID), "j1")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestImpl_ListActive_OK(t *testing.T) {
	repo := &fakeRepo{appsWithJob: []svc.ApplicationWithJob{{JobTitle: "BE"}}}
	service := svc.NewImplService(repo, &fakeJobService{})

	got, err := service.ListActive(ctxAs(testUserID))
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if repo.gotUserID != testUserID {
		t.Errorf("expected userID %q, got %q", testUserID, repo.gotUserID)
	}
}

func TestImpl_ListActive_NoIdentity(t *testing.T) {
	repo := &fakeRepo{}
	service := svc.NewImplService(repo, &fakeJobService{})

	_, err := service.ListActive(context.Background())
	if err == nil {
		t.Fatal("expected error for missing identity")
	}
	if repo.calls != 0 {
		t.Errorf("repo must not be called without identity, calls=%d", repo.calls)
	}
}

func TestImpl_UpdateStage_OK(t *testing.T) {
	repo := &fakeRepo{}
	service := svc.NewImplService(repo, &fakeJobService{})

	if err := service.UpdateStage(ctxAs(testUserID), "j1", "1", dto.UpdateStageInput{Stage: appcommon.StageOffer}); err != nil {
		t.Fatalf("UpdateStage() error = %v", err)
	}
	if repo.gotUserID != testUserID {
		t.Errorf("expected userID %q, got %q", testUserID, repo.gotUserID)
	}
	if repo.calls != 1 {
		t.Errorf("expected repo.UpdateStage to be called once, got %d", repo.calls)
	}
}

func TestImpl_UpdateStage_NoIdentity(t *testing.T) {
	repo := &fakeRepo{}
	service := svc.NewImplService(repo, &fakeJobService{})

	err := service.UpdateStage(context.Background(), "j1", "1", dto.UpdateStageInput{Stage: appcommon.StageOffer})
	if err == nil {
		t.Fatal("expected error for missing identity")
	}
	if repo.calls != 0 {
		t.Errorf("repo must not be called without identity, calls=%d", repo.calls)
	}
}

func TestImpl_UpdateStage_RepoError(t *testing.T) {
	repo := &fakeRepo{err: errors.New("db")}
	service := svc.NewImplService(repo, &fakeJobService{})

	if err := service.UpdateStage(ctxAs(testUserID), "j1", "1", dto.UpdateStageInput{Stage: appcommon.StageOffer}); err == nil {
		t.Fatal("expected repo error to propagate")
	}
}

func TestImpl_UpdateStageByID_OK(t *testing.T) {
	repo := &fakeRepo{}
	service := svc.NewImplService(repo, &fakeJobService{})

	if err := service.UpdateStageByID(ctxAs(testUserID), "42", dto.UpdateStageInput{Stage: appcommon.StageOffer}); err != nil {
		t.Fatalf("UpdateStageByID() error = %v", err)
	}
	if repo.gotUserID != testUserID {
		t.Errorf("expected userID %q, got %q", testUserID, repo.gotUserID)
	}
	if repo.calls != 1 {
		t.Errorf("expected repo.UpdateStage to be called once via UpdateStageByID, got %d", repo.calls)
	}
}

func TestImpl_UpdateStageByID_NoIdentity(t *testing.T) {
	repo := &fakeRepo{}
	service := svc.NewImplService(repo, &fakeJobService{})

	err := service.UpdateStageByID(context.Background(), "42", dto.UpdateStageInput{Stage: appcommon.StageOffer})
	if err == nil {
		t.Fatal("expected error for missing identity")
	}
	if repo.calls != 0 {
		t.Errorf("repo must not be called without identity, calls=%d", repo.calls)
	}
}

func TestImpl_UpdateNote_OK(t *testing.T) {
	repo := &fakeRepo{}
	service := svc.NewImplService(repo, &fakeJobService{})

	if err := service.UpdateNote(ctxAs(testUserID), "j1", "1", dto.UpdateNoteInput{Note: "hello"}); err != nil {
		t.Fatalf("UpdateNote() error = %v", err)
	}
	if repo.gotUserID != testUserID {
		t.Errorf("expected userID %q, got %q", testUserID, repo.gotUserID)
	}
	if repo.calls != 1 {
		t.Errorf("expected repo.UpdateNote to be called once, got %d", repo.calls)
	}
}

func TestImpl_UpdateNote_NoIdentity(t *testing.T) {
	repo := &fakeRepo{}
	service := svc.NewImplService(repo, &fakeJobService{})

	err := service.UpdateNote(context.Background(), "j1", "1", dto.UpdateNoteInput{Note: "x"})
	if err == nil {
		t.Fatal("expected error for missing identity")
	}
	if repo.calls != 0 {
		t.Errorf("repo must not be called without identity, calls=%d", repo.calls)
	}
}

func TestImpl_UpdateNote_RepoError(t *testing.T) {
	repo := &fakeRepo{err: errors.New("db")}
	service := svc.NewImplService(repo, &fakeJobService{})

	if err := service.UpdateNote(ctxAs(testUserID), "j1", "1", dto.UpdateNoteInput{Note: "x"}); err == nil {
		t.Fatal("expected repo error to propagate")
	}
}

// TestImpl_Constructor_PanicsOnNilRepo guards the wire setup — a nil repo
// is always a wiring error and must fail loudly at startup, never at the
// first request.
func TestImpl_Constructor_PanicsOnNilRepo(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil repo")
		}
	}()
	_ = svc.NewImplService(nil, &fakeJobService{})
}

// TestImpl_Constructor_PanicsOnNilJobSvc — same as above for the job service.
func TestImpl_Constructor_PanicsOnNilJobSvc(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil jobSvc")
		}
	}()
	_ = svc.NewImplService(&fakeRepo{}, nil)
}

// Ensure test doubles implement the production interfaces.
var _ svc.ApplicationRepository = (*fakeRepo)(nil)
var _ jobsvc.JobService = (*fakeJobService)(nil)
