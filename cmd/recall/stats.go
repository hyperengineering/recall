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
  recall stats --health`,
	RunE: runStats,
}

var statsHealth bool

func init() {
	statsCmd.Flags().BoolVar(&statsHealth, "health", false, "Include health check")
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

	fmt.Println("Local Store Statistics")
	fmt.Println("----------------------")
	fmt.Printf("Lore count:     %d\n", stats.LoreCount)
	fmt.Printf("Pending sync:   %d\n", stats.PendingSync)
	fmt.Printf("Schema version: %s\n", stats.SchemaVersion)

	if !stats.LastSync.IsZero() {
		fmt.Printf("Last sync:      %s (%s ago)\n",
			stats.LastSync.Format(time.RFC3339),
			time.Since(stats.LastSync).Round(time.Minute))
	} else {
		fmt.Println("Last sync:      never")
	}

	if statsHealth {
		fmt.Println()
		fmt.Println("Health Check")
		fmt.Println("------------")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		health := client.HealthCheck(ctx)

		status := "healthy"
		if !health.Healthy {
			status = "unhealthy"
		}
		fmt.Printf("Status:           %s\n", status)
		fmt.Printf("Store OK:         %v\n", health.StoreOK)
		fmt.Printf("Engram reachable: %v\n", health.EngramReachable)

		if health.Error != "" {
			fmt.Printf("Error:            %s\n", health.Error)
		}
	}

	return nil
}
