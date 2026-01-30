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

Subcommands:
  push      Push local changes to Engram
  bootstrap Download full snapshot from Engram

Example:
  recall sync push
  recall sync bootstrap`,
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

func init() {
	syncCmd.AddCommand(syncPushCmd)
	syncCmd.AddCommand(syncBootstrapCmd)
}

func runSyncPush(cmd *cobra.Command, args []string) error {
	cfg, err := loadAndValidateConfig()
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
	defer client.Close()

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
	cfg, err := loadAndValidateConfig()
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
	defer client.Close()

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
