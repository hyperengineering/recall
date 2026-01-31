package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hyperengineering/recall"
	"github.com/hyperengineering/recall/internal/store"
)

// handleStoreList handles the recall_store_list tool call.
func (s *Server) handleStoreList(ctx context.Context, args map[string]any) (*ToolResult, error) {
	// Extract optional prefix parameter
	prefix := ""
	if p, ok := args["prefix"].(string); ok {
		prefix = p
	}

	result, err := s.client.ListStores(ctx, prefix)
	if err != nil {
		// Check for offline mode
		if err == recall.ErrOffline {
			return &ToolResult{
				Content: "Store list unavailable: Engram not configured (offline mode)",
				IsError: true,
			}, nil
		}
		return &ToolResult{
			Content: fmt.Sprintf("list stores failed: %v", err),
			IsError: true,
		}, nil
	}

	output := formatStoreList(result, prefix)
	return &ToolResult{Content: output}, nil
}

// handleStoreInfo handles the recall_store_info tool call.
func (s *Server) handleStoreInfo(ctx context.Context, args map[string]any) (*ToolResult, error) {
	// Extract store parameter, resolve if not provided
	storeID, err := s.resolveStore(args)
	if err != nil {
		return &ToolResult{
			Content: fmt.Sprintf("invalid store ID: %v", err),
			IsError: true,
		}, nil
	}

	info, err := s.client.GetStoreInfo(ctx, storeID)
	if err != nil {
		// Check for offline mode
		if err == recall.ErrOffline {
			return &ToolResult{
				Content: "Store info unavailable: Engram not configured (offline mode)",
				IsError: true,
			}, nil
		}
		// Check for specific error messages
		if strings.Contains(err.Error(), "store not found") {
			return &ToolResult{
				Content: fmt.Sprintf("Store not found: %q\nUse recall_store_list to see available stores.", storeID),
				IsError: true,
			}, nil
		}
		if strings.Contains(err.Error(), "invalid store ID") {
			return &ToolResult{
				Content: fmt.Sprintf("Invalid store ID: %q\nStore IDs must be lowercase alphanumeric with hyphens, 1-4 path segments separated by '/'.", storeID),
				IsError: true,
			}, nil
		}
		return &ToolResult{
			Content: fmt.Sprintf("get store info failed: %v", err),
			IsError: true,
		}, nil
	}

	output := formatStoreInfo(info)
	return &ToolResult{Content: output}, nil
}

// resolveStore extracts and validates the store parameter from args.
// If not provided, uses the store resolution priority chain.
func (s *Server) resolveStore(args map[string]any) (string, error) {
	// Extract explicit store parameter
	explicit := ""
	if st, ok := args["store"].(string); ok && st != "" {
		explicit = st
	}

	// Use store resolution: explicit > env > default
	storeID, err := store.ResolveStore(explicit)
	if err != nil {
		return "", err
	}

	return storeID, nil
}

// formatStoreList formats the store list response for display.
func formatStoreList(result *recall.StoreListResult, prefix string) string {
	if len(result.Stores) == 0 {
		if prefix != "" {
			return fmt.Sprintf("No stores found with prefix %q.", prefix)
		}
		return "No stores found."
	}

	var sb strings.Builder
	if prefix != "" {
		sb.WriteString(fmt.Sprintf("Available stores with prefix %q (%d):\n\n", prefix, result.Total))
	} else {
		sb.WriteString(fmt.Sprintf("Available stores (%d):\n\n", result.Total))
	}

	for _, st := range result.Stores {
		sb.WriteString(fmt.Sprintf("  %s\n", st.ID))
		if st.Description != "" {
			sb.WriteString(fmt.Sprintf("    Description: %s\n", st.Description))
		}
		sb.WriteString(fmt.Sprintf("    Lore: %d entries | Updated: %s\n\n",
			st.RecordCount, formatRelativeTime(st.LastAccessed)))
	}

	return sb.String()
}

// formatStoreInfo formats the store info response for display.
func formatStoreInfo(info *recall.StoreInfo) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Store: %s\n", info.ID))
	if info.Description != "" {
		sb.WriteString(fmt.Sprintf("Description: %s\n", info.Description))
	}
	sb.WriteString(fmt.Sprintf("Created: %s\n", formatTimestamp(info.Created)))
	sb.WriteString(fmt.Sprintf("Updated: %s\n", formatTimestamp(info.LastAccessed)))
	sb.WriteString("\n")

	// Statistics
	sb.WriteString("Statistics:\n")
	sb.WriteString(fmt.Sprintf("  Lore Count: %d\n", info.Stats.ActiveLore))
	sb.WriteString(fmt.Sprintf("  Average Confidence: %.2f\n", info.Stats.QualityStats.AverageConfidence))

	validatedPct := float64(0)
	if info.Stats.ActiveLore > 0 {
		validatedPct = float64(info.Stats.QualityStats.ValidatedCount) / float64(info.Stats.ActiveLore) * 100
	}
	sb.WriteString(fmt.Sprintf("  Validated Entries: %d (%.1f%%)\n",
		info.Stats.QualityStats.ValidatedCount, validatedPct))
	sb.WriteString("\n")

	// Category distribution
	if len(info.Stats.CategoryStats) > 0 {
		sb.WriteString("Category Distribution:\n")
		for cat, count := range info.Stats.CategoryStats {
			pct := float64(0)
			if info.Stats.ActiveLore > 0 {
				pct = float64(count) / float64(info.Stats.ActiveLore) * 100
			}
			sb.WriteString(fmt.Sprintf("  %-25s %d (%.1f%%)\n", cat, count, pct))
		}
	}

	return sb.String()
}

// formatRelativeTime formats a timestamp as relative time (e.g., "2h ago").
func formatRelativeTime(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return timestamp
	}

	duration := time.Since(t)
	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		mins := int(duration.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", mins)
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	default:
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

// formatTimestamp formats a timestamp for display.
func formatTimestamp(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return timestamp
	}
	return t.Format("2006-01-02 15:04:05 UTC")
}
