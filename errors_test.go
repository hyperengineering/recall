package recall_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hyperengineering/recall"
)

func TestSentinelErrors_ErrorsIs(t *testing.T) {
	tests := []struct {
		name     string
		sentinel error
	}{
		{"ErrNotFound", recall.ErrNotFound},
		{"ErrOffline", recall.ErrOffline},
		{"ErrModelMismatch", recall.ErrModelMismatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := fmt.Errorf("operation failed: %w", tt.sentinel)
			if !errors.Is(wrapped, tt.sentinel) {
				t.Errorf("errors.Is(wrapped, %v) = false, want true", tt.sentinel)
			}
		})
	}
}

func TestSentinelErrors_WrappedStillMatches(t *testing.T) {
	wrapped := fmt.Errorf("wrap: %w", recall.ErrNotFound)
	if !errors.Is(wrapped, recall.ErrNotFound) {
		t.Error("errors.Is(wrapped, ErrNotFound) = false, want true")
	}
}

func TestValidationError_ErrorsAs(t *testing.T) {
	err := &recall.ValidationError{Field: "LocalPath", Message: "required: path to SQLite database"}

	var ve *recall.ValidationError
	if !errors.As(err, &ve) {
		t.Fatal("errors.As failed to extract ValidationError")
	}
	if ve.Field != "LocalPath" {
		t.Errorf("Field = %q, want %q", ve.Field, "LocalPath")
	}
}

func TestValidationError_ErrorFormat(t *testing.T) {
	err := &recall.ValidationError{Field: "LocalPath", Message: "required: path to SQLite database"}
	want := "config: LocalPath: required: path to SQLite database"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestSyncError_ErrorsAs(t *testing.T) {
	inner := errors.New("connection refused")
	err := &recall.SyncError{Operation: "push", StatusCode: 503, Err: inner}

	var se *recall.SyncError
	if !errors.As(err, &se) {
		t.Fatal("errors.As failed to extract SyncError")
	}
	if se.Operation != "push" {
		t.Errorf("Operation = %q, want %q", se.Operation, "push")
	}
	if se.StatusCode != 503 {
		t.Errorf("StatusCode = %d, want %d", se.StatusCode, 503)
	}
}

func TestSyncError_ErrorFormat(t *testing.T) {
	inner := errors.New("connection refused")
	err := &recall.SyncError{Operation: "push", StatusCode: 503, Err: inner}
	want := "sync: push failed (status 503): connection refused"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestSyncError_Unwrap(t *testing.T) {
	inner := errors.New("connection refused")
	err := &recall.SyncError{Operation: "push", StatusCode: 503, Err: inner}

	if !errors.Is(err, inner) {
		t.Error("errors.Is(syncErr, inner) = false, want true (Unwrap should expose inner)")
	}
}
