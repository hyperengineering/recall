package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hyperengineering/recall"
	"github.com/hyperengineering/recall/internal/store"
	"github.com/spf13/cobra"
)

var (
	syncReinit bool
	syncForce  bool
	syncStore  string
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize with Engram",
	Long: `Synchronize local lore with the Engram central service.

Subcommands:
  push      Push local changes to Engram
  bootstrap Download full snapshot from Engram

Flags:
  --reinit  Reinitialize database from Engram (replaces all local data)
  --force   Skip confirmation prompts (for scripting)

Example:
  recall sync push
  recall sync bootstrap
  recall sync --reinit
  recall sync --reinit --force`,
	RunE: runSync,
}

var syncPushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push local changes to Engram",
	Long: `Push pending local lore and feedback to the Engram central service.

Example:
  recall sync push
  recall sync push --json`,
	RunE: runSyncPush,
}

var syncBootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Download full snapshot from Engram",
	Long: `Download a complete lore snapshot from Engram, replacing local data.

This is used to:
  - Initialize a new client with the full knowledge base
  - Refresh local data with complete server state
  - Obtain embeddings for semantic similarity queries

Warning: This replaces ALL local lore with the server snapshot.

Example:
  recall sync bootstrap
  recall sync bootstrap --json`,
	RunE: runSyncBootstrap,
}

var syncDeltaCmd = &cobra.Command{
	Use:   "delta",
	Short: "Fetch incremental updates from Engram",
	Long: `Fetch only changes since the last sync, efficiently keeping local data current.

This is faster than bootstrap for regular updates:
  - Downloads only new, updated, and deleted entries
  - Preserves locally recorded lore
  - Updates the last_sync timestamp on success

On success, displays:
  - Number of entries added or removed (if any)
  - Current local lore count
  - Duration of the sync operation

Requires: Prior bootstrap (run 'recall sync bootstrap' first)

Example:
  recall sync delta
  recall sync delta --json`,
	RunE: runSyncDelta,
}

func init() {
	syncCmd.Flags().BoolVar(&syncReinit, "reinit", false, "Reinitialize database from Engram")
	syncCmd.Flags().BoolVar(&syncForce, "force", false, "Skip confirmation prompts")
	syncCmd.PersistentFlags().StringVar(&syncStore, "store", "", "Store ID to operate against (default: resolved from ENGRAM_STORE or 'default')")
	syncCmd.AddCommand(syncPushCmd)
	syncCmd.AddCommand(syncBootstrapCmd)
	syncCmd.AddCommand(syncDeltaCmd)
}

// loadSyncConfig loads config and applies the --store flag if set.
func loadSyncConfig() (recall.Config, error) {
	cfg, err := loadAndValidateConfig()
	if err != nil {
		return recall.Config{}, err
	}

	// Apply --store flag if set
	if syncStore != "" {
		if err := store.ValidateStoreID(syncStore); err != nil {
			return recall.Config{}, fmt.Errorf("invalid store ID %q: %w", syncStore, err)
		}
		cfg.Store = syncStore
		cfg.LocalPath = store.StoreDBPath(syncStore)
	}

	return cfg, nil
}

func runSyncPush(cmd *cobra.Command, args []string) error {
	cfg, err := loadSyncConfig()
	if err != nil {
		return err
	}

	if cfg.IsOffline() {
		return fmt.Errorf("sync unavailable: ENGRAM_URL not configured (offline-only mode)")
	}

	client, err := recall.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	statsBefore, _ := client.Stats()

	var syncErr error
	start := time.Now()

	out := cmd.OutOrStdout()
	syncErr = runWithSpinner(out, "Pushing to Engram", func() error {
		return client.SyncPush(ctx)
	})

	duration := time.Since(start)

	if syncErr != nil {
		return fmt.Errorf("push: %w", syncErr)
	}

	statsAfter, _ := client.Stats()

	return outputSyncPush(cmd, statsBefore, statsAfter, duration)
}

func runSyncBootstrap(cmd *cobra.Command, args []string) error {
	cfg, err := loadSyncConfig()
	if err != nil {
		return err
	}

	if cfg.IsOffline() {
		return fmt.Errorf("bootstrap unavailable: ENGRAM_URL not configured (offline-only mode)")
	}

	client, err := recall.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var bootstrapErr error
	start := time.Now()

	out := cmd.OutOrStdout()
	bootstrapErr = runWithSpinner(out, "Bootstrapping from Engram", func() error {
		return client.Bootstrap(ctx)
	})

	duration := time.Since(start)

	if bootstrapErr != nil {
		return fmt.Errorf("bootstrap: %w", bootstrapErr)
	}

	stats, _ := client.Stats()

	return outputSyncBootstrap(cmd, stats, duration)
}

// runSync handles the sync command, potentially with --reinit flag.
func runSync(cmd *cobra.Command, args []string) error {
	if !syncReinit {
		// No --reinit flag, show help
		return cmd.Help()
	}

	// --reinit mode
	cfg, err := loadSyncConfig()
	if err != nil {
		return err
	}

	client, err := recall.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}
	defer func() { _ = client.Close() }()

	out := cmd.OutOrStdout()

	// Check for pending sync entries before prompting
	stats, err := client.Stats()
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}

	if stats.PendingSync > 0 {
		printError(out, "Cannot reinitialize: %d unsynced entries in queue", stats.PendingSync)
		printMuted(out, "Run 'recall sync push' first to sync changes, or clear the queue manually")
		return recall.ErrPendingSyncExists
	}

	// Prompt for confirmation unless --force
	if !syncForce {
		warning := fmt.Sprintf("This will REPLACE ALL local lore with data from Engram.\nCurrent local lore count: %d", stats.LoreCount)
		prompt := "Type 'yes' to continue: "
		fmt.Fprint(out, renderConfirmation(warning, prompt))

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "yes" {
			printMuted(out, "Aborted.")
			return nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	opts := recall.ReinitOptions{
		Force: syncForce,
	}

	// Try reinit from Engram first
	var result *recall.ReinitResult
	var reinitErr error
	start := time.Now()

	// The closure captures and assigns to outer-scope variables (result, reinitErr)
	// so they remain accessible after runWithSpinner completes.
	reinitErr = runWithSpinner(out, "Reinitializing from Engram", func() error {
		result, reinitErr = client.Reinitialize(ctx, opts)
		return reinitErr
	})

	duration := time.Since(start)

	// Handle Engram unreachable case
	if reinitErr != nil && !errors.Is(reinitErr, recall.ErrPendingSyncExists) {
		if cfg.IsOffline() || isNetworkError(reinitErr) {
			// Engram unreachable - prompt for empty DB
			if !syncForce {
				warning := "Engram is unreachable."
				prompt := "Create an empty database instead? Type 'yes' to continue: "
				fmt.Fprint(out, renderConfirmation(warning, prompt))

				reader := bufio.NewReader(os.Stdin)
				response, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("read confirmation: %w", err)
				}
				response = strings.TrimSpace(strings.ToLower(response))
				if response != "yes" {
					printMuted(out, "Aborted.")
					return nil
				}
			}

			// Retry with AllowEmpty
			opts.AllowEmpty = true
			start = time.Now()
			reinitErr = runWithSpinner(out, "Creating empty database", func() error {
				result, reinitErr = client.Reinitialize(ctx, opts)
				return reinitErr
			})
			duration = time.Since(start)
		}
	}

	if reinitErr != nil {
		return fmt.Errorf("reinit: %w", reinitErr)
	}

	return outputReinit(cmd, result, duration)
}

// isNetworkError checks if an error indicates a network/connectivity issue.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "network is unreachable") ||
		strings.Contains(errStr, "dial tcp") ||
		errors.Is(err, recall.ErrOffline)
}

func runSyncDelta(cmd *cobra.Command, args []string) error {
	cfg, err := loadSyncConfig()
	if err != nil {
		return err
	}

	if cfg.IsOffline() {
		return fmt.Errorf("delta sync unavailable: ENGRAM_URL not configured (offline-only mode)")
	}

	client, err := recall.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	statsBefore, _ := client.Stats()

	var deltaErr error
	start := time.Now()

	out := cmd.OutOrStdout()
	deltaErr = runWithSpinner(out, "Syncing delta from Engram", func() error {
		return client.SyncDelta(ctx)
	})

	duration := time.Since(start)

	if deltaErr != nil {
		return fmt.Errorf("delta sync: %w", deltaErr)
	}

	statsAfter, _ := client.Stats()

	return outputSyncDelta(cmd, statsBefore, statsAfter, duration)
}
