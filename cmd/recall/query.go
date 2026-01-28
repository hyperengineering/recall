package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/hyperengineering/recall"
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query [search terms]",
	Short: "Query for relevant lore",
	Long: `Search for lore semantically similar to the query.

Example:
  recall query "implementing message consumers"
  recall query "database performance" -k 10 --min-confidence 0.7
  recall query "testing strategies" --categories TESTING_STRATEGY,PATTERN_OUTCOME`,
	Args: cobra.ExactArgs(1),
	RunE: runQuery,
}

var (
	queryK             int
	queryMinConfidence float64
	queryCategories    string
)

func init() {
	queryCmd.Flags().IntVarP(&queryK, "limit", "k", 5, "Maximum number of results")
	queryCmd.Flags().Float64Var(&queryMinConfidence, "min-confidence", 0.5, "Minimum confidence threshold")
	queryCmd.Flags().StringVar(&queryCategories, "categories", "", "Comma-separated list of categories to filter")
}

func runQuery(cmd *cobra.Command, args []string) error {
	client, err := recall.New(loadConfig())
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}
	defer client.Close()

	params := recall.QueryParams{
		Query:         args[0],
		K:             queryK,
		MinConfidence: queryMinConfidence,
	}

	if queryCategories != "" {
		cats := strings.Split(queryCategories, ",")
		for _, c := range cats {
			params.Categories = append(params.Categories, recall.Category(strings.TrimSpace(c)))
		}
	}

	result, err := client.Query(context.Background(), params)
	if err != nil {
		return fmt.Errorf("query lore: %w", err)
	}

	if len(result.Lore) == 0 {
		fmt.Println("No matching lore found.")
		return nil
	}

	fmt.Printf("Found %d matching entries:\n\n", len(result.Lore))

	for i, lore := range result.Lore {
		ref := ""
		for r, id := range result.SessionRefs {
			if id == lore.ID {
				ref = r
				break
			}
		}

		fmt.Printf("[%s] %s (confidence: %.2f, validated: %d times)\n",
			ref, lore.Category, lore.Confidence, lore.ValidationCount)
		fmt.Printf("    %s\n", lore.Content)
		if lore.Context != "" {
			fmt.Printf("    Context: %s\n", lore.Context)
		}
		if i < len(result.Lore)-1 {
			fmt.Println()
		}
	}

	return nil
}
