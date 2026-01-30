package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/hyperengineering/recall"
	"github.com/spf13/cobra"
)

var (
	cfgLorePath  string
	cfgEngramURL string
	cfgAPIKey    string
	cfgSourceID  string
	outputJSON   bool
)

var rootCmd = &cobra.Command{
	Use:   "recall",
	Short: "Recall - Lore management CLI",
	Long: `Recall is a CLI tool for managing experiential lore.

It allows AI agents and developers to capture, query, and synchronize
learned insights across sessions and environments.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(renderBannerWithTagline())
		fmt.Println()
		cmd.Help()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgLorePath, "lore-path", "", "Path to local lore database (default: ./data/lore.db)")
	rootCmd.PersistentFlags().StringVar(&cfgEngramURL, "engram-url", "", "URL of Engram central service")
	rootCmd.PersistentFlags().StringVar(&cfgAPIKey, "api-key", "", "API key for Engram authentication")
	rootCmd.PersistentFlags().StringVar(&cfgSourceID, "source-id", "", "Client source identifier")
	rootCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "Output as JSON")

	rootCmd.AddCommand(recordCmd)
	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(feedbackCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(sessionCmd)
}

func loadConfig() recall.Config {
	cfg := recall.DefaultConfig()

	// Override with flags (flags take priority over env vars)
	if cfgLorePath != "" {
		cfg.LocalPath = cfgLorePath
	}
	if cfgEngramURL != "" {
		cfg.EngramURL = cfgEngramURL
	}
	if cfgAPIKey != "" {
		cfg.APIKey = cfgAPIKey
	}
	if cfgSourceID != "" {
		cfg.SourceID = cfgSourceID
	}

	// Override with environment variables (only if flags not set)
	if v := os.Getenv("RECALL_DB_PATH"); v != "" && cfgLorePath == "" {
		cfg.LocalPath = v
	}
	if v := os.Getenv("ENGRAM_URL"); v != "" && cfgEngramURL == "" {
		cfg.EngramURL = v
	}
	if v := os.Getenv("ENGRAM_API_KEY"); v != "" && cfgAPIKey == "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("RECALL_SOURCE_ID"); v != "" && cfgSourceID == "" {
		cfg.SourceID = v
	}

	return cfg
}

// loadAndValidateConfig loads config from flags/env and validates it.
// This is a convenience wrapper for commands that need validated config.
func loadAndValidateConfig() (recall.Config, error) {
	cfg := loadConfig()
	if err := validateConfig(cfg); err != nil {
		return recall.Config{}, err
	}
	return cfg, nil
}

// validateConfig checks config and returns user-friendly error.
func validateConfig(cfg recall.Config) error {
	if err := cfg.Validate(); err != nil {
		var ve *recall.ValidationError
		if errors.As(err, &ve) {
			envVar := fieldToEnvVar(ve.Field)
			flag := fieldToFlag(ve.Field)
			return fmt.Errorf("configuration: %s: %s — set %s or use --%s",
				ve.Field, ve.Message, envVar, flag)
		}
		return err
	}
	return nil
}

func fieldToEnvVar(field string) string {
	switch field {
	case "LocalPath":
		return "RECALL_DB_PATH"
	case "EngramURL":
		return "ENGRAM_URL"
	case "APIKey":
		return "ENGRAM_API_KEY"
	case "SourceID":
		return "RECALL_SOURCE_ID"
	default:
		return "RECALL_" + strings.ToUpper(field)
	}
}

func fieldToFlag(field string) string {
	switch field {
	case "LocalPath":
		return "lore-path"
	case "EngramURL":
		return "engram-url"
	case "APIKey":
		return "api-key"
	case "SourceID":
		return "source-id"
	default:
		return strings.ToLower(field)
	}
}
