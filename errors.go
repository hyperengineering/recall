package recall

import (
	"errors"
	"fmt"
)

// Common errors returned by the Recall client.
var (
	// ErrNotFound is returned when a lore entry is not found.
	ErrNotFound = errors.New("lore not found")

	// ErrInvalidCategory is returned when an invalid category is provided.
	ErrInvalidCategory = errors.New("invalid lore category")

	// ErrContentTooLong is returned when content exceeds MaxContentLength.
	ErrContentTooLong = errors.New("content exceeds maximum length")

	// ErrContextTooLong is returned when context exceeds MaxContextLength.
	ErrContextTooLong = errors.New("context exceeds maximum length")

	// ErrInvalidConfidence is returned when confidence is out of range [0, 1].
	ErrInvalidConfidence = errors.New("confidence must be between 0 and 1")

	// ErrEmptyContent is returned when content is empty.
	ErrEmptyContent = errors.New("content cannot be empty")

	// ErrStoreClosed is returned when operating on a closed store.
	ErrStoreClosed = errors.New("store is closed")

	// ErrSyncFailed is returned when a sync operation fails.
	ErrSyncFailed = errors.New("sync operation failed")

	// ErrOffline is returned when network operation is attempted in offline mode.
	ErrOffline = errors.New("operation unavailable in offline mode")

	// ErrModelMismatch is returned when embedding model versions don't match.
	ErrModelMismatch = errors.New("embedding model mismatch")

	// ErrSessionRefNotFound is returned when a session reference cannot be resolved.
	ErrSessionRefNotFound = errors.New("session reference not found")
)

// ValidationError is returned when configuration validation fails.
// Extractable via errors.As().
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("config: %s: %s", e.Field, e.Message)
}

// SyncError is returned when a sync operation fails with details.
// Extractable via errors.As(). Supports Unwrap().
type SyncError struct {
	Operation  string
	StatusCode int
	Err        error
}

func (e *SyncError) Error() string {
	return fmt.Sprintf("sync: %s failed (status %d): %v", e.Operation, e.StatusCode, e.Err)
}

func (e *SyncError) Unwrap() error { return e.Err }
