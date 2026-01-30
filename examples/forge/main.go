// Package main demonstrates Recall integration with a Forge-like agent framework.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/hyperengineering/recall"
)

func main() {
	// Configure for Forge environment
	cfg := recall.Config{
		LocalPath: getEnv("FORGE_DATA_DIR", "./data") + "/lore.db",
		EngramURL: os.Getenv("ENGRAM_URL"),
		APIKey:    os.Getenv("ENGRAM_API_KEY"),
		SourceID:  getHostname(),
	}

	// Create client
	client, err := recall.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create Recall client: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// Sync on startup (pull latest from Engram)
	if cfg.EngramURL != "" {
		fmt.Println("Syncing with Engram...")
		if err := client.SyncPull(ctx); err != nil {
			log.Printf("Warning: Initial sync failed: %v", err)
		}
	}

	// Simulate agent workflow with passive injection
	fmt.Println("\n=== Agent Workflow Start ===")
	fmt.Println("Querying for relevant context...")

	// Query for lore relevant to current task
	minConf := 0.6
	result, err := client.Query(ctx, recall.QueryParams{
		Query:         "implementing REST API endpoints with authentication",
		K:             5,
		MinConfidence: &minConf,
		Categories: []recall.Category{
			recall.CategoryPatternOutcome,
			recall.CategoryInterfaceLesson,
			recall.CategoryDependencyBehavior,
		},
	})
	if err != nil {
		log.Fatalf("Failed to query lore: %v", err)
	}

	// Display injected lore (passive injection)
	if len(result.Lore) > 0 {
		fmt.Println("\n## Relevant Lore")
		for ref, id := range result.SessionRefs {
			for _, l := range result.Lore {
				if l.ID == id {
					fmt.Printf("[%s] %s (confidence: %.2f, validated: %d times)\n",
						ref, l.Category, l.Confidence, l.ValidationCount)
					fmt.Printf("    %s\n\n", l.Content)
					break
				}
			}
		}
	} else {
		fmt.Println("No relevant lore found for this context.")
	}

	// Simulate agent work...
	fmt.Println("=== Agent performing task ===")
	fmt.Println("(Agent implements the feature using the context from lore)")

	// Agent discovers something new during implementation
	fmt.Println("\n=== Agent discovers new insight ===")
	newLore, err := client.Record(
		"JWT tokens should include a jti claim for revocation support in distributed systems",
		recall.CategoryPatternOutcome,
		recall.WithContext("auth-api-implementation"),
		recall.WithConfidence(0.7),
	)
	if err != nil {
		log.Printf("Warning: Failed to record lore: %v", err)
	} else {
		fmt.Printf("Recorded new insight: %s\n", newLore.ID)
	}

	// ctx is used above for Sync and Query
	_ = ctx

	// Task completion - agent provides feedback
	fmt.Println("\n=== Task Complete - Providing Feedback ===")

	// Get all session lore for reference
	sessionLore := client.GetSessionLore()
	fmt.Printf("Session tracked %d lore entries\n", len(sessionLore))

	// Identify which lore was helpful (in real usage, the agent would determine this)
	var helpfulRefs []string
	for ref := range result.SessionRefs {
		helpfulRefs = append(helpfulRefs, ref)
	}

	if len(helpfulRefs) > 0 {
		feedbackResult, err := client.FeedbackBatch(ctx, recall.FeedbackParams{
			Helpful: helpfulRefs[:1], // Mark first as helpful for demo
		})
		if err != nil {
			log.Printf("Warning: Failed to apply feedback: %v", err)
		} else {
			for _, update := range feedbackResult.Updated {
				fmt.Printf("Updated %s: %.2f -> %.2f (validated %d times)\n",
					update.ID[:8], update.Previous, update.Current, update.ValidationCount)
			}
		}
	}

	// Show final stats
	fmt.Println("\n=== Session Summary ===")
	stats, _ := client.Stats()
	fmt.Printf("Local lore count: %d\n", stats.LoreCount)
	fmt.Printf("Pending sync: %d\n", stats.PendingSync)

	// Sync on shutdown would happen via client.Close()
	fmt.Println("\nShutting down (will flush pending to Engram if configured)...")
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
