package store_test

import (
	"errors"
	"testing"

	"github.com/hyperengineering/recall/internal/store"
)

func TestValidateStoreID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		// Valid cases
		{"simple", "my-project", false},
		{"with numbers", "project-123", false},
		{"single char", "a", false},
		{"two chars", "ab", false},
		{"multi-segment", "org/team/project", false},
		{"max segments (4)", "a/b/c/d", false},
		{"numeric only", "123", false},
		{"alphanumeric", "abc123def", false},
		{"hyphen middle", "my-project-name", false},
		{"long segment", "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz01", false}, // 64 chars

		// Invalid cases
		{"empty", "", true},
		{"uppercase", "My-Project", true},
		{"leading hyphen", "-project", true},
		{"trailing hyphen", "project-", true},
		{"consecutive hyphens", "my--project", true},
		{"underscore", "my_project", true},
		{"space", "my project", true},
		{"special chars", "my@project", true},
		{"too many segments (5)", "a/b/c/d/e", true},
		{"leading slash", "/project", true},
		{"trailing slash", "project/", true},
		{"empty segment", "org//team", true},
		{"segment leading hyphen", "org/-team", true},
		{"segment trailing hyphen", "org/team-", true},
		{"segment too long (65 chars)", "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz012", true}, // 65 chars exceeds 64 char segment limit
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.ValidateStoreID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateStoreID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
			if tt.wantErr && err != nil {
				if !errors.Is(err, store.ErrInvalidStoreID) {
					t.Errorf("ValidateStoreID(%q) error = %v, want ErrInvalidStoreID", tt.id, err)
				}
			}
		})
	}
}

func TestValidateStoreID_MaxLength(t *testing.T) {
	// Per OpenAPI spec, total max length is 128 characters
	// Use multi-segment ID to test boundary
	// 2 segments of 63 chars each + 1 slash = 127 chars (valid)
	seg63 := "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0" // 63 chars
	validID := seg63 + "/" + seg63                                              // 127 chars

	if err := store.ValidateStoreID(validID); err != nil {
		t.Errorf("ValidateStoreID(127 chars) unexpected error: %v", err)
	}

	// Exactly 128 chars - should be valid
	seg64 := seg63 + "a" // 64 chars
	exact128 := seg64 + "/" + seg64[:63] // 64 + 1 + 63 = 128 chars
	if err := store.ValidateStoreID(exact128); err != nil {
		t.Errorf("ValidateStoreID(128 chars) unexpected error: %v", err)
	}

	// 129 chars - should be invalid (exceeds 128 limit)
	tooLongID := seg64 + "/" + seg64 // 64 + 1 + 64 = 129 chars
	if err := store.ValidateStoreID(tooLongID); err == nil {
		t.Error("ValidateStoreID(129 chars) expected error, got nil")
	}
}

func TestIsReservedStoreID(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		reserved bool
	}{
		{"default is reserved", "default", true},
		{"_system is reserved", "_system", true},
		{"normal ID not reserved", "my-project", false},
		{"empty not reserved", "", false},
		{"DEFAULT not reserved (case sensitive)", "DEFAULT", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := store.IsReservedStoreID(tt.id)
			if got != tt.reserved {
				t.Errorf("IsReservedStoreID(%q) = %v, want %v", tt.id, got, tt.reserved)
			}
		})
	}
}

func TestValidateStoreIDForCreation(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		wantErr   error
		wantNoErr bool
	}{
		{"valid ID", "my-project", nil, true},
		{"default reserved", "default", store.ErrReservedStoreID, false},
		{"_system reserved", "_system", store.ErrReservedStoreID, false},
		{"invalid format", "My-Project", store.ErrInvalidStoreID, false},
		{"empty invalid", "", store.ErrInvalidStoreID, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.ValidateStoreIDForCreation(tt.id)
			if tt.wantNoErr {
				if err != nil {
					t.Errorf("ValidateStoreIDForCreation(%q) unexpected error: %v", tt.id, err)
				}
				return
			}
			if err == nil {
				t.Errorf("ValidateStoreIDForCreation(%q) expected error, got nil", tt.id)
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateStoreIDForCreation(%q) error = %v, want %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestErrInvalidStoreID_Message(t *testing.T) {
	err := store.ErrInvalidStoreID
	msg := err.Error()
	if msg == "" {
		t.Error("ErrInvalidStoreID should have a descriptive message")
	}
}

func TestErrReservedStoreID_Message(t *testing.T) {
	err := store.ErrReservedStoreID
	msg := err.Error()
	if msg == "" {
		t.Error("ErrReservedStoreID should have a descriptive message")
	}
}
