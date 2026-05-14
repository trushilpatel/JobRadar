package company_test

import (
	"context"
	"errors"
	"testing"

	companydto "github.com/trushilpatel/jobradar/service/company/dto"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
)

// recordingPublic counts every CompanyService call so we can verify
// BasePublicService delegates each method (and ImplService delegates through
// the same base).
type recordingPublic struct {
	listCalls, getCalls int
}

func (r *recordingPublic) ListPublic(_ context.Context, _ companydto.CompanyFilter, _, _ int) (companydto.CompanyPublicListResult, error) {
	r.listCalls++
	return companydto.CompanyPublicListResult{}, nil
}
func (r *recordingPublic) GetPublic(_ context.Context, _ string) (*companydto.CompanyPublicView, error) {
	r.getCalls++
	return nil, nil
}

// recordingAdmin counts every CompanyAdminService call so we can verify
// BaseAdminService delegates each method.
type recordingAdmin struct {
	list, get, create                      int
	updPublic, updSettings, del, setAll    int
	getByIDs, getAll                       int
	returnedSettingsErr, returnedSetAllErr error
	listResult                             companydto.CompanyAdminListResult
}

func (r *recordingAdmin) List(_ context.Context, _ companydto.CompanyFilter, _, _ int) (companydto.CompanyAdminListResult, error) {
	r.list++
	return r.listResult, nil
}
func (r *recordingAdmin) Get(_ context.Context, _ string) (*companydto.CompanyAdminView, error) {
	r.get++
	return nil, nil
}
func (r *recordingAdmin) Create(_ context.Context, _ companydto.CompanyPublicInput, _ companydto.CompanyScanSettingsInput) (*companydto.CompanyAdminView, error) {
	r.create++
	return nil, nil
}
func (r *recordingAdmin) UpdatePublic(_ context.Context, _ string, _ companydto.CompanyPublicInput) error {
	r.updPublic++
	return nil
}
func (r *recordingAdmin) UpdateScanSettings(_ context.Context, _ string, _ companydto.CompanyScanSettingsInput) error {
	r.updSettings++
	return r.returnedSettingsErr
}
func (r *recordingAdmin) Delete(_ context.Context, _ string) error {
	r.del++
	return nil
}
func (r *recordingAdmin) SetAllActive(_ context.Context, _ bool) error {
	r.setAll++
	return r.returnedSetAllErr
}
func (r *recordingAdmin) GetByIDs(_ context.Context, _ []string) ([]companydto.CompanyAdminView, error) {
	r.getByIDs++
	return nil, nil
}
func (r *recordingAdmin) GetAll(_ context.Context) ([]companydto.CompanyAdminView, error) {
	r.getAll++
	return nil, nil
}

// TestImplService_DelegatesEverything proves that the pass-through impl
// decorator forwards both public-chain methods unchanged. The decorator
// exists only as an insertion point for future business logic; today the
// behaviour contract is "do nothing extra".
func TestImplService_DelegatesEverything(t *testing.T) {
	next := &recordingPublic{}
	svc := companysvc.NewImplService(next)

	if _, err := svc.ListPublic(context.Background(), companydto.CompanyFilter{}, 1, 10); err != nil {
		t.Fatalf("ListPublic: %v", err)
	}
	if _, err := svc.GetPublic(context.Background(), "x"); err != nil {
		t.Fatalf("GetPublic: %v", err)
	}
	if next.listCalls != 1 || next.getCalls != 1 {
		t.Errorf("delegation broken: list=%d get=%d (want 1/1)", next.listCalls, next.getCalls)
	}
}

// TestNewBasePublicService_PanicsOnNil and TestNewBaseAdminService_PanicsOnNil
// lock down the wiring guard. A nil `next` is always a wire mistake, never a
// runtime condition — surfacing it as a panic in development is by design.
func TestNewBasePublicService_PanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = companysvc.NewBasePublicService(nil)
}

func TestNewBaseAdminService_PanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = companysvc.NewBaseAdminService(nil)
}

// TestBasePublicService_DelegatesEverything wraps recordingPublic in a
// BasePublicService directly (without the impl/cache decorators) to confirm
// every interface method routes to next.
func TestBasePublicService_DelegatesEverything(t *testing.T) {
	next := &recordingPublic{}
	base := companysvc.NewBasePublicService(next)

	_, _ = base.ListPublic(context.Background(), companydto.CompanyFilter{}, 1, 10)
	_, _ = base.GetPublic(context.Background(), "x")

	if next.listCalls != 1 || next.getCalls != 1 {
		t.Errorf("base public delegation broken: list=%d get=%d", next.listCalls, next.getCalls)
	}
}

// TestBaseAdminService_DelegatesEverything wraps recordingAdmin in a bare
// BaseAdminService and calls every interface method — every counter must
// increment exactly once.
func TestBaseAdminService_DelegatesEverything(t *testing.T) {
	next := &recordingAdmin{}
	base := companysvc.NewBaseAdminService(next)

	_, _ = base.List(context.Background(), companydto.CompanyFilter{}, 1, 10)
	_, _ = base.Get(context.Background(), "x")
	_, _ = base.Create(context.Background(), companydto.CompanyPublicInput{}, companydto.CompanyScanSettingsInput{})
	_ = base.UpdatePublic(context.Background(), "x", companydto.CompanyPublicInput{})
	_ = base.UpdateScanSettings(context.Background(), "x", companydto.CompanyScanSettingsInput{})
	_ = base.Delete(context.Background(), "x")
	_ = base.SetAllActive(context.Background(), true)
	_, _ = base.GetByIDs(context.Background(), []string{"x"})
	_, _ = base.GetAll(context.Background())

	counts := []struct {
		name string
		got  int
	}{
		{"List", next.list},
		{"Get", next.get},
		{"Create", next.create},
		{"UpdatePublic", next.updPublic},
		{"UpdateScanSettings", next.updSettings},
		{"Delete", next.del},
		{"SetAllActive", next.setAll},
		{"GetByIDs", next.getByIDs},
		{"GetAll", next.getAll},
	}
	for _, c := range counts {
		if c.got != 1 {
			t.Errorf("%s call count = %d, want 1", c.name, c.got)
		}
	}
}

// TestAdminImpl_SetAllActiveAndQueriesDelegate confirms the AdminImpl decorator
// pass-through behaviour for the methods it does NOT override — List, Get,
// SetAllActive, GetByIDs, GetAll — and that UpdateScanSettings reaches next.
func TestAdminImpl_SetAllActiveAndQueriesDelegate(t *testing.T) {
	next := &recordingAdmin{}
	svc := companysvc.NewAdminImplService(next, nil)

	_, _ = svc.List(context.Background(), companydto.CompanyFilter{}, 1, 10)
	_, _ = svc.Get(context.Background(), "x")
	_ = svc.SetAllActive(context.Background(), true)
	_, _ = svc.GetByIDs(context.Background(), []string{"a"})
	_, _ = svc.GetAll(context.Background())
	_ = svc.UpdateScanSettings(context.Background(), "x", companydto.CompanyScanSettingsInput{})

	if next.list != 1 || next.get != 1 || next.setAll != 1 || next.getByIDs != 1 || next.getAll != 1 || next.updSettings != 1 {
		t.Errorf("delegation broken: %+v", next)
	}
}

// TestAdminImpl_Create_NextErrorBubbles confirms that when the underlying
// admin service errors out, the cache invalidation step is skipped and the
// caller still receives the error.
func TestAdminImpl_Create_NextErrorBubbles(t *testing.T) {
	boom := errors.New("create failed")
	fake := &fakeAdmin{createErr: boom}
	svc := companysvc.NewAdminImplService(fake, nil)
	if _, err := svc.Create(context.Background(), companydto.CompanyPublicInput{ID: "x"}, companydto.CompanyScanSettingsInput{}); !errors.Is(err, boom) {
		t.Errorf("expected wrapped boom, got %v", err)
	}
}

// TestAdminImpl_UpdatePublic_NextErrorBubbles asserts the post-write cache
// delete is skipped on error — but the pre-write delete is best-effort, so
// the err returned from next still surfaces to the caller.
func TestAdminImpl_UpdatePublic_NextErrorBubbles(t *testing.T) {
	boom := errors.New("update failed")
	fake := &fakeAdmin{updateErr: boom}
	svc := companysvc.NewAdminImplService(fake, nil)
	if err := svc.UpdatePublic(context.Background(), "x", companydto.CompanyPublicInput{Name: "y"}); !errors.Is(err, boom) {
		t.Errorf("expected wrapped boom, got %v", err)
	}
}

// TestAdminImpl_Delete_NextErrorBubbles is the symmetric Delete check.
func TestAdminImpl_Delete_NextErrorBubbles(t *testing.T) {
	boom := errors.New("delete failed")
	fake := &fakeAdmin{deleteErr: boom}
	svc := companysvc.NewAdminImplService(fake, nil)
	if err := svc.Delete(context.Background(), "x"); !errors.Is(err, boom) {
		t.Errorf("expected wrapped boom, got %v", err)
	}
}
