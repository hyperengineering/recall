package main

import (
	"context"
	"fmt"
	"time"

	"github.com/hyperengineering/recall"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show store statistics",
	Long: `Display statistics about the local lore store.

Example:
  recall stats
  recall stats --health
  recall stats --json`,
	RunE: runStats,
}

var statsHealth bool

func init() {
	statsCmd.Flags().BoolVar(&statsHealth, "health", false, "Include health check")
}

// statsOutput represents the JSON output structure for stats
type statsOutput struct {
	LoreCount     int        `json:"lore_count"`
	PendingSync   int        `json:"pending_sync"`
	SchemaVersion string     `json:"schema_version"`
	LastSync      *time.Time `json:"last_sync,omitempty"`
	Health        *healthOutput `json:"health,omitempty"`
}

type healthOutput struct {
	Healthy        bool   `json:"healthy"`
	StoreOK        bool   `json:"store_ok"`
	EngramReachable bool  `json:"engram_reachable"`
	Error          string `json:"error,omitempty"`
}

func runStats(cmd *cobra.Command, args []string) error {
	client, err := recall.New(loadConfig())
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}
	defer client.Close()

	stats, err := client.Stats()
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}

	out := cmd.OutOrStdout()

	// Build output structure
	result := statsOutput{
		LoreCount:     stats.LoreCount,
		PendingSync:   stats.PendingSync,
		SchemaVersion: stats.SchemaVersion,
	}
	if !stats.LastSync.IsZero() {
		result.LastSync = &stats.LastSync
	}

	// Include health check if requested
	var health *recall.HealthStatus
	if statsHealth {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		h := client.HealthCheck(ctx)
		health = &h
		result.Health = &healthOutput{
			Healthy:        h.Healthy,
			StoreOK:        h.StoreOK,
			EngramReachable: h.EngramReachable,
			Error:          h.Error,
		}
	}

	// JSON output
	if outputJSON {
		return outputAsJSON(cmd, result)
	}

	// Human-readable output with styling
	printInfo(out, "Local Store Statistics")
	fmt.Fprintf(out, "  Lore count:     %d\n", stats.LoreCount)
	fmt.Fprintf(out, "  Pending sync:   %d\n", stats.PendingSync)
	fmt.Fprintf(out, "  Schema version: %s\n", stats.SchemaVersion)

	if !stats.LastSync.IsZero() {
		fmt.Fprintf(out, "  Last sync:      %s (%s ago)\n",
			stats.LastSync.Format(time.RFC3339),
			time.Since(stats.LastSync).Round(time.Minute))
	} else {
		printMuted(out, "  Last sync:      never")
	}

	if health != nil {
		fmt.Fprintln(out)
		printInfo(out, "Health Check")

		if health.Healthy {
			printSuccess(out, "Status: healthy")
		} else {
			printError(out, "Status: unhealthy")
		}

		if health.StoreOK {
			fmt.Fprintf(out, "  Store:  %s\n", successStyle.Render("OK"))
		} else {
			fmt.Fprintf(out, "  Store:  %s\n", errorStyle.Render("FAILED"))
		}

		if health.EngramReachable {
			fmt.Fprintf(out, "  Engram: %s\n", successStyle.Render("reachable"))
		} else {
			fmt.Fprintf(out, "  Engram: %s\n", warningStyle.Render("unreachable"))
		}

		if health.Error != "" {
			printError(out, "Error: %s", health.Error)
		}
	}

	return nil
}
