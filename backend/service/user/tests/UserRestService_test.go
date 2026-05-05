package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trushilpatel/jobradar/common/identity"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
)

// stubService is a hand-rolled Service double tailored to handler tests.
// We don't reuse the test fakes from sibling files because each test file
// needs slightly different observability hooks.
type stubService struct {
	updatedID   string
	updatedName string
	updatedErr  error
	deletedID   string
	deletedErr  error
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
