package supabaseadmin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/platform/supabaseadmin"
)

func TestNewRESTClient_Validation(t *testing.T) {
	if _, err := supabaseadmin.NewRESTClient("", "key", nil); err == nil {
		t.Fatal("empty baseURL: want err")
	}
	if _, err := supabaseadmin.NewRESTClient("https://x.supabase.co", "", nil); err == nil {
		t.Fatal("empty serviceKey: want err")
	}
	if _, err := supabaseadmin.NewRESTClient("https://x.supabase.co", "key", nil); err != nil {
		t.Fatalf("valid args: unexpected err %v", err)
	}
}

func TestDeleteUser_RequestShapeAndSuccess(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotAuth   string
		gotAPIKey string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("apikey")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := supabaseadmin.NewRESTClient(srv.URL, "service-key", srv.Client())
	if err != nil {
		t.Fatalf("NewRESTClient: %v", err)
	}

	if err := client.DeleteUser(context.Background(), "user-123"); err != nil {
		t.Fatalf("DeleteUser: unexpected err %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("method: got %q, want DELETE", gotMethod)
	}
	if gotPath != "/auth/v1/admin/users/user-123" {
		t.Errorf("path: got %q", gotPath)
	}
	if gotAuth != "Bearer service-key" {
		t.Errorf("Authorization: got %q", gotAuth)
	}
	if gotAPIKey != "service-key" {
		t.Errorf("apikey: got %q", gotAPIKey)
	}
}

func TestDeleteUser_NotFoundWrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"msg":"User not found"}`))
	}))
	defer srv.Close()

	client, _ := supabaseadmin.NewRESTClient(srv.URL, "k", srv.Client())
	err := client.DeleteUser(context.Background(), "missing-user")
	if err == nil {
		t.Fatal("DeleteUser on 404: want err")
	}
	if !errors.Is(err, supabaseadmin.ErrUserNotFound) {
		t.Fatalf("DeleteUser on 404: want ErrUserNotFound wrapped, got %v", err)
	}
}

func TestDeleteUser_ServerErrorReturnsContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	client, _ := supabaseadmin.NewRESTClient(srv.URL, "k", srv.Client())
	err := client.DeleteUser(context.Background(), "u")
	if err == nil {
		t.Fatal("DeleteUser on 500: want err")
	}
	if errors.Is(err, supabaseadmin.ErrUserNotFound) {
		t.Fatal("DeleteUser on 500: must NOT match ErrUserNotFound")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status 500 in err: %v", err)
	}
}

func TestDeleteUser_EmptyUserID(t *testing.T) {
	client, _ := supabaseadmin.NewRESTClient("https://x.supabase.co", "k", http.DefaultClient)
	if err := client.DeleteUser(context.Background(), ""); err == nil {
		t.Fatal("empty userID: want err")
	}
}
