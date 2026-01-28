package main

import (
	"context"
	"fmt"
	"time"

	"github.com/hyperengineering/recall"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize with Engram",
	Long: `Synchronize local lore with the Engram central service.

Example:
  recall sync           # Full sync (push + pull)
  recall sync --push    # Push local changes only
  recall sync --pull    # Pull remote changes only`,
	RunE: runSync,
}

var (
	syncPush bool
	syncPull bool
)

func init() {
	syncCmd.Flags().BoolVar(&syncPush, "push", false, "Push local changes only")
	syncCmd.Flags().BoolVar(&syncPull, "pull", false, "Pull remote changes only")
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg := loadConfig()
	if cfg.EngramURL == "" {
		return fmt.Errorf("ENGRAM_URL not configured")
	}

	client, err := recall.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	start := time.Now()

	if syncPush && !syncPull {
		fmt.Println("Pushing local changes...")
		if err := client.SyncPush(ctx); err != nil {
			return fmt.Errorf("push: %w", err)
		}
		fmt.Printf("Push complete (took %s)\n", time.Since(start).Round(time.Millisecond))
		return nil
	}

	if syncPull && !syncPush {
		fmt.Println("Pulling remote changes...")
		if err := client.SyncPull(ctx); err != nil {
			return fmt.Errorf("pull: %w", err)
		}
		fmt.Printf("Pull complete (took %s)\n", time.Since(start).Round(time.Millisecond))
		return nil
	}

	// Full sync
	fmt.Println("Synchronizing with Engram...")
	if err := client.Sync(ctx); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	fmt.Printf("Sync complete (took %s)\n", time.Since(start).Round(time.Millisecond))

	// Show stats
	stats, err := client.Stats()
	if err == nil {
		fmt.Printf("Local lore count: %d\n", stats.LoreCount)
		fmt.Printf("Pending sync: %d\n", stats.PendingSync)
	}

	return nil
}
