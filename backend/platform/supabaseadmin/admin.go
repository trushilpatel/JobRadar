// Package supabaseadmin wraps the Supabase Auth Admin REST API. v1 ships
// only DeleteUser since that is the single capability the user-deletion
// flow needs; further endpoints can be added without breaking the
// Client interface.
//
// All callers depend on Client (the interface), never on the concrete
// REST implementation, so tests can substitute a fake without spinning
// up an HTTP server.
package supabaseadmin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrUserNotFound is returned when the Supabase admin API responds with
// 404 to a delete request — the user is already gone, which the caller
// usually wants to treat as success.
var ErrUserNotFound = errors.New("supabaseadmin: user not found")

// Client is the contract every implementation must honour. Methods are
// safe for concurrent use.
type Client interface {
	// DeleteUser removes the Supabase auth user with the given ID.
	// Wraps ErrUserNotFound on 404 so callers can use errors.Is.
	DeleteUser(ctx context.Context, userID string) error
}

// RESTClient is the production implementation: a REST call to
// {baseURL}/auth/v1/admin/users/{id} with the service-role key.
type RESTClient struct {
	baseURL    string
	serviceKey string
	httpClient *http.Client
}

// NewRESTClient constructs a Client speaking to the Supabase admin REST
// API. baseURL is SUPABASE_URL (without trailing slash); serviceKey is
// SUPABASE_SERVICE_ROLE_KEY. httpClient is required — callers retain
// ownership and should size timeouts to their own SLAs.
//
// Returns an error when baseURL or serviceKey is empty so misconfiguration
// fails at startup, not on the first user-delete request.
func NewRESTClient(baseURL, serviceKey string, httpClient *http.Client) (*RESTClient, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil, errors.New("supabaseadmin: SUPABASE_URL is required")
	}
	if serviceKey == "" {
		return nil, errors.New("supabaseadmin: SUPABASE_SERVICE_ROLE_KEY is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &RESTClient{
		baseURL:    baseURL,
		serviceKey: serviceKey,
		httpClient: httpClient,
	}, nil
}

// DeleteUser issues DELETE /auth/v1/admin/users/{userID}.
//
// Maps:
//   - 2xx       → nil
//   - 404       → ErrUserNotFound (wrapped)
//   - everything else → fmt.Errorf containing status + body excerpt
func (c *RESTClient) DeleteUser(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("supabaseadmin: userID is required")
	}

	endpoint := fmt.Sprintf("%s/auth/v1/admin/users/%s", c.baseURL, url.PathEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("supabaseadmin: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.serviceKey)
	req.Header.Set("apikey", c.serviceKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("supabaseadmin: delete user %s: %w", userID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Drain body to allow connection reuse; ignore errors.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	excerpt := strings.TrimSpace(string(body))

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("supabaseadmin: user %s: %w (body: %s)", userID, ErrUserNotFound, excerpt)
	}
	return fmt.Errorf("supabaseadmin: delete user %s: status %d (body: %s)", userID, resp.StatusCode, excerpt)
}
