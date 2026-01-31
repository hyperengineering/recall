package store

import (
	"fmt"
	"os"
)

// ResolveStore determines the store ID to use based on priority chain.
// Priority: explicit > ENGRAM_STORE env > "default"
// Returns the resolved store ID and any validation error.
func ResolveStore(explicit string) (string, error) {
	// 1. Explicit parameter takes precedence
	if explicit != "" {
		if err := ValidateStoreID(explicit); err != nil {
			return "", fmt.Errorf("invalid store ID %q: %w", explicit, err)
		}
		return explicit, nil
	}

	// 2. Environment variable
	if envStore := os.Getenv("ENGRAM_STORE"); envStore != "" {
		if err := ValidateStoreID(envStore); err != nil {
			return "", fmt.Errorf("invalid ENGRAM_STORE %q: %w", envStore, err)
		}
		return envStore, nil
	}

	// 3. Default fallback
	return "default", nil
}
