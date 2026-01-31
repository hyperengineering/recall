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

var storeExportCmd = &cobra.Command{
	Use:   "export <store-id>",
	Short: "Export store data to a file",
	Long: `Export all lore from a store to a backup file.

Supports JSON (default) and SQLite formats. JSON exports stream data
to avoid memory issues with large stores.

Examples:
  recall store export my-store -o backup.json
  recall store export my-store -o backup.db --format sqlite
  recall store export default -o default-backup.json`,
	Args: cobra.ExactArgs(1),
	RunE: runStoreExport,
}

var (
	exportOutputPath string
	exportFormat     string
)

func init() {
	storeExportCmd.Flags().StringVarP(&exportOutputPath, "output", "o", "", "Output file path (required)")
	storeExportCmd.Flags().StringVar(&exportFormat, "format", "json", "Export format: json, sqlite")
	_ = storeExportCmd.MarkFlagRequired("output")

	storeCmd.AddCommand(storeExportCmd)
}

// ExportResult for JSON output.
type ExportResult struct {
	StoreID   string `json:"store_id"`
	Format    string `json:"format"`
	LoreCount int    `json:"lore_count"`
	FilePath  string `json:"file_path"`
	FileSize  int64  `json:"file_size"`
	Duration  string `json:"duration"`
}

func runStoreExport(cmd *cobra.Command, args []string) error {
	storeID := args[0]
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	// Validate store ID
	if err := store.ValidateStoreID(storeID); err != nil {
		return fmt.Errorf("invalid store ID %q: %w", storeID, err)
	}

	// Validate format
	format := strings.ToLower(exportFormat)
	if format != "json" && format != "sqlite" {
		return fmt.Errorf("invalid format %q: must be 'json' or 'sqlite'", exportFormat)
	}

	// Check if store exists
	dbPath := store.StoreDBPath(storeID)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("store %q not found", storeID)
	}

	// Open store
	s, err := recall.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	// Get lore count before export
	loreCount, err := s.LoreCount()
	if err != nil {
		return fmt.Errorf("get lore count: %w", err)
	}

	if !outputJSON {
		printInfo(out, "Exporting store '%s' to %s...", storeID, exportOutputPath)
		fmt.Fprintf(out, "  Format: %s\n", strings.ToUpper(format))
	}

	startTime := time.Now()

	// Perform export
	var fileSize int64
	switch format {
	case "json":
		fileSize, err = exportJSON(ctx, s, storeID, exportOutputPath)
	case "sqlite":
		fileSize, err = exportSQLite(ctx, s, exportOutputPath)
	}

	if err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	duration := time.Since(startTime)

	// Get actual file size
	if fi, statErr := os.Stat(exportOutputPath); statErr == nil {
		fileSize = fi.Size()
	}

	if outputJSON {
		return outputAsJSON(cmd, ExportResult{
			StoreID:   storeID,
			Format:    format,
			LoreCount: loreCount,
			FilePath:  exportOutputPath,
			FileSize:  fileSize,
			Duration:  duration.Round(time.Millisecond).String(),
		})
	}

	// Build summary panel
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Lore entries: %d\n", loreCount))
	summary.WriteString(fmt.Sprintf("File size:    %s\n", formatBytes(fileSize)))
	summary.WriteString(fmt.Sprintf("Duration:     %s\n", duration.Round(time.Millisecond)))
	summary.WriteString(fmt.Sprintf("Output:       %s", exportOutputPath))

	fmt.Fprintln(out, renderPanel("Export Summary", summary.String()))
	printSuccess(out, "Export complete")

	return nil
}

// ensureParentDir creates the parent directory of path if it doesn't exist.
func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}
	return nil
}

// exportJSON exports the store to a JSON file.
func exportJSON(ctx context.Context, s *recall.Store, storeID, destPath string) (int64, error) {
	// Ensure output directory exists
	if err := ensureParentDir(destPath); err != nil {
		return 0, err
	}

	// Create output file
	f, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()

	// Stream export
	if err := s.ExportJSON(ctx, storeID, f); err != nil {
		_ = os.Remove(destPath)
		return 0, err
	}

	if err := f.Sync(); err != nil {
		return 0, fmt.Errorf("sync file: %w", err)
	}

	fi, err := f.Stat()
	if err != nil {
		return 0, nil
	}
	return fi.Size(), nil
}

// exportSQLite exports the store to a SQLite file.
func exportSQLite(ctx context.Context, s *recall.Store, destPath string) (int64, error) {
	// Ensure output directory exists
	if err := ensureParentDir(destPath); err != nil {
		return 0, err
	}

	if err := s.ExportSQLite(ctx, destPath); err != nil {
		return 0, err
	}

	fi, err := os.Stat(destPath)
	if err != nil {
		return 0, nil
	}
	return fi.Size(), nil
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
