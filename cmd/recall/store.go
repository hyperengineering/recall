package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hyperengineering/recall"
	"github.com/hyperengineering/recall/internal/store"
	"github.com/spf13/cobra"
)

var storeCmd = &cobra.Command{
	Use:   "store",
	Short: "Manage local lore stores",
	Long: `Manage local lore stores for project isolation.

Subcommands:
  list    List all local stores
  create  Create a new store
  delete  Delete an existing store
  info    Show store details and statistics

Example:
  recall store list
  recall store create my-project --description "My project lore"
  recall store info my-project`,
}

var storeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List local stores",
	Long: `List all local stores with statistics.

Example:
  recall store list
  recall store list --json`,
	RunE: runStoreList,
}

var storeCreateCmd = &cobra.Command{
	Use:   "create <store-id>",
	Short: "Create a new store",
	Long: `Create a new local store for project isolation.

Store ID format:
  - Lowercase alphanumeric characters and hyphens
  - 1 to 4 path segments separated by '/'
  - Each segment 1-64 characters
  - No leading/trailing hyphens, no consecutive hyphens

Example:
  recall store create my-project
  recall store create neuralmux/engram --description "Engram project"`,
	Args: cobra.ExactArgs(1),
	RunE: runStoreCreate,
}

var storeDeleteCmd = &cobra.Command{
	Use:   "delete <store-id>",
	Short: "Delete a store",
	Long: `Delete a local store and all its lore.

Requires --confirm flag for safety. Use --force to skip interactive prompt.
Cannot delete the 'default' store.

Example:
  recall store delete my-project --confirm
  recall store delete my-project --confirm --force`,
	Args: cobra.ExactArgs(1),
	RunE: runStoreDelete,
}

var storeInfoCmd = &cobra.Command{
	Use:   "info [store-id]",
	Short: "Show store details",
	Long: `Display detailed information and statistics for a store.

If store-id is not provided, uses the resolved store from environment/config.

Example:
  recall store info my-project
  recall store info --json
  recall store info`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStoreInfo,
}

var (
	storeDescription   string
	storeDeleteConfirm bool
	storeDeleteForce   bool
)

func init() {
	storeCreateCmd.Flags().StringVar(&storeDescription, "description", "", "Store description")
	storeDeleteCmd.Flags().BoolVar(&storeDeleteConfirm, "confirm", false, "Confirm deletion (required)")
	storeDeleteCmd.Flags().BoolVar(&storeDeleteForce, "force", false, "Skip interactive prompt")

	storeCmd.AddCommand(storeListCmd)
	storeCmd.AddCommand(storeCreateCmd)
	storeCmd.AddCommand(storeDeleteCmd)
	storeCmd.AddCommand(storeInfoCmd)
}

// StoreListEntry represents a store in list output.
type StoreListEntry struct {
	ID          string    `json:"id"`
	Description string    `json:"description,omitempty"`
	LoreCount   int       `json:"lore_count"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

// StoreListResult for JSON output.
type StoreListResult struct {
	Stores []StoreListEntry `json:"stores"`
	Total  int              `json:"total"`
}

func runStoreList(cmd *cobra.Command, args []string) error {
	storeRoot := store.DefaultStoreRoot()

	// Check if stores directory exists
	if _, err := os.Stat(storeRoot); os.IsNotExist(err) {
		if outputJSON {
			return outputAsJSON(cmd, StoreListResult{Stores: []StoreListEntry{}, Total: 0})
		}
		out := cmd.OutOrStdout()
		printWarning(out, "No stores found.")
		printMuted(out, "Create one with: recall store create <store-id>")
		return nil
	}

	// Scan stores directory
	entries, err := os.ReadDir(storeRoot)
	if err != nil {
		return fmt.Errorf("read stores directory: %w", err)
	}

	var stores []StoreListEntry
	for _, dirEntry := range entries {
		if !dirEntry.IsDir() {
			continue
		}

		storeID := store.DecodeStorePath(dirEntry.Name())
		dbPath := filepath.Join(storeRoot, dirEntry.Name(), "lore.db")

		// Skip if no database file
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			continue
		}

		// Open store to get stats
		s, err := recall.NewStore(dbPath)
		if err != nil {
			// Skip stores that can't be opened
			continue
		}

		desc, _ := s.GetStoreDescription()
		stats, _ := s.GetDetailedStats()
		_ = s.Close() // Best-effort close; store is read-only here

		listEntry := StoreListEntry{
			ID:          storeID,
			Description: desc,
		}
		if stats != nil {
			listEntry.LoreCount = stats.LoreCount
			listEntry.UpdatedAt = stats.LastUpdated
		}

		stores = append(stores, listEntry)
	}

	// Sort by ID
	sort.Slice(stores, func(i, j int) bool {
		return stores[i].ID < stores[j].ID
	})

	if outputJSON {
		return outputAsJSON(cmd, StoreListResult{Stores: stores, Total: len(stores)})
	}

	out := cmd.OutOrStdout()

	if len(stores) == 0 {
		printWarning(out, "No stores found.")
		printMuted(out, "Create one with: recall store create <store-id>")
		return nil
	}

	// Table header
	printInfo(out, "Local Stores (%d):", len(stores))
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%-30s %-35s %10s %15s\n", "STORE ID", "DESCRIPTION", "LORE COUNT", "UPDATED")
	fmt.Fprintf(out, "%-30s %-35s %10s %15s\n", strings.Repeat("-", 30), strings.Repeat("-", 35), strings.Repeat("-", 10), strings.Repeat("-", 15))

	for _, s := range stores {
		desc := s.Description
		if len(desc) > 35 {
			desc = desc[:32] + "..."
		}

		updated := "-"
		if !s.UpdatedAt.IsZero() {
			updated = formatRelativeTime(s.UpdatedAt)
		}

		fmt.Fprintf(out, "%-30s %-35s %10d %15s\n", s.ID, desc, s.LoreCount, updated)
	}

	return nil
}

// StoreCreateResult for JSON output.
type StoreCreateResult struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location"`
}

func runStoreCreate(cmd *cobra.Command, args []string) error {
	storeID := args[0]

	// Validate store ID for creation (rejects reserved IDs)
	if err := store.ValidateStoreIDForCreation(storeID); err != nil {
		return fmt.Errorf("invalid store ID %q: %w\n\nStore IDs must be lowercase alphanumeric with hyphens, 1-4 path segments separated by '/'.\nValid examples: my-project, team/project, org/team/service", storeID, err)
	}

	// Check if store already exists
	dbPath := store.StoreDBPath(storeID)
	storeDir := filepath.Dir(dbPath)

	if _, err := os.Stat(dbPath); err == nil {
		return fmt.Errorf("store %q already exists at %s", storeID, storeDir)
	}

	// Create store directory
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		return fmt.Errorf("create store directory: %w", err)
	}

	// Initialize store database
	s, err := recall.NewStore(dbPath)
	if err != nil {
		// Clean up directory on failure (best-effort)
		_ = os.RemoveAll(storeDir)
		return fmt.Errorf("initialize store: %w", err)
	}

	// Set description if provided
	if storeDescription != "" {
		if err := s.SetStoreDescription(storeDescription); err != nil {
			_ = s.Close()            // Best-effort close
			_ = os.RemoveAll(storeDir) // Best-effort cleanup
			return fmt.Errorf("set description: %w", err)
		}
	}

	if err := s.Close(); err != nil {
		_ = os.RemoveAll(storeDir) // Best-effort cleanup
		return fmt.Errorf("close store: %w", err)
	}

	if outputJSON {
		return outputAsJSON(cmd, StoreCreateResult{
			ID:          storeID,
			Description: storeDescription,
			Location:    storeDir,
		})
	}

	out := cmd.OutOrStdout()
	printSuccess(out, "Store created: %s", storeID)
	if storeDescription != "" {
		fmt.Fprintf(out, "  Description: %s\n", storeDescription)
	}
	fmt.Fprintf(out, "  Location: %s\n", storeDir)

	return nil
}

// StoreDeleteResult for JSON output.
type StoreDeleteResult struct {
	ID        string `json:"id"`
	LoreCount int    `json:"lore_count_deleted"`
}

func runStoreDelete(cmd *cobra.Command, args []string) error {
	storeID := args[0]

	// Validate store ID
	if err := store.ValidateStoreID(storeID); err != nil {
		return fmt.Errorf("invalid store ID %q: %w", storeID, err)
	}

	// Check for --confirm flag
	if !storeDeleteConfirm {
		return fmt.Errorf("--confirm flag is required for delete\n\nUsage: recall store delete <store-id> --confirm [--force]")
	}

	// Cannot delete "default" store
	if storeID == "default" {
		return fmt.Errorf("cannot delete protected store 'default'\n\nUse 'recall sync --reinit' to reinitialize the default store")
	}

	// Check if store exists
	dbPath := store.StoreDBPath(storeID)
	storeDir := filepath.Dir(dbPath)

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("store %q not found", storeID)
	}

	// Get lore count for warning
	var loreCount int
	s, err := recall.NewStore(dbPath)
	if err == nil {
		stats, _ := s.Stats()
		if stats != nil {
			loreCount = stats.LoreCount
		}
		_ = s.Close() // Best-effort close; store is being deleted anyway
	}

	out := cmd.OutOrStdout()

	// Prompt for confirmation unless --force
	if !storeDeleteForce {
		printWarning(out, "This will permanently delete store '%s' and all %d lore entries.", storeID, loreCount)
		fmt.Fprintf(out, "Type '%s' to confirm: ", storeID)

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		response = strings.TrimSpace(response)
		if response != storeID {
			printMuted(out, "Aborted.")
			return nil
		}
	}

	// Delete store directory
	if err := os.RemoveAll(storeDir); err != nil {
		return fmt.Errorf("delete store: %w", err)
	}

	if outputJSON {
		return outputAsJSON(cmd, StoreDeleteResult{
			ID:        storeID,
			LoreCount: loreCount,
		})
	}

	printSuccess(out, "Store deleted: %s", storeID)
	if loreCount > 0 {
		fmt.Fprintf(out, "  Deleted %d lore entries\n", loreCount)
	}

	return nil
}

// StoreInfoResult for JSON output.
type StoreInfoResult struct {
	ID                   string         `json:"id"`
	Description          string         `json:"description,omitempty"`
	Location             string         `json:"location"`
	CreatedAt            time.Time      `json:"created_at,omitempty"`
	UpdatedAt            time.Time      `json:"updated_at,omitempty"`
	LoreCount            int            `json:"lore_count"`
	AverageConfidence    float64        `json:"average_confidence"`
	CategoryDistribution map[string]int `json:"category_distribution"`
	Resolved             bool           `json:"resolved,omitempty"`
}

func runStoreInfo(cmd *cobra.Command, args []string) error {
	var storeID string
	var resolved bool

	if len(args) > 0 {
		storeID = args[0]
	} else {
		// Resolve store from environment
		var err error
		storeID, err = store.ResolveStore("")
		if err != nil {
			return fmt.Errorf("resolve store: %w", err)
		}
		resolved = true
	}

	// Validate store ID
	if err := store.ValidateStoreID(storeID); err != nil {
		return fmt.Errorf("invalid store ID %q: %w", storeID, err)
	}

	// Check if store exists
	dbPath := store.StoreDBPath(storeID)
	storeDir := filepath.Dir(dbPath)

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("store %q not found", storeID)
	}

	// Open store
	s, err := recall.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	// Get metadata
	desc, _ := s.GetStoreDescription()
	createdAt, _ := s.GetStoreCreatedAt()

	// Get detailed stats
	stats, err := s.GetDetailedStats()
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}

	if outputJSON {
		catDist := make(map[string]int)
		for cat, count := range stats.CategoryDistribution {
			catDist[string(cat)] = count
		}
		return outputAsJSON(cmd, StoreInfoResult{
			ID:                   storeID,
			Description:          desc,
			Location:             storeDir,
			CreatedAt:            createdAt,
			UpdatedAt:            stats.LastUpdated,
			LoreCount:            stats.LoreCount,
			AverageConfidence:    stats.AverageConfidence,
			CategoryDistribution: catDist,
			Resolved:             resolved,
		})
	}

	out := cmd.OutOrStdout()

	// Header
	if resolved {
		printInfo(out, "Store: %s (resolved from environment)", storeID)
	} else {
		printInfo(out, "Store: %s", storeID)
	}

	if desc != "" {
		fmt.Fprintf(out, "  Description: %s\n", desc)
	}
	fmt.Fprintf(out, "  Location: %s\n", storeDir)
	if !createdAt.IsZero() {
		fmt.Fprintf(out, "  Created: %s\n", createdAt.Format("2006-01-02 15:04:05 MST"))
	}
	if !stats.LastUpdated.IsZero() {
		fmt.Fprintf(out, "  Updated: %s\n", stats.LastUpdated.Format("2006-01-02 15:04:05 MST"))
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Statistics:")
	fmt.Fprintf(out, "  Lore Count: %d\n", stats.LoreCount)
	fmt.Fprintf(out, "  Average Confidence: %.2f\n", stats.AverageConfidence)

	if len(stats.CategoryDistribution) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Category Distribution:")

		// Sort categories by count (descending)
		type catCount struct {
			cat   recall.Category
			count int
		}
		var cats []catCount
		for cat, count := range stats.CategoryDistribution {
			cats = append(cats, catCount{cat, count})
		}
		sort.Slice(cats, func(i, j int) bool {
			return cats[i].count > cats[j].count
		})

		for _, cc := range cats {
			var pct float64
			if stats.LoreCount > 0 {
				pct = float64(cc.count) / float64(stats.LoreCount) * 100
			}
			fmt.Fprintf(out, "  %-25s %4d (%.1f%%)\n", cc.cat, cc.count, pct)
		}
	}

	return nil
}

// formatRelativeTime formats a time as a relative string (e.g., "2h ago")
func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}

	d := time.Since(t)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	default:
		weeks := int(d.Hours() / 24 / 7)
		if weeks == 1 {
			return "1w ago"
		}
		return fmt.Sprintf("%dw ago", weeks)
	}
}
