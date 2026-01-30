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
	Long: `Provide feedback on lore to adjust confidence scores.

Confidence adjustments:
  - helpful:      +0.08 confidence
  - incorrect:    -0.15 confidence
  - not_relevant:  no change

Single-item mode:
  recall feedback --id <lore-id> --type helpful
  recall feedback --id L1 --type incorrect

Batch mode:
  recall feedback --helpful L1,L2 --incorrect L3
  recall feedback --helpful "queue consumer idempotency"`,
	RunE: runFeedback,
}

var (
	// Single-item mode
	feedbackID   string
	feedbackType string

	// Batch mode
	feedbackHelpful     string
	feedbackNotRelevant string
	feedbackIncorrect   string
)

var validFeedbackTypes = []string{"helpful", "incorrect", "not_relevant"}

func init() {
	// Single-item flags
	feedbackCmd.Flags().StringVar(&feedbackID, "id", "", "Lore ID or session ref (L1, L2, ...)")
	feedbackCmd.Flags().StringVar(&feedbackType, "type", "", "Feedback type: helpful, incorrect, not_relevant")

	// Batch flags
	feedbackCmd.Flags().StringVar(&feedbackHelpful, "helpful", "", "Comma-separated helpful refs")
	feedbackCmd.Flags().StringVar(&feedbackNotRelevant, "not-relevant", "", "Comma-separated not-relevant refs")
	feedbackCmd.Flags().StringVar(&feedbackIncorrect, "incorrect", "", "Comma-separated incorrect refs")
}

func runFeedback(cmd *cobra.Command, args []string) error {
	cfg, err := loadAndValidateConfig()
	if err != nil {
		return err
	}

	// Determine mode: single-item vs batch
	singleMode := feedbackID != "" || feedbackType != ""
	batchMode := feedbackHelpful != "" || feedbackNotRelevant != "" || feedbackIncorrect != ""

	if singleMode && batchMode {
		return fmt.Errorf("cannot mix --id/--type with batch flags (--helpful, --incorrect, --not-relevant)")
	}

	if !singleMode && !batchMode {
		return fmt.Errorf("provide --id and --type, or use batch flags (--helpful, --incorrect, --not-relevant)")
	}

	client, err := recall.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}
	defer func() { _ = client.Close() }()

	if singleMode {
		return runFeedbackSingle(cmd, client)
	}
	return runFeedbackBatch(cmd, client)
}

func runFeedbackSingle(cmd *cobra.Command, client *recall.Client) error {
	if feedbackID == "" {
		return fmt.Errorf("--id is required in single-item mode")
	}
	if feedbackType == "" {
		return fmt.Errorf("--type is required in single-item mode")
	}

	ft, err := parseFeedbackType(feedbackType)
	if err != nil {
		return err
	}

	lore, err := client.Feedback(feedbackID, ft)
	if err != nil {
		return fmt.Errorf("apply feedback: %w", err)
	}

	return outputFeedbackSingle(cmd, feedbackID, lore)
}

func runFeedbackBatch(cmd *cobra.Command, client *recall.Client) error {
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

	return outputFeedbackBatch(cmd, result)
}

func parseFeedbackType(s string) (recall.FeedbackType, error) {
	normalized := strings.ToLower(strings.TrimSpace(s))
	switch normalized {
	case "helpful":
		return recall.Helpful, nil
	case "incorrect":
		return recall.Incorrect, nil
	case "not_relevant", "not-relevant", "notrelevant":
		return recall.NotRelevant, nil
	default:
		return "", fmt.Errorf("invalid feedback type %q: valid types are %s",
			s, strings.Join(validFeedbackTypes, ", "))
	}
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
