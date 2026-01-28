package main

import (
	"os"

	"github.com/hyperengineering/recall"
	"github.com/spf13/cobra"
)

var (
	cfgLorePath  string
	cfgEngramURL string
	cfgAPIKey    string
)

var rootCmd = &cobra.Command{
	Use:   "recall",
	Short: "Recall - Lore management CLI",
	Long: `Recall is a CLI tool for managing experiential lore.

It allows AI agents and developers to capture, query, and synchronize
learned insights across sessions and environments.`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgLorePath, "lore-path", "", "Path to local lore database (default: ./data/lore.db)")
	rootCmd.PersistentFlags().StringVar(&cfgEngramURL, "engram-url", "", "URL of Engram central service")
	rootCmd.PersistentFlags().StringVar(&cfgAPIKey, "api-key", "", "API key for Engram authentication")

	rootCmd.AddCommand(recordCmd)
	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(feedbackCmd)
	rootCmd.AddCommand(statsCmd)
}

func loadConfig() recall.Config {
	cfg := recall.DefaultConfig()

	// Override with flags
	if cfgLorePath != "" {
		cfg.LocalPath = cfgLorePath
	}
	if cfgEngramURL != "" {
		cfg.EngramURL = cfgEngramURL
	}
	if cfgAPIKey != "" {
		cfg.APIKey = cfgAPIKey
	}

	// Override with environment variables
	if v := os.Getenv("RECALL_DB_PATH"); v != "" && cfgLorePath == "" {
		cfg.LocalPath = v
	}
	if v := os.Getenv("ENGRAM_URL"); v != "" && cfgEngramURL == "" {
		cfg.EngramURL = v
	}
	if v := os.Getenv("ENGRAM_API_KEY"); v != "" && cfgAPIKey == "" {
		cfg.APIKey = v
	}

	return cfg
}
