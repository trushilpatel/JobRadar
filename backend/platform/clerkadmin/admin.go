// Package clerkadmin wraps the Clerk Backend REST API. v1 ships only
// DeleteUser since that is the single capability the user-deletion flow needs;
// further endpoints can be added without breaking the Client interface.
//
// All callers depend on Client (the interface), never on the concrete REST
// implementation, so tests can substitute a fake without spinning up an HTTP server.
package clerkadmin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrUserNotFound is returned when the Clerk API responds with 404 to a delete
// request — the user is already gone, which the caller usually treats as success.
var ErrUserNotFound = errors.New("clerkadmin: user not found")

// Client is the contract every implementation must honour. Methods are safe
// for concurrent use.
type Client interface {
	// DeleteUser removes the Clerk user with the given ID.
	// Wraps ErrUserNotFound on 404 so callers can use errors.Is.
	DeleteUser(ctx context.Context, userID string) error
}

// RESTClient is the production implementation: a REST call to
// {baseURL}/v1/users/{id} with the Clerk secret key.
type RESTClient struct {
	baseURL    string
	secretKey  string
	httpClient *http.Client
}

// NewRESTClient constructs a Client speaking to the Clerk Backend API.
// baseURL should be "https://api.clerk.dev"; secretKey is CLERK_SECRET_KEY.
// httpClient may be nil — a default with 10s timeout is used.
//
// Returns an error when baseURL or secretKey is empty so misconfiguration
// fails at startup, not on the first user-delete request.
func NewRESTClient(baseURL, secretKey string, httpClient *http.Client) (*RESTClient, error) {
	if secretKey == "" {
		return nil, errors.New("clerkadmin: secretKey must not be empty")
	}
	if baseURL == "" {
		return nil, errors.New("clerkadmin: baseURL must not be empty")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &RESTClient{baseURL: baseURL, secretKey: secretKey, httpClient: httpClient}, nil
}

// DeleteUser issues DELETE /v1/users/{userID}.
//
// Maps:
//   - 2xx       → nil
//   - 404       → ErrUserNotFound (wrapped)
//   - everything else → fmt.Errorf containing status code
func (c *RESTClient) DeleteUser(ctx context.Context, userID string) error {
	url := fmt.Sprintf("%s/v1/users/%s", c.baseURL, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("clerkadmin: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("clerkadmin: delete user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrUserNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("clerkadmin: delete user %s: unexpected status %d", userID, resp.StatusCode)
	}
	return nil
}

// Compile-time assertion.
var _ Client = (*RESTClient)(nil)
