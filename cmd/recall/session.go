package main

import (
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "List lore surfaced during the current session",
	Long: `Display all lore entries that were surfaced (queried) during the current CLI session.

Each entry shows its session reference (L1, L2, ...) for use with the feedback command.

Note: Session tracking is per-process. In CLI mode, each command invocation
is a separate session. This command is more useful in interactive/REPL modes.`,
	RunE: runSession,
}

func runSession(cmd *cobra.Command, args []string) error {
	if _, err := loadAndValidateConfig(); err != nil {
		return err
	}

	// For CLI, session is ephemeral per invocation
	// The session command makes more sense in interactive mode
	// For now, explain the limitation

	if outputJSON {
		return outputAsJSON(cmd, []interface{}{})
	}

	outputText(cmd, "No lore surfaced in current session.\n")
	outputText(cmd, "(CLI commands run in separate sessions — use 'query' first, then 'feedback')\n")
	return nil
}
