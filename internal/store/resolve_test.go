package store_test

import (
	"errors"
	"os"
	"testing"

	"github.com/hyperengineering/recall/internal/store"
)

func TestResolveStore_ExplicitParam(t *testing.T) {
	// Save and clear env
	origEnv := os.Getenv("ENGRAM_STORE")
	os.Unsetenv("ENGRAM_STORE")
	t.Cleanup(func() {
		if origEnv != "" {
			os.Setenv("ENGRAM_STORE", origEnv)
		}
	})

	got, err := store.ResolveStore("my-project")
	if err != nil {
		t.Fatalf("ResolveStore(explicit) unexpected error: %v", err)
	}
	if got != "my-project" {
		t.Errorf("ResolveStore(explicit) = %q, want %q", got, "my-project")
	}
}

func TestResolveStore_EnvVar(t *testing.T) {
	// Set env var
	origEnv := os.Getenv("ENGRAM_STORE")
	os.Setenv("ENGRAM_STORE", "env-project")
	t.Cleanup(func() {
		if origEnv != "" {
			os.Setenv("ENGRAM_STORE", origEnv)
		} else {
			os.Unsetenv("ENGRAM_STORE")
		}
	})

	got, err := store.ResolveStore("")
	if err != nil {
		t.Fatalf("ResolveStore(env) unexpected error: %v", err)
	}
	if got != "env-project" {
		t.Errorf("ResolveStore(env) = %q, want %q", got, "env-project")
	}
}

func TestResolveStore_DefaultFallback(t *testing.T) {
	// Clear env var
	origEnv := os.Getenv("ENGRAM_STORE")
	os.Unsetenv("ENGRAM_STORE")
	t.Cleanup(func() {
		if origEnv != "" {
			os.Setenv("ENGRAM_STORE", origEnv)
		}
	})

	got, err := store.ResolveStore("")
	if err != nil {
		t.Fatalf("ResolveStore(default) unexpected error: %v", err)
	}
	if got != "default" {
		t.Errorf("ResolveStore(default) = %q, want %q", got, "default")
	}
}

func TestResolveStore_ExplicitOverEnv(t *testing.T) {
	// Set env var but provide explicit
	origEnv := os.Getenv("ENGRAM_STORE")
	os.Setenv("ENGRAM_STORE", "env-project")
	t.Cleanup(func() {
		if origEnv != "" {
			os.Setenv("ENGRAM_STORE", origEnv)
		} else {
			os.Unsetenv("ENGRAM_STORE")
		}
	})

	got, err := store.ResolveStore("explicit-project")
	if err != nil {
		t.Fatalf("ResolveStore(explicit over env) unexpected error: %v", err)
	}
	if got != "explicit-project" {
		t.Errorf("ResolveStore(explicit over env) = %q, want %q", got, "explicit-project")
	}
}

func TestResolveStore_InvalidExplicit(t *testing.T) {
	_, err := store.ResolveStore("INVALID-Store")
	if err == nil {
		t.Error("ResolveStore(invalid explicit) expected error, got nil")
	}
	if !errors.Is(err, store.ErrInvalidStoreID) {
		t.Errorf("ResolveStore(invalid explicit) error = %v, want ErrInvalidStoreID", err)
	}
}

func TestResolveStore_InvalidEnv(t *testing.T) {
	origEnv := os.Getenv("ENGRAM_STORE")
	os.Setenv("ENGRAM_STORE", "INVALID-Store")
	t.Cleanup(func() {
		if origEnv != "" {
			os.Setenv("ENGRAM_STORE", origEnv)
		} else {
			os.Unsetenv("ENGRAM_STORE")
		}
	})

	_, err := store.ResolveStore("")
	if err == nil {
		t.Error("ResolveStore(invalid env) expected error, got nil")
	}
	if !errors.Is(err, store.ErrInvalidStoreID) {
		t.Errorf("ResolveStore(invalid env) error = %v, want ErrInvalidStoreID", err)
	}
}

func TestResolveStore_ReservedAllowed(t *testing.T) {
	// Reserved IDs should be allowed (for targeting)
	origEnv := os.Getenv("ENGRAM_STORE")
	os.Unsetenv("ENGRAM_STORE")
	t.Cleanup(func() {
		if origEnv != "" {
			os.Setenv("ENGRAM_STORE", origEnv)
		}
	})

	tests := []string{"default", "_system"}
	for _, id := range tests {
		t.Run(id, func(t *testing.T) {
			got, err := store.ResolveStore(id)
			if err != nil {
				t.Fatalf("ResolveStore(%q) unexpected error: %v", id, err)
			}
			if got != id {
				t.Errorf("ResolveStore(%q) = %q, want %q", id, got, id)
			}
		})
	}
}

func TestResolveStore_PathStyleID(t *testing.T) {
	origEnv := os.Getenv("ENGRAM_STORE")
	os.Unsetenv("ENGRAM_STORE")
	t.Cleanup(func() {
		if origEnv != "" {
			os.Setenv("ENGRAM_STORE", origEnv)
		}
	})

	got, err := store.ResolveStore("org/team/project")
	if err != nil {
		t.Fatalf("ResolveStore(path-style) unexpected error: %v", err)
	}
	if got != "org/team/project" {
		t.Errorf("ResolveStore(path-style) = %q, want %q", got, "org/team/project")
	}
}
