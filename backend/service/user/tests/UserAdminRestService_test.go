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
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
)

// newAdminTestRouter wires the AdminRestRouter onto a fresh chi mux with an
// injectable middleware that attaches an Identity (for the grantedBy lookup in
// setRoles). Routes are mounted under /admin so the test paths match the
// production prefix; the AdminRestRouter itself adds the /users prefix. Uses
// a fresh, isolated zone config/registry per call — never
// apispec.APIZoneConfig()/AdminZoneConfig() (see apps/api/app_test.go for
// why that matters).
func newAdminTestRouter(svc usersvc.AdminService, ident identity.Identity) http.Handler {
	rest := usersvc.NewAdminRestService(svc)
	router := usersvc.NewAdminRestRouter(rest, apihuma.NewZoneConfig("test", "0.0.0", apihuma.NewSharedRegistry()))
	mux := chi.NewMux()
	mux.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := identity.NewContext(r.Context(), ident)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	router.Register(mux)
	return mux
}

// ---------- GET /users ----------

func TestAdminRest_List_HappyPath(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{
		listResult: usersvc.AdminListResult{
			Items: []usersvc.UserWithRoles{
				{Roles: []string{"user"}},
			},
			Total: 1,
		},
	}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/users?status=active&email=alice&page=2&limit=5")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Data  []usersvc.UserWithRoles `json:"data"`
		Total int                     `json:"total"`
		Page  int                     `json:"page"`
		Limit int                     `json:"limit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Total != 1 || body.Page != 2 || body.Limit != 5 {
		t.Errorf("paging fields = total=%d page=%d limit=%d, want 1/2/5", body.Total, body.Page, body.Limit)
	}
	if !repo.listCalled {
		t.Error("repo.List not invoked")
	}
}

// Items must serialise as `[]` not `null` when the repo returns a nil slice;
// the rest layer is responsible for that normalisation.
func TestAdminRest_List_NilItemsBecomesEmptyArray(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{listResult: usersvc.AdminListResult{Items: nil, Total: 0}}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/users")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), `"data":[]`) {
		t.Errorf("body = %s, want data:[] for nil items", raw)
	}
}

func TestAdminRest_List_RepoErrorMapsThroughHandleError(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{listErr: domainerrs.ErrInternal.WithMessage("boom")}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/users")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

// ---------- GET /users/{id} ----------

func TestAdminRest_Get_HappyPath(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{getResult: &usersvc.UserWithRoles{Roles: []string{"admin"}}}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/users/u1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !repo.getCalled {
		t.Error("repo.GetWithRoles not invoked")
	}
}

func TestAdminRest_Get_RepoError(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{getErr: domainerrs.ErrNotFound.WithMessage("missing")}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/users/ghost")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// ---------- PATCH /users/{id}/roles ----------

func TestAdminRest_SetRoles_HappyPath(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()

	body := strings.NewReader(`{"role_ids":["admin","user"]}`)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/users/u1/roles", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if repo.setRolesArgs.id != "u1" {
		t.Errorf("id = %q, want u1", repo.setRolesArgs.id)
	}
	if repo.setRolesArgs.grantedBy != "admin-1" {
		t.Errorf("grantedBy = %q, want admin-1 (from ctx identity)", repo.setRolesArgs.grantedBy)
	}
	if len(repo.setRolesArgs.roleIDs) != 2 {
		t.Errorf("roleIDs = %v, want [admin,user]", repo.setRolesArgs.roleIDs)
	}
}

// PATCH must accept a missing role_ids and normalise it to an empty slice so
// the repo's "clear all roles" path is exercised — instead of nil-panicking
// or surfacing "invalid request".
func TestAdminRest_SetRoles_NilRolesNormalisedToEmpty(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/users/u1/roles", strings.NewReader(`{}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if repo.setRolesArgs.roleIDs == nil {
		t.Error("roleIDs is nil; rest layer must normalise to empty slice")
	}
	if len(repo.setRolesArgs.roleIDs) != 0 {
		t.Errorf("roleIDs len = %d, want 0", len(repo.setRolesArgs.roleIDs))
	}
}

func TestAdminRest_SetRoles_MalformedBody(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/users/u1/roles", strings.NewReader("not json"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if repo.setRolesCalled {
		t.Error("repo must not run on malformed body")
	}
}

func TestAdminRest_SetRoles_RepoError(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{setRolesErr: domainerrs.ErrBadRequest.WithMessage("nope")}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/users/u1/roles", strings.NewReader(`{"role_ids":["admin"]}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// ---------- PATCH /users/{id}/status ----------

func TestAdminRest_SetStatus_HappyPath(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/users/u1/status",
		strings.NewReader(`{"status":"suspended"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if repo.setStatusArgs.id != "u1" || repo.setStatusArgs.status != "suspended" {
		t.Errorf("args = %+v, want u1/suspended", repo.setStatusArgs)
	}
}

func TestAdminRest_SetStatus_MalformedBody(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/users/u1/status", strings.NewReader("not json"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// Empty status must short-circuit with a 400 before the repo runs so the
// validation chain is consistent with the body-parse failure case.
func TestAdminRest_SetStatus_EmptyStatusReturns400(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/users/u1/status", strings.NewReader(`{}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if repo.setStatusCalled {
		t.Error("repo.SetStatus must not run when status is empty")
	}
}

func TestAdminRest_SetStatus_RepoError(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{setStatusErr: domainerrs.ErrNotFound.WithMessage("nope")}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/users/u1/status", strings.NewReader(`{"status":"active"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// ---------- DELETE /users/{id} ----------

func TestAdminRest_Delete_Returns204(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/users/u1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if repo.deleteID != "u1" {
		t.Errorf("repo deleteID = %q, want u1", repo.deleteID)
	}
}

func TestAdminRest_Delete_RepoError(t *testing.T) {
	t.Parallel()
	repo := &fakeAdminRepo{deleteErr: domainerrs.ErrNotFound.WithMessage("nope")}
	srv := httptest.NewServer(newAdminTestRouter(repo, identity.Identity{UserID: "admin-1"}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/users/ghost", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// ---------- Constructor invariants ----------

// NewAdminRestService must panic when next is nil — guards wire setup.
func TestAdminRest_NewServiceNilNextPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = usersvc.NewAdminRestService(nil)
}

// Stub helper so we can reference context.Background once via the imports
// list without unused-import errors when the test file is otherwise
// httptest-only.
var _ = context.Background
