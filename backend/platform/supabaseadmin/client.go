// Package supabaseadmin provides a tiny client for the Supabase Auth admin
// API. The only operation the application currently needs is DeleteUser,
// invoked when a caller deletes their own account so the Supabase auth row
// disappears in lock-step with the application's users row.
//
// The client is intentionally minimal: a single HTTP call with the service
// role key. Callers must guard the service role key like a database password
// — it is read once at construction from env vars and never logged.
package supabaseadmin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is the contract every implementation honours. Splitting out the
// interface keeps the user service testable without spinning up a fake HTTP
// server in every unit test.
type Client interface {
	DeleteUser(ctx context.Context, userID string) error
}

// httpClient is the production implementation. It targets
// {SupabaseURL}/auth/v1/admin/users/{userID} with the service role key as
// both the apikey and bearer token, which is what Supabase requires.
type httpClient struct {
	baseURL    string
	serviceKey string
	http       *http.Client
}

// NewClient constructs an httpClient. Returns an error when either input is
// empty so a misconfigured deployment fails at boot rather than the first
// delete request.
func NewClient(supabaseURL, serviceRoleKey string) (Client, error) {
	if strings.TrimSpace(supabaseURL) == "" {
		return nil, errors.New("supabaseadmin: SUPABASE_URL must not be empty")
	}
	if strings.TrimSpace(serviceRoleKey) == "" {
		return nil, errors.New("supabaseadmin: service role key must not be empty")
	}
	return &httpClient{
		baseURL:    strings.TrimRight(supabaseURL, "/"),
		serviceKey: serviceRoleKey,
		http:       &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// DeleteUser removes the Supabase auth.users row for userID. Returns nil on
// 2xx (including 404 — already deleted is success for our caller). Any other
// status code surfaces as an error so the user service can roll back the
// delete chain.
func (c *httpClient) DeleteUser(ctx context.Context, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("supabaseadmin: userID must not be empty")
	}
	url := fmt.Sprintf("%s/auth/v1/admin/users/%s", c.baseURL, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("supabaseadmin: build request: %w", err)
	}
	req.Header.Set("apikey", c.serviceKey)
	req.Header.Set("Authorization", "Bearer "+c.serviceKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("supabaseadmin: delete user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusNotFound {
		// Already gone — treat as idempotent success.
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("supabaseadmin: delete user: status=%d body=%s", resp.StatusCode, string(body))
}
