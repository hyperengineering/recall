package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hyperengineering/recall"
	"github.com/hyperengineering/recall/internal/store"
	"github.com/spf13/cobra"
)

var storeImportCmd = &cobra.Command{
	Use:   "import <store-id>",
	Short: "Import data into a store from a file",
	Long: `Import lore from an export file into a store.

The store must already exist. Use 'recall store create' to create it first.

Merge strategies:
  skip    - Skip entries that already exist (by ID)
  replace - Replace existing entries with imported versions
  merge   - Upsert entries by ID (default)

Format auto-detection:
  .json           -> JSON format
  .db, .sqlite    -> SQLite format

Examples:
  recall store import my-store -i backup.json
  recall store import my-store -i backup.json --merge-strategy replace
  recall store import my-store -i backup.json --dry-run
  recall store import my-store -i backup.db`,
	Args: cobra.ExactArgs(1),
	RunE: runStoreImport,
}

var (
	importInputPath    string
	importMergeStrategy string
	importDryRun       bool
	importFormat       string
)

func init() {
	storeImportCmd.Flags().StringVarP(&importInputPath, "input", "i", "", "Input file path (required)")
	storeImportCmd.Flags().StringVar(&importMergeStrategy, "merge-strategy", "merge", "Merge strategy: skip, replace, merge")
	storeImportCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Preview import without making changes")
	storeImportCmd.Flags().StringVar(&importFormat, "format", "", "Override format detection: json, sqlite")
	_ = storeImportCmd.MarkFlagRequired("input")

	storeCmd.AddCommand(storeImportCmd)
}

// ImportResultOutput for JSON output.
type ImportResultOutput struct {
	StoreID       string   `json:"store_id"`
	InputFile     string   `json:"input_file"`
	Format        string   `json:"format"`
	Strategy      string   `json:"merge_strategy"`
	DryRun        bool     `json:"dry_run"`
	Total         int      `json:"total"`
	Created       int      `json:"created"`
	Merged        int      `json:"merged"`
	Skipped       int      `json:"skipped"`
	ErrorCount    int      `json:"error_count"`
	Errors        []string `json:"errors,omitempty"`
	Duration      string   `json:"duration"`
}

func runStoreImport(cmd *cobra.Command, args []string) error {
	storeID := args[0]
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	// Validate store ID
	if err := store.ValidateStoreID(storeID); err != nil {
		return fmt.Errorf("invalid store ID %q: %w", storeID, err)
	}

	// Validate merge strategy
	strategy := recall.MergeStrategy(strings.ToLower(importMergeStrategy))
	switch strategy {
	case recall.MergeStrategySkip, recall.MergeStrategyReplace, recall.MergeStrategyMerge:
		// valid
	default:
		return fmt.Errorf("invalid merge strategy %q: must be 'skip', 'replace', or 'merge'", importMergeStrategy)
	}

	// Check if input file exists
	if _, err := os.Stat(importInputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", importInputPath)
	}

	// Detect format
	format := detectImportFormat(importInputPath)
	if importFormat != "" {
		format = strings.ToLower(importFormat)
	}
	if format != "json" && format != "sqlite" {
		return fmt.Errorf("cannot detect format for %q, use --format to specify", importInputPath)
	}

	// Check if store exists
	dbPath := store.StoreDBPath(storeID)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("store %q not found\n\nCreate it first with: recall store create %s", storeID, storeID)
	}

	// Open store
	s, err := recall.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	if !outputJSON {
		if importDryRun {
			printInfo(out, "Previewing import into store '%s' from %s...", storeID, importInputPath)
		} else {
			printInfo(out, "Importing into store '%s' from %s...", storeID, importInputPath)
		}
		fmt.Fprintf(out, "  Format: %s\n", strings.ToUpper(format))
		fmt.Fprintf(out, "  Strategy: %s\n", strategy)
	}

	startTime := time.Now()

	// Perform import
	var result *recall.ImportResult
	switch format {
	case "json":
		result, err = importJSON(ctx, s, importInputPath, strategy, importDryRun)
	case "sqlite":
		result, err = importSQLite(ctx, s, importInputPath, strategy, importDryRun)
	}

	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	duration := time.Since(startTime)

	if outputJSON {
		return outputAsJSON(cmd, ImportResultOutput{
			StoreID:       storeID,
			InputFile:     importInputPath,
			Format:        format,
			Strategy:      string(strategy),
			DryRun:        importDryRun,
			Total:         result.Total,
			Created:       result.Created,
			Merged:        result.Merged,
			Skipped:       result.Skipped,
			ErrorCount:    len(result.Errors),
			Errors:        result.Errors,
			Duration:      duration.Round(time.Millisecond).String(),
		})
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "  Total entries: %d\n", result.Total)
	if importDryRun {
		fmt.Fprintf(out, "  Would create: %d\n", result.Created)
		if strategy == recall.MergeStrategySkip {
			fmt.Fprintf(out, "  Would skip: %d\n", result.Skipped)
		} else {
			fmt.Fprintf(out, "  Would merge: %d\n", result.Merged)
		}
	} else {
		fmt.Fprintf(out, "  Created: %d\n", result.Created)
		if strategy == recall.MergeStrategySkip {
			fmt.Fprintf(out, "  Skipped: %d\n", result.Skipped)
		} else {
			fmt.Fprintf(out, "  Merged: %d\n", result.Merged)
		}
	}
	fmt.Fprintf(out, "  Errors: %d\n", len(result.Errors))

	if len(result.Errors) > 0 {
		fmt.Fprintln(out)
		printWarning(out, "Errors encountered:")
		maxErrors := 10
		for i, err := range result.Errors {
			if i >= maxErrors {
				fmt.Fprintf(out, "  ... and %d more errors\n", len(result.Errors)-maxErrors)
				break
			}
			fmt.Fprintf(out, "  - %s\n", err)
		}
	}

	fmt.Fprintln(out)
	if importDryRun {
		printMuted(out, "Dry-run complete. No changes made.")
	} else {
		printSuccess(out, "Import complete.")
	}

	return nil
}

// detectImportFormat detects the format based on file extension.
func detectImportFormat(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return "json"
	case ".db", ".sqlite", ".sqlite3":
		return "sqlite"
	default:
		return ""
	}
}

// importJSON imports from a JSON file.
func importJSON(ctx context.Context, s *recall.Store, inputPath string, strategy recall.MergeStrategy, dryRun bool) (*recall.ImportResult, error) {
	f, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("open input file: %w", err)
	}
	defer f.Close()

	return s.ImportJSON(ctx, f, strategy, dryRun)
}

// importSQLite imports from a SQLite database file.
func importSQLite(ctx context.Context, s *recall.Store, inputPath string, strategy recall.MergeStrategy, dryRun bool) (*recall.ImportResult, error) {
	// Open the source SQLite database
	srcStore, err := recall.NewStore(inputPath)
	if err != nil {
		return nil, fmt.Errorf("open source database: %w", err)
	}
	defer srcStore.Close()

	// Export the source to a temp JSON file for import
	// This reuses the streaming infrastructure
	tmpFile, err := os.CreateTemp("", "recall-import-*.json")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Export source to temp JSON
	if err := srcStore.ExportJSON(ctx, "import", tmpFile); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("export source to JSON: %w", err)
	}
	tmpFile.Close()

	// Import from temp JSON
	tmpReader, err := os.Open(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("open temp file: %w", err)
	}
	defer tmpReader.Close()

	return s.ImportJSON(ctx, tmpReader, strategy, dryRun)
}
