package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
)

// stubService is a hand-rolled Service double tailored to handler tests.
// We don't reuse the test fakes from sibling files because each test file
// needs slightly different observability hooks.
type stubService struct {
	updatedID      string
	updatedName    string
	updatedErr     error
	deletedID      string
	deletedErr     error
	configGet      *usermodel.UserConfig
	configGetErr   error
	configPatch    *usermodel.UserConfig
	configPatchErr error
}

func (s *stubService) Get(_ context.Context, _ string) (*usermodel.User, error) {
	return nil, nil
}
func (s *stubService) GetByEmail(_ context.Context, _ string) (*usermodel.User, error) {
	return nil, nil
}
func (s *stubService) Provision(_ context.Context, _, _ string) (*usermodel.User, error) {
	return nil, nil
}
func (s *stubService) UpdateProfile(_ context.Context, id, name string) error {
	s.updatedID = id
	s.updatedName = name
	return s.updatedErr
}
func (s *stubService) Delete(_ context.Context, id string) error {
	s.deletedID = id
	return s.deletedErr
}
func (s *stubService) GetConfig(_ context.Context, _ string) (*usermodel.UserConfig, error) {
	if s.configGetErr != nil {
		return nil, s.configGetErr
	}
	if s.configGet != nil {
		return s.configGet, nil
	}
	return &usermodel.UserConfig{}, nil
}
func (s *stubService) PatchConfig(_ context.Context, _ string, partial usermodel.UserConfig) (*usermodel.UserConfig, error) {
	if s.configPatchErr != nil {
		return nil, s.configPatchErr
	}
	if s.configPatch != nil {
		return s.configPatch, nil
	}
	return &partial, nil
}

// newTestRouter wires the user RestRouter behind an injectable identity
// middleware so tests can attach an Identity to ctx without a full JWT.
func newTestRouter(svc usersvc.Service, ident identity.Identity) http.Handler {
	rest := usersvc.NewRestService(svc)
	attach := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := identity.NewContext(r.Context(), ident)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	router := usersvc.NewRestRouter(rest, attach)
	mux := chi.NewMux()
	router.Register(mux)
	return mux
}

func TestGetMeReturnsIdentityBundle(t *testing.T) {
	t.Parallel()
	ident := identity.Identity{
		UserID:      "u1",
		Email:       "alice@example.com",
		Roles:       []string{"user", "admin"},
		Status:      "active",
		DisplayName: "Alice",
	}
	srv := httptest.NewServer(newTestRouter(&stubService{}, ident))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/me/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Data struct {
			UserID      string   `json:"user_id"`
			Email       string   `json:"email"`
			Roles       []string `json:"roles"`
			Status      string   `json:"status"`
			DisplayName string   `json:"display_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.UserID != "u1" || body.Data.Email != "alice@example.com" {
		t.Errorf("unexpected body: %+v", body.Data)
	}
	if len(body.Data.Roles) != 2 {
		t.Errorf("roles = %v, want 2", body.Data.Roles)
	}
}

func TestPatchMeUpdatesProfile(t *testing.T) {
	t.Parallel()
	svc := &stubService{}
	ident := identity.Identity{UserID: "u1", Email: "a@b.com", Status: "active"}
	srv := httptest.NewServer(newTestRouter(svc, ident))
	defer srv.Close()

	body := strings.NewReader(`{"display_name":"Alice"}`)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/me/", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if svc.updatedID != "u1" {
		t.Errorf("updated id = %q, want u1", svc.updatedID)
	}
	if svc.updatedName != "Alice" {
		t.Errorf("updated name = %q", svc.updatedName)
	}
}

func TestPatchMeRejectsMalformedBody(t *testing.T) {
	t.Parallel()
	svc := &stubService{}
	ident := identity.Identity{UserID: "u1"}
	srv := httptest.NewServer(newTestRouter(svc, ident))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/me/", strings.NewReader("not json"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestDeleteMeReturns204(t *testing.T) {
	t.Parallel()
	svc := &stubService{}
	ident := identity.Identity{UserID: "u1"}
	srv := httptest.NewServer(newTestRouter(svc, ident))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/me/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if svc.deletedID != "u1" {
		t.Errorf("delete id = %q, want u1", svc.deletedID)
	}
}

// DELETE /api/me must surface the service error to the caller via the
// standard HandleError path — a typed *ServiceError becomes its mapped
// HTTP status, not a silent 204.
func TestDeleteMeServiceError(t *testing.T) {
	t.Parallel()
	svc := &stubService{deletedErr: errBadRequest("nope")}
	ident := identity.Identity{UserID: "u1"}
	srv := httptest.NewServer(newTestRouter(svc, ident))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/me/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// PATCH /api/me must surface the service error via HandleError — e.g.
// validation errors → 400.
func TestPatchMeServiceError(t *testing.T) {
	t.Parallel()
	svc := &stubService{updatedErr: errBadRequest("nope")}
	ident := identity.Identity{UserID: "u1"}
	srv := httptest.NewServer(newTestRouter(svc, ident))
	defer srv.Close()

	body := strings.NewReader(`{"display_name":"Alice"}`)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/me/", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// GET /api/me must serialise an empty roles slice as `[]`, not `null`, so
// the frontend can rely on roles being iterable. The same handler also
// covers the case where Identity.Roles starts as nil — make sure that
// branch is exercised.
func TestGetMeNilRolesBecomesEmptyArray(t *testing.T) {
	t.Parallel()
	ident := identity.Identity{UserID: "u1", Roles: nil}
	srv := httptest.NewServer(newTestRouter(&stubService{}, ident))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/me/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		Data struct {
			Roles []string `json:"roles"`
		} `json:"data"`
	}
	raw, _ := io.ReadAll(resp.Body)
	if err := json.NewDecoder(strings.NewReader(string(raw))).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Roles == nil {
		t.Errorf("roles is nil, want empty array; raw=%s", raw)
	}
	if !strings.Contains(string(raw), `"roles":[]`) {
		t.Errorf("body lacks 'roles:[]': %s", raw)
	}
}

// errBadRequest wraps the sentinel BAD_REQUEST so RestService tests can
// surface a typed ServiceError without spelling out the package name at every
// call site.
func errBadRequest(msg string) error {
	return domainerrs.ErrBadRequest.WithMessage(msg)
}

// ---------- GET /me/config ----------

// A user with no stored preferences must receive the zero-value defaults —
// show_keyboard_shortcuts = false.
func TestGetMyConfigReturnsDefault(t *testing.T) {
	t.Parallel()
	svc := &stubService{} // configGet is nil → returns &UserConfig{}
	ident := identity.Identity{UserID: "u1", Email: "a@b.com", Status: "active"}
	srv := httptest.NewServer(newTestRouter(svc, ident))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/me/config")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Data usermodel.UserConfig `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.ShowKeyboardShortcuts {
		t.Errorf("show_keyboard_shortcuts = true, want false (default)")
	}
}

// A user with stored preferences must have them returned verbatim.
func TestGetMyConfigReturnsExistingConfig(t *testing.T) {
	t.Parallel()
	svc := &stubService{
		configGet: &usermodel.UserConfig{ShowKeyboardShortcuts: true},
	}
	ident := identity.Identity{UserID: "u1", Email: "a@b.com", Status: "active"}
	srv := httptest.NewServer(newTestRouter(svc, ident))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/me/config")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Data usermodel.UserConfig `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Data.ShowKeyboardShortcuts {
		t.Errorf("show_keyboard_shortcuts = false, want true")
	}
}

// ---------- PATCH /me/config ----------

// A well-formed PATCH body must be forwarded to the service PatchConfig and
// the merged result returned to the caller.
func TestPatchMyConfigCallsServiceWithBody(t *testing.T) {
	t.Parallel()
	svc := &stubService{
		configPatch: &usermodel.UserConfig{ShowKeyboardShortcuts: true},
	}
	ident := identity.Identity{UserID: "u1", Email: "a@b.com", Status: "active"}
	srv := httptest.NewServer(newTestRouter(svc, ident))
	defer srv.Close()

	body := strings.NewReader(`{"show_keyboard_shortcuts":true}`)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/me/config", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Data usermodel.UserConfig `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.Data.ShowKeyboardShortcuts {
		t.Errorf("show_keyboard_shortcuts = false, want true")
	}
}

// A malformed JSON body must return 400 immediately without calling the
// service.
func TestPatchMyConfigRejectsMalformedBody(t *testing.T) {
	t.Parallel()
	svc := &stubService{}
	ident := identity.Identity{UserID: "u1"}
	srv := httptest.NewServer(newTestRouter(svc, ident))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/me/config", strings.NewReader("not json {{{"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// A service error on PATCH /me/config must be surfaced as the appropriate
// HTTP status via HandleError.
func TestPatchMyConfigServiceError(t *testing.T) {
	t.Parallel()
	svc := &stubService{configPatchErr: errBadRequest("cannot update")}
	ident := identity.Identity{UserID: "u1"}
	srv := httptest.NewServer(newTestRouter(svc, ident))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/me/config", strings.NewReader(`{"show_keyboard_shortcuts":false}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// A service error on GET /me/config must be surfaced via HandleError.
func TestGetMyConfigServiceError(t *testing.T) {
	t.Parallel()
	svc := &stubService{configGetErr: errBadRequest("config unavailable")}
	ident := identity.Identity{UserID: "u1"}
	srv := httptest.NewServer(newTestRouter(svc, ident))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/me/config")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
