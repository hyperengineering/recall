package main

import (
	"fmt"
	"sort"

	"github.com/hyperengineering/recall"
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "List lore surfaced during the current session",
	Long: `Display all lore entries that were surfaced (queried) during the current CLI session.

Each entry shows its session reference (L1, L2, ...) for use with the feedback command.

Note: In CLI mode, each command invocation is a separate session.
Use 'query' first to surface lore, then use 'feedback' within the same
process or reference lore by ID.

Example:
  recall session
  recall session --json`,
	RunE: runSession,
}

func runSession(cmd *cobra.Command, args []string) error {
	cfg, err := loadAndValidateConfig()
	if err != nil {
		return err
	}

	client, err := recall.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}
	defer func() { _ = client.Close() }()

	sessionLore := client.GetSessionLore()

	// Sort by session ref (L1, L2, L3...)
	sort.Slice(sessionLore, func(i, j int) bool {
		return sessionLore[i].SessionRef < sessionLore[j].SessionRef
	})

	return outputSessionLore(cmd, sessionLore)
}
