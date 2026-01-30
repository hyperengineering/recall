package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/hyperengineering/recall"
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query <search terms>",
	Short: "Query for relevant lore",
	Long: `Search for lore semantically similar to the query.

Example:
  recall query "implementing message consumers"
  recall query "database performance" --top 10 --min-confidence 0.7
  recall query "testing strategies" --category TESTING_STRATEGY,PATTERN_OUTCOME --json`,
	Args: cobra.ExactArgs(1),
	RunE: runQuery,
}

var (
	queryTop           int
	queryMinConfidence float64
	queryCategory      string
)

func init() {
	queryCmd.Flags().IntVarP(&queryTop, "top", "k", 5, "Maximum number of results")
	queryCmd.Flags().Float64Var(&queryMinConfidence, "min-confidence", 0.0, "Minimum confidence threshold")
	queryCmd.Flags().StringVar(&queryCategory, "category", "", "Comma-separated categories to filter")
}

func runQuery(cmd *cobra.Command, args []string) error {
	cfg, err := loadAndValidateConfig()
	if err != nil {
		return err
	}

	client, err := recall.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}
	defer func() { _ = client.Close() }()

	params := recall.QueryParams{
		Query: args[0],
		K:     queryTop,
	}

	if cmd.Flags().Changed("min-confidence") {
		params.MinConfidence = &queryMinConfidence
	}

	if queryCategory != "" {
		cats := strings.Split(queryCategory, ",")
		for _, c := range cats {
			params.Categories = append(params.Categories, recall.Category(strings.TrimSpace(c)))
		}
	}

	result, err := client.Query(context.Background(), params)
	if err != nil {
		return fmt.Errorf("query lore: %w", err)
	}

	return outputQueryResult(cmd, result)
}
