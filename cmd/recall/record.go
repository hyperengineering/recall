package main

import (
	"context"
	"fmt"

	"github.com/hyperengineering/recall"
	"github.com/spf13/cobra"
)

var recordCmd = &cobra.Command{
	Use:   "record [content]",
	Short: "Record new lore",
	Long: `Record a new piece of experiential knowledge.

Example:
  recall record "Queue consumers benefit from idempotency checks" -c PATTERN_OUTCOME
  recall record "ORM generates N+1 queries without eager loading" -c DEPENDENCY_BEHAVIOR --context "story-2.1"`,
	Args: cobra.ExactArgs(1),
	RunE: runRecord,
}

var (
	recordCategory   string
	recordContext    string
	recordConfidence float64
)

func init() {
	recordCmd.Flags().StringVarP(&recordCategory, "category", "c", "PATTERN_OUTCOME", "Lore category")
	recordCmd.Flags().StringVar(&recordContext, "context", "", "Additional context (story, epic, situation)")
	recordCmd.Flags().Float64Var(&recordConfidence, "confidence", 0.7, "Initial confidence (0.0-1.0)")
}

func runRecord(cmd *cobra.Command, args []string) error {
	client, err := recall.New(loadConfig())
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}
	defer client.Close()

	lore, err := client.Record(context.Background(), recall.RecordParams{
		Content:    args[0],
		Category:   recall.Category(recordCategory),
		Context:    recordContext,
		Confidence: recordConfidence,
	})
	if err != nil {
		return fmt.Errorf("record lore: %w", err)
	}

	fmt.Printf("Recorded: %s\n", lore.ID)
	fmt.Printf("Category: %s\n", lore.Category)
	fmt.Printf("Confidence: %.2f\n", lore.Confidence)

	return nil
}
