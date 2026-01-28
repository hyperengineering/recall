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
	cfg := recall.Config{
		LocalPath:   "./data/lore.db",
		OfflineMode: true, // No Engram connection
	}

	client, err := recall.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Record some lore
	fmt.Println("Recording lore...")

	lore1, err := client.Record(ctx, recall.RecordParams{
		Content:  "Queue consumers benefit from idempotency checks to handle redelivered messages",
		Category: recall.CategoryPatternOutcome,
		Context:  "message-processing-story",
	})
	if err != nil {
		log.Fatalf("Failed to record lore: %v", err)
	}
	fmt.Printf("Recorded: %s\n", lore1.ID)

	lore2, err := client.Record(ctx, recall.RecordParams{
		Content:  "ORM generates N+1 queries for belongs_to relationships unless eager loading is configured",
		Category: recall.CategoryDependencyBehavior,
		Context:  "performance-investigation",
	})
	if err != nil {
		log.Fatalf("Failed to record lore: %v", err)
	}
	fmt.Printf("Recorded: %s\n", lore2.ID)

	lore3, err := client.Record(ctx, recall.RecordParams{
		Content:  "Integration tests for async workers need to verify idempotency by sending duplicate messages",
		Category: recall.CategoryTestingStrategy,
		Context:  "worker-testing",
	})
	if err != nil {
		log.Fatalf("Failed to record lore: %v", err)
	}
	fmt.Printf("Recorded: %s\n", lore3.ID)

	// Query for relevant lore
	fmt.Println("\nQuerying for lore about message handling...")

	result, err := client.Query(ctx, recall.QueryParams{
		Query:         "implementing message consumers",
		K:             5,
		MinConfidence: 0.5,
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
		feedbackResult, err := client.Feedback(ctx, recall.FeedbackParams{
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
