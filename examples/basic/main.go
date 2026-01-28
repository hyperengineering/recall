// Package main demonstrates basic usage of the Recall library.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hyperengineering/recall"
)

func main() {
	// Create a client with local-only configuration
	// (offline mode is implied when EngramURL is not set)
	cfg := recall.Config{
		LocalPath: "./data/lore.db",
	}

	client, err := recall.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Record some lore
	fmt.Println("Recording lore...")

	lore1, err := client.Record(
		"Queue consumers benefit from idempotency checks to handle redelivered messages",
		recall.CategoryPatternOutcome,
		recall.WithContext("message-processing-story"),
	)
	if err != nil {
		log.Fatalf("Failed to record lore: %v", err)
	}
	fmt.Printf("Recorded: %s\n", lore1.ID)

	lore2, err := client.Record(
		"ORM generates N+1 queries for belongs_to relationships unless eager loading is configured",
		recall.CategoryDependencyBehavior,
		recall.WithContext("performance-investigation"),
	)
	if err != nil {
		log.Fatalf("Failed to record lore: %v", err)
	}
	fmt.Printf("Recorded: %s\n", lore2.ID)

	lore3, err := client.Record(
		"Integration tests for async workers need to verify idempotency by sending duplicate messages",
		recall.CategoryTestingStrategy,
		recall.WithContext("worker-testing"),
	)
	if err != nil {
		log.Fatalf("Failed to record lore: %v", err)
	}
	fmt.Printf("Recorded: %s\n", lore3.ID)

	// ctx is used below for Query/Feedback
	_ = ctx

	// Query for relevant lore
	fmt.Println("\nQuerying for lore about message handling...")

	minConf := 0.5
	result, err := client.Query(ctx, recall.QueryParams{
		Query:         "implementing message consumers",
		K:             5,
		MinConfidence: &minConf,
	})
	if err != nil {
		log.Fatalf("Failed to query lore: %v", err)
	}

	fmt.Printf("Found %d matching entries:\n", len(result.Lore))
	for _, l := range result.Lore {
		fmt.Printf("  [%s] %s (%.2f)\n", l.Category, l.Content[:50]+"...", l.Confidence)
	}

	// Provide feedback
	fmt.Println("\nProviding feedback...")

	// Find the session refs
	var helpfulRefs []string
	for ref, id := range result.SessionRefs {
		if id == lore1.ID {
			helpfulRefs = append(helpfulRefs, ref)
		}
	}

	if len(helpfulRefs) > 0 {
		feedbackResult, err := client.FeedbackBatch(ctx, recall.FeedbackParams{
			Helpful: helpfulRefs,
		})
		if err != nil {
			log.Fatalf("Failed to apply feedback: %v", err)
		}

		for _, update := range feedbackResult.Updated {
			fmt.Printf("Updated %s: %.2f -> %.2f\n", update.ID[:8], update.Previous, update.Current)
		}
	}

	// Show stats
	fmt.Println("\nStore statistics:")
	stats, err := client.Stats()
	if err != nil {
		log.Fatalf("Failed to get stats: %v", err)
	}
	fmt.Printf("  Lore count: %d\n", stats.LoreCount)
	fmt.Printf("  Pending sync: %d\n", stats.PendingSync)
}
