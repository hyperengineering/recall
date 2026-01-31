package main

import (
	"context"
	"fmt"
	"strings"
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
	defer func() { _ = client.Close() }()

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
	var statsContent strings.Builder
	statsContent.WriteString(fmt.Sprintf("Lore count:     %d\n", stats.LoreCount))
	statsContent.WriteString(fmt.Sprintf("Pending sync:   %d\n", stats.PendingSync))
	statsContent.WriteString(fmt.Sprintf("Schema version: %s\n", stats.SchemaVersion))
	if !stats.LastSync.IsZero() {
		statsContent.WriteString(fmt.Sprintf("Last sync:      %s (%s ago)",
			stats.LastSync.Format(time.RFC3339),
			time.Since(stats.LastSync).Round(time.Minute)))
	} else {
		statsContent.WriteString("Last sync:      never")
	}

	_, _ = fmt.Fprintln(out, renderPanel("Local Store Statistics", statsContent.String()))

	if health != nil {
		var healthContent strings.Builder
		if health.Healthy {
			healthContent.WriteString(fmt.Sprintf("%s Status: healthy\n", iconSuccess))
		} else {
			healthContent.WriteString(fmt.Sprintf("%s Status: unhealthy\n", iconError))
		}

		if health.StoreOK {
			healthContent.WriteString(fmt.Sprintf("Store:  %s\n", successStyle.Render("OK")))
		} else {
			healthContent.WriteString(fmt.Sprintf("Store:  %s\n", errorStyle.Render("FAILED")))
		}

		if health.EngramReachable {
			healthContent.WriteString(fmt.Sprintf("Engram: %s", successStyle.Render("reachable")))
		} else {
			healthContent.WriteString(fmt.Sprintf("Engram: %s", warningStyle.Render("unreachable")))
		}

		if health.Error != "" {
			healthContent.WriteString(fmt.Sprintf("\n%s Error: %s", iconError, health.Error))
		}

		_, _ = fmt.Fprintln(out, renderPanel("Health Check", healthContent.String()))
	}

	return nil
}
