package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hyperengineering/recall"
)

// EngramClient abstracts HTTP communication with the Engram central service.
// Implementations must be safe for concurrent use.
//
// Note: Legacy lore/feedback methods (PushLore, PushFeedback, GetDelta) were
// removed in Story 10.1. The new sync protocol uses change_log-based push/delta
// via /sync/* endpoints, defined in the recall package (SyncPushRequest, etc.).
type EngramClient interface {
	// HealthCheck validates connectivity and returns Engram metadata.
	// Returns embedding model name for compatibility validation.
	HealthCheck(ctx context.Context) (*HealthResponse, error)

	// DownloadSnapshot streams the full lore database snapshot.
	// Caller must close the returned ReadCloser.
	DownloadSnapshot(ctx context.Context) (io.ReadCloser, error)

	// ListStores returns all available stores.
	// If prefix is non-empty, filters stores by ID prefix.
	ListStores(ctx context.Context, prefix string) (*ListStoresResponse, error)

	// GetStoreInfo returns detailed information about a specific store.
	GetStoreInfo(ctx context.Context, storeID string) (*StoreInfoResponse, error)

	// CreateStore creates a new store on Engram.
	CreateStore(ctx context.Context, req *CreateStoreRequest) (*CreateStoreResponse, error)

	// DeleteStore deletes a store on Engram. Requires confirm=true.
	DeleteStore(ctx context.Context, storeID string) error

	// Store-prefixed Operations

	// DownloadSnapshotFromStore streams the lore snapshot for a specific store.
	DownloadSnapshotFromStore(ctx context.Context, storeID string) (io.ReadCloser, error)

	// DeleteLoreFromStore deletes a specific lore entry from a store.
	DeleteLoreFromStore(ctx context.Context, storeID, loreID string) error
}

// encodeStoreID URL-encodes a store ID for use in path parameters.
// Example: "neuralmux/engram" -> "neuralmux%2Fengram"
//
// Note: This function is duplicated in sync.go due to import cycle constraints.
// Both implementations must remain identical.
func encodeStoreID(storeID string) string {
	return url.PathEscape(storeID)
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

func (c *HTTPClient) ListStores(ctx context.Context, prefix string) (*ListStoresResponse, error) {
	url := c.baseURL + "/api/v1/stores"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &recall.SyncError{Operation: "list_stores", Err: err}
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &recall.SyncError{Operation: "list_stores", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, newSyncError("list_stores", resp.StatusCode, respBody)
	}

	var result ListStoresResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, &recall.SyncError{Operation: "list_stores", Err: err}
	}

	// Apply prefix filter client-side if specified
	if prefix != "" {
		filtered := make([]StoreListItem, 0)
		for _, s := range result.Stores {
			if strings.HasPrefix(s.ID, prefix) {
				filtered = append(filtered, s)
			}
		}
		result.Stores = filtered
		result.Total = len(filtered)
	}

	return &result, nil
}

func (c *HTTPClient) GetStoreInfo(ctx context.Context, storeID string) (*StoreInfoResponse, error) {
	// URL-encode the store ID (handles "/" in path-style IDs)
	encodedID := encodeStoreID(storeID)
	reqURL := fmt.Sprintf("%s/api/v1/stores/%s", c.baseURL, encodedID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, &recall.SyncError{Operation: "get_store_info", Err: err}
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &recall.SyncError{Operation: "get_store_info", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, newSyncError("get_store_info", resp.StatusCode, respBody)
	}

	var result StoreInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, &recall.SyncError{Operation: "get_store_info", Err: err}
	}

	return &result, nil
}

func (c *HTTPClient) CreateStore(ctx context.Context, req *CreateStoreRequest) (*CreateStoreResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, &recall.SyncError{Operation: "create_store", Err: err}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/stores", bytes.NewReader(body))
	if err != nil {
		return nil, &recall.SyncError{Operation: "create_store", Err: err}
	}
	c.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &recall.SyncError{Operation: "create_store", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, newSyncError("create_store", resp.StatusCode, respBody)
	}

	var result CreateStoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, &recall.SyncError{Operation: "create_store", Err: err}
	}

	return &result, nil
}

func (c *HTTPClient) DeleteStore(ctx context.Context, storeID string) error {
	encodedID := encodeStoreID(storeID)
	reqURL := fmt.Sprintf("%s/api/v1/stores/%s?confirm=true", c.baseURL, encodedID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return &recall.SyncError{Operation: "delete_store", Err: err}
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &recall.SyncError{Operation: "delete_store", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return newSyncError("delete_store", resp.StatusCode, respBody)
	}

	return nil
}

func (c *HTTPClient) DownloadSnapshotFromStore(ctx context.Context, storeID string) (io.ReadCloser, error) {
	encodedID := encodeStoreID(storeID)
	reqURL := fmt.Sprintf("%s/api/v1/stores/%s/lore/snapshot", c.baseURL, encodedID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, &recall.SyncError{Operation: "download_snapshot_from_store", Err: err}
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &recall.SyncError{Operation: "download_snapshot_from_store", Err: err}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, newSyncError("download_snapshot_from_store", resp.StatusCode, body)
	}

	return resp.Body, nil
}

func (c *HTTPClient) DeleteLoreFromStore(ctx context.Context, storeID, loreID string) error {
	encodedStoreID := encodeStoreID(storeID)
	reqURL := fmt.Sprintf("%s/api/v1/stores/%s/lore/%s", c.baseURL, encodedStoreID, loreID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return &recall.SyncError{Operation: "delete_lore_from_store", Err: err}
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &recall.SyncError{Operation: "delete_lore_from_store", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return newSyncError("delete_lore_from_store", resp.StatusCode, respBody)
	}

	return nil
}
