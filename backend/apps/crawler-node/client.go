package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// httpOrchestratorClient talks to the orchestrator over HTTP with Bearer-token
// auth, a configurable HTTP timeout, and linear retry on transient (5xx)
// errors.
type httpOrchestratorClient struct {
	baseURL    string
	token      string
	maxRetry   int
	httpClient *http.Client
}

// newHTTPClient constructs an httpOrchestratorClient from Config.
func newHTTPClient(cfg *Config) OrchestratorClient {
	return &httpOrchestratorClient{
		baseURL:  cfg.OrchestratorURL,
		token:    cfg.CrawlerToken,
		maxRetry: cfg.RetryMaxAttempts,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.HTTPTimeoutSeconds) * time.Second,
		},
	}
}

// orchestratorWorkEnvelope is the JSON envelope returned by GET /node/work.
type orchestratorWorkEnvelope struct {
	Data orchestratorWorkData `json:"data"`
}

type orchestratorWorkData struct {
	Assignment      *WorkUnit `json:"assignment"`
	HeartbeatAcked  []string  `json:"heartbeat_acked"`
	HeartbeatMissed []string  `json:"heartbeat_missed"`
}

func (c *httpOrchestratorClient) GetWork(ctx context.Context, holding []string) (*GetWorkResult, error) {
	u, err := url.Parse(c.baseURL + "/node/work")
	if err != nil {
		return nil, fmt.Errorf("client: parse URL: %w", err)
	}
	if len(holding) > 0 {
		q := u.Query()
		for _, id := range holding {
			q.Add("holding", id)
		}
		u.RawQuery = q.Encode()
	}

	var env orchestratorWorkEnvelope
	if err := c.do(ctx, http.MethodGet, u.String(), nil, &env); err != nil {
		return nil, err
	}
	return &GetWorkResult{
		Assignment:      env.Data.Assignment,
		HeartbeatMissed: env.Data.HeartbeatMissed,
	}, nil
}

func (c *httpOrchestratorClient) SubmitBatch(ctx context.Context, batch PageBatch) error {
	// PageBatch contains only JSON-serializable primitive types; marshal never fails.
	body, _ := json.Marshal(batch) //nolint:errcheck
	return c.do(ctx, http.MethodPost, c.baseURL+"/node/results", body, nil)
}

func (c *httpOrchestratorClient) ReportError(ctx context.Context, e WorkError) error {
	// WorkError contains only JSON-serializable primitive types; marshal never fails.
	body, _ := json.Marshal(e) //nolint:errcheck
	return c.do(ctx, http.MethodPost, c.baseURL+"/node/error", body, nil)
}

// do executes an HTTP request with linear retry on 5xx responses.
// dest may be nil when the caller does not need the response body.
func (c *httpOrchestratorClient) do(ctx context.Context, method, rawURL string, body []byte, dest any) error {
	attempt := 0
	for {
		attempt++
		err := c.doOnce(ctx, method, rawURL, body, dest)
		if err == nil {
			return nil
		}
		// Retry only retriable errors; context cancellations and non-5xx are final.
		if !isRetriable(err) || attempt >= c.maxRetry {
			return err
		}
	}
}

// doOnce sends a single HTTP request.
func (c *httpOrchestratorClient) doOnce(ctx context.Context, method, rawURL string, body []byte, dest any) error {
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return fmt.Errorf("client: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("client: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode >= http.StatusInternalServerError {
		return &retriableError{status: resp.StatusCode}
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("client: server returned %d", resp.StatusCode)
	}

	if dest != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("client: decode response: %w", err)
		}
	}
	return nil
}

// retriableError marks a 5xx response as eligible for retry.
type retriableError struct{ status int }

func (e *retriableError) Error() string { return fmt.Sprintf("client: server returned %d", e.status) }

func isRetriable(err error) bool {
	_, ok := err.(*retriableError) //nolint:errorlint
	return ok
}
