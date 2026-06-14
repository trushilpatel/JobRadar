package clerkadmin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trushilpatel/jobradar/platform/clerkadmin"
)

func TestNewRESTClient_EmptySecretKey_ReturnsError(t *testing.T) {
	_, err := clerkadmin.NewRESTClient("https://api.clerk.dev", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secretKey")
}

func TestNewRESTClient_EmptyBaseURL_ReturnsError(t *testing.T) {
	_, err := clerkadmin.NewRESTClient("", "sk_test_abc", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "baseURL")
}

func TestNewRESTClient_ValidParams_Succeeds(t *testing.T) {
	c, err := clerkadmin.NewRESTClient("https://api.clerk.dev", "sk_test_abc", nil)
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestDeleteUser_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/v1/users/user_abc123", r.URL.Path)
		assert.Equal(t, "Bearer sk_test_secret", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := clerkadmin.NewRESTClient(srv.URL, "sk_test_secret", nil)
	require.NoError(t, err)

	err = c.DeleteUser(context.Background(), "user_abc123")
	assert.NoError(t, err)
}

func TestDeleteUser_NotFound_ReturnsErrUserNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := clerkadmin.NewRESTClient(srv.URL, "sk_test_secret", nil)
	require.NoError(t, err)

	err = c.DeleteUser(context.Background(), "user_gone")
	require.Error(t, err)
	assert.True(t, errors.Is(err, clerkadmin.ErrUserNotFound))
}

func TestDeleteUser_ServerError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := clerkadmin.NewRESTClient(srv.URL, "sk_test_secret", nil)
	require.NoError(t, err)

	err = c.DeleteUser(context.Background(), "user_err")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestDeleteUser_NetworkError_ReturnsError(t *testing.T) {
	// Start a server and immediately close it to simulate connection refused.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close()

	c, err := clerkadmin.NewRESTClient(srv.URL, "sk_test_secret", nil)
	require.NoError(t, err)

	err = c.DeleteUser(context.Background(), "user_net")
	require.Error(t, err)
}

func TestDeleteUser_204NoContent_Succeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := clerkadmin.NewRESTClient(srv.URL, "sk_test_secret", nil)
	require.NoError(t, err)

	err = c.DeleteUser(context.Background(), "user_204")
	assert.NoError(t, err)
}
