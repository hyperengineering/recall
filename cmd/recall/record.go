package main

import (
	"fmt"

	"github.com/hyperengineering/recall"
	"github.com/spf13/cobra"
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record new lore",
	Long: `Record a new piece of experiential knowledge.

Example:
  recall record --content "Queue consumers benefit from idempotency checks" --category PATTERN_OUTCOME
  recall record --content "ORM generates N+1 queries" -c DEPENDENCY_BEHAVIOR --context story-2.1 --json`,
	RunE: runRecord,
}

var (
	recordContent    string
	recordCategory   string
	recordContext    string
	recordConfidence float64
)

func init() {
	recordCmd.Flags().StringVar(&recordContent, "content", "", "Lore content (required)")
	recordCmd.Flags().StringVarP(&recordCategory, "category", "c", "", "Lore category (required)")
	recordCmd.Flags().StringVar(&recordContext, "context", "", "Additional context (story, epic, situation)")
	recordCmd.Flags().Float64Var(&recordConfidence, "confidence", 0.5, "Initial confidence (0.0-1.0)")

	recordCmd.MarkFlagRequired("content")
	recordCmd.MarkFlagRequired("category")
}

func runRecord(cmd *cobra.Command, args []string) error {
	cfg, err := loadAndValidateConfig()
	if err != nil {
		return err
	}

	client, err := recall.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}
	defer client.Close()

	// Build options
	var opts []recall.RecordOption
	if recordContext != "" {
		opts = append(opts, recall.WithContext(recordContext))
	}
	if cmd.Flags().Changed("confidence") {
		opts = append(opts, recall.WithConfidence(recordConfidence))
	}

	lore, err := client.Record(recordContent, recall.Category(recordCategory), opts...)
	if err != nil {
		return fmt.Errorf("record lore: %w", err)
	}

	return outputLore(cmd, lore)
}
