package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hyperengineering/recall"
)

// EngramClient abstracts HTTP communication with the Engram central service.
// Implementations must be safe for concurrent use.
type EngramClient interface {
	// HealthCheck validates connectivity and returns Engram metadata.
	// Returns embedding model name for compatibility validation.
	HealthCheck(ctx context.Context) (*HealthResponse, error)

	// DownloadSnapshot streams the full lore database snapshot.
	// Caller must close the returned ReadCloser.
	DownloadSnapshot(ctx context.Context) (io.ReadCloser, error)

	// PushLore sends a batch of lore entries to Engram.
	PushLore(ctx context.Context, req *PushLoreRequest) (*PushLoreResponse, error)

	// PushFeedback sends a batch of feedback updates to Engram.
	PushFeedback(ctx context.Context, req *PushFeedbackRequest) (*PushFeedbackResponse, error)

	// GetDelta retrieves lore changes since the given timestamp.
	// Used for incremental sync after initial bootstrap.
	GetDelta(ctx context.Context, since time.Time) (*DeltaResult, error)
}

// HTTPClient implements EngramClient using net/http.
type HTTPClient struct {
	baseURL    string
	apiKey     string
	sourceID   string
	httpClient *http.Client
}

// NewHTTPClient creates a new Engram HTTP client.
// sourceID is optional; if non-empty, it's sent as X-Recall-Source-ID header for observability.
func NewHTTPClient(engramURL, apiKey, sourceID string) *HTTPClient {
	return &HTTPClient{
		baseURL:  strings.TrimSuffix(engramURL, "/"),
		apiKey:   apiKey,
		sourceID: sourceID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WithHTTPClient sets a custom http.Client (for testing or custom timeouts).
func (c *HTTPClient) WithHTTPClient(client *http.Client) *HTTPClient {
	c.httpClient = client
	return c
}

func (c *HTTPClient) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", "recall-client/1.0")
	if strings.TrimSpace(c.sourceID) != "" {
		req.Header.Set("X-Recall-Source-ID", c.sourceID)
	}
}

func newSyncError(op string, statusCode int, body []byte) *recall.SyncError {
	msg := ""
	if len(body) > 0 && statusCode >= 400 {
		if len(body) > 200 {
			msg = string(body[:200]) + "..."
		} else {
			msg = string(body)
		}
	}
	return &recall.SyncError{
		Operation:  op,
		StatusCode: statusCode,
		Err:        fmt.Errorf("HTTP %d: %s", statusCode, msg),
	}
}

func (c *HTTPClient) HealthCheck(ctx context.Context) (*HealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/health", nil)
	if err != nil {
		return nil, &recall.SyncError{Operation: "health_check", Err: err}
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &recall.SyncError{Operation: "health_check", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, newSyncError("health_check", resp.StatusCode, body)
	}

	var health HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, &recall.SyncError{Operation: "health_check", Err: err}
	}

	return &health, nil
}

func (c *HTTPClient) DownloadSnapshot(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/lore/snapshot", nil)
	if err != nil {
		return nil, &recall.SyncError{Operation: "download_snapshot", Err: err}
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &recall.SyncError{Operation: "download_snapshot", Err: err}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, newSyncError("download_snapshot", resp.StatusCode, body)
	}

	return resp.Body, nil
}

func (c *HTTPClient) PushLore(ctx context.Context, req *PushLoreRequest) (*PushLoreResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, &recall.SyncError{Operation: "push_lore", Err: err}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/lore", bytes.NewReader(body))
	if err != nil {
		return nil, &recall.SyncError{Operation: "push_lore", Err: err}
	}
	c.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &recall.SyncError{Operation: "push_lore", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, newSyncError("push_lore", resp.StatusCode, respBody)
	}

	var result PushLoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, &recall.SyncError{Operation: "push_lore", Err: err}
	}

	return &result, nil
}

func (c *HTTPClient) PushFeedback(ctx context.Context, req *PushFeedbackRequest) (*PushFeedbackResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, &recall.SyncError{Operation: "push_feedback", Err: err}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/lore/feedback", bytes.NewReader(body))
	if err != nil {
		return nil, &recall.SyncError{Operation: "push_feedback", Err: err}
	}
	c.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &recall.SyncError{Operation: "push_feedback", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, newSyncError("push_feedback", resp.StatusCode, respBody)
	}

	var result PushFeedbackResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, &recall.SyncError{Operation: "push_feedback", Err: err}
	}

	return &result, nil
}

func (c *HTTPClient) GetDelta(ctx context.Context, since time.Time) (*DeltaResult, error) {
	url := fmt.Sprintf("%s/api/v1/lore/delta?since=%s", c.baseURL, since.Format(time.RFC3339))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &recall.SyncError{Operation: "get_delta", Err: err}
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &recall.SyncError{Operation: "get_delta", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, newSyncError("get_delta", resp.StatusCode, respBody)
	}

	var result DeltaResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, &recall.SyncError{Operation: "get_delta", Err: err}
	}

	return &result, nil
}
