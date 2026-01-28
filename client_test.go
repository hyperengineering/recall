package recall_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hyperengineering/recall"
)

func TestNew_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("New() returned nil client")
	}
}

func TestNew_MissingLocalPath(t *testing.T) {
	_, err := recall.New(recall.Config{LocalPath: ""})
	if err == nil {
		t.Fatal("New() returned nil error, want ValidationError")
	}

	var ve *recall.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("New() returned %T, want *ValidationError", err)
	}
	if ve.Field != "LocalPath" {
		t.Errorf("ValidationError.Field = %q, want %q", ve.Field, "LocalPath")
	}
}

func TestNew_NoEngramURL_OfflineMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error for offline-only config: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("New() returned nil client for offline-only config")
	}
}

func TestNew_EngramURLWithoutAPIKey(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	_, err := recall.New(recall.Config{
		LocalPath: dbPath,
		EngramURL: "http://engram:8080",
	})
	if err == nil {
		t.Fatal("New() returned nil error, want ValidationError for missing APIKey")
	}

	var ve *recall.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("New() returned %T, want *ValidationError", err)
	}
	if ve.Field != "APIKey" {
		t.Errorf("ValidationError.Field = %q, want %q", ve.Field, "APIKey")
	}
}

func TestNew_StoreInitError_WrapsWithClientPrefix(t *testing.T) {
	// Use a path that will cause store initialization to fail
	// (directory that doesn't exist and can't be created)
	invalidPath := "/nonexistent/deeply/nested/path/that/cannot/exist/test.db"

	_, err := recall.New(recall.Config{LocalPath: invalidPath})
	if err == nil {
		t.Fatal("New() returned nil error for invalid path")
	}

	// Verify error is wrapped with "client:" prefix
	errStr := err.Error()
	if len(errStr) < 7 || errStr[:7] != "client:" {
		t.Errorf("error should have 'client:' prefix, got: %q", errStr)
	}
}

func TestClient_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "concurrent.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	const numGoroutines = 10
	var wg sync.WaitGroup

	// Launch goroutines performing Record concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := client.Record(ctx, recall.RecordParams{
				Content:  "Concurrent lore content",
				Category: recall.CategoryArchitecturalDecision,
			})
			if err != nil {
				t.Errorf("goroutine %d: Record() error: %v", id, err)
			}
		}(i)
	}

	// Launch goroutines performing Query concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := client.Query(ctx, recall.QueryParams{
				Query: "concurrent",
				K:     5,
			})
			if err != nil {
				t.Errorf("goroutine %d: Query() error: %v", id, err)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
}

