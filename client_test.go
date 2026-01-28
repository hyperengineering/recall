package recall_test

import (
	"errors"
	"path/filepath"
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

