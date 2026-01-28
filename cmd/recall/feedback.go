package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/hyperengineering/recall"
	"github.com/spf13/cobra"
)

var feedbackCmd = &cobra.Command{
	Use:   "feedback",
	Short: "Provide feedback on recalled lore",
	Long: `Provide feedback on lore that was surfaced during a session.

This command updates confidence scores based on your feedback:
  - helpful: +0.08 confidence
  - incorrect: -0.15 confidence
  - not-relevant: no change (context mismatch, not lore quality)

Example:
  recall feedback --helpful L1,L2 --incorrect L3
  recall feedback --helpful "queue consumer idempotency"`,
	RunE: runFeedback,
}

var (
	feedbackHelpful     string
	feedbackNotRelevant string
	feedbackIncorrect   string
)

func init() {
	feedbackCmd.Flags().StringVar(&feedbackHelpful, "helpful", "", "Comma-separated list of helpful lore refs")
	feedbackCmd.Flags().StringVar(&feedbackNotRelevant, "not-relevant", "", "Comma-separated list of not-relevant lore refs")
	feedbackCmd.Flags().StringVar(&feedbackIncorrect, "incorrect", "", "Comma-separated list of incorrect lore refs")
}

func runFeedback(cmd *cobra.Command, args []string) error {
	if feedbackHelpful == "" && feedbackNotRelevant == "" && feedbackIncorrect == "" {
		return fmt.Errorf("at least one feedback flag is required")
	}

	client, err := recall.New(loadConfig())
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}
	defer client.Close()

	params := recall.FeedbackParams{}

	if feedbackHelpful != "" {
		params.Helpful = splitAndTrim(feedbackHelpful)
	}
	if feedbackNotRelevant != "" {
		params.NotRelevant = splitAndTrim(feedbackNotRelevant)
	}
	if feedbackIncorrect != "" {
		params.Incorrect = splitAndTrim(feedbackIncorrect)
	}

	result, err := client.FeedbackBatch(context.Background(), params)
	if err != nil {
		return fmt.Errorf("apply feedback: %w", err)
	}

	if len(result.Updated) == 0 {
		fmt.Println("No lore entries were updated.")
		return nil
	}

	fmt.Printf("Updated %d entries:\n", len(result.Updated))
	for _, update := range result.Updated {
		direction := "→"
		if update.Current > update.Previous {
			direction = "↑"
		} else if update.Current < update.Previous {
			direction = "↓"
		}
		fmt.Printf("  %s: %.2f %s %.2f (validated: %d)\n",
			update.ID[:8], update.Previous, direction, update.Current, update.ValidationCount)
	}

	return nil
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
