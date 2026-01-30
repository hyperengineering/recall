package main

import (
	"github.com/hyperengineering/recall"
	recallmcp "github.com/hyperengineering/recall/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server for coding agent integration",
	Long: `Start a Model Context Protocol (MCP) server over stdio.

This allows coding agents like Claude Code to use Recall tools directly.

Configuration in Claude Code (~/.claude/claude_desktop_config.json):

  {
    "mcpServers": {
      "recall": {
        "command": "recall",
        "args": ["mcp"],
        "env": {
          "RECALL_DB_PATH": "/path/to/lore.db"
        }
      }
    }
  }

Environment variables:
  RECALL_DB_PATH    Path to local SQLite database (required)
  RECALL_SOURCE_ID  Client identifier (default: hostname)
  ENGRAM_URL        Engram service URL (optional, enables sync)
  ENGRAM_API_KEY    Engram API key (required if ENGRAM_URL set)`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, args []string) error {
	// Load configuration from environment
	cfg := loadConfig()
	if err := validateConfig(cfg); err != nil {
		return err
	}

	// Create Recall client - this persists for the server lifetime
	client, err := recall.New(cfg)
	if err != nil {
		return err
	}
	defer client.Close()

	// Start MCP server over stdio
	server := recallmcp.NewServer(client)
	return server.Run()
}
