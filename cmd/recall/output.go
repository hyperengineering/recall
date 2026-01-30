package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hyperengineering/recall"
	"github.com/spf13/cobra"
)

// outputLore prints a single lore entry in the configured format.
func outputLore(cmd *cobra.Command, lore *recall.Lore) error {
	if outputJSON {
		return outputAsJSON(cmd, lore)
	}
	return outputLoreHuman(cmd, lore)
}

// outputAsJSON writes any value as formatted JSON to the command's stdout.
func outputAsJSON(cmd *cobra.Command, v interface{}) error {
	out := cmd.OutOrStdout()
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// outputLoreHuman prints a lore entry in human-readable format.
func outputLoreHuman(cmd *cobra.Command, lore *recall.Lore) error {
	out := cmd.OutOrStdout()
	printSuccess(out, "Recorded: %s", lore.ID)
	fmt.Fprintf(out, "  Category: %s\n", lore.Category)
	fmt.Fprintf(out, "  Confidence: %.2f\n", lore.Confidence)
	if lore.Context != "" {
		fmt.Fprintf(out, "  Context: %s\n", lore.Context)
	}
	return nil
}

// outputText prints text to the command's stdout.
func outputText(cmd *cobra.Command, format string, args ...interface{}) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, format, args...)
}

// outputError prints an error to stderr, ensuring no API keys are leaked.
func outputError(w io.Writer, err error) {
	msg := scrubSensitiveData(err.Error())
	printError(w, "%s", msg)
}

// scrubSensitiveData removes potential API keys from error messages.
// The library already avoids including keys, but this is defense in depth.
func scrubSensitiveData(msg string) string {
	// Scrub anything that looks like a bearer token reference
	// API keys should never appear, but if they do, redact them
	if cfgAPIKey != "" && strings.Contains(msg, cfgAPIKey) {
		msg = strings.ReplaceAll(msg, cfgAPIKey, "[REDACTED]")
	}
	return msg
}

// outputQueryResult prints query results in configured format.
func outputQueryResult(cmd *cobra.Command, result *recall.QueryResult) error {
	if outputJSON {
		return outputAsJSON(cmd, result)
	}
	return outputQueryResultHuman(cmd, result)
}

func outputQueryResultHuman(cmd *cobra.Command, result *recall.QueryResult) error {
	out := cmd.OutOrStdout()

	if len(result.Lore) == 0 {
		printWarning(out, "No matching lore found.")
		return nil
	}

	printInfo(out, "Found %d matching entries:", len(result.Lore))
	fmt.Fprintln(out)

	for i, lore := range result.Lore {
		ref := findRefForID(result.SessionRefs, lore.ID)

		// Header with ref and metadata
		if isTTY() {
			fmt.Fprintf(out, "%s %s %s\n",
				labelStyle.Render(fmt.Sprintf("[%s]", ref)),
				lore.Category,
				mutedStyle.Render(fmt.Sprintf("(confidence: %.2f, validated: %d)", lore.Confidence, lore.ValidationCount)))
		} else {
			fmt.Fprintf(out, "[%s] %s (confidence: %.2f, validated: %d times)\n",
				ref, lore.Category, lore.Confidence, lore.ValidationCount)
		}

		// Content with markdown rendering
		content := renderMarkdown(lore.Content)
		// Indent each line
		// Indent each line, preserving empty lines within content
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			fmt.Fprintf(out, "    %s\n", line)
		}

		if lore.Context != "" {
			if isTTY() {
				fmt.Fprintf(out, "    %s\n", mutedStyle.Render("Context: "+lore.Context))
			} else {
				fmt.Fprintf(out, "    Context: %s\n", lore.Context)
			}
		}
		if i < len(result.Lore)-1 {
			fmt.Fprintln(out)
		}
	}

	return nil
}

func findRefForID(refs map[string]string, id string) string {
	for ref, refID := range refs {
		if refID == id {
			return ref
		}
	}
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

// outputFeedbackSingle prints single feedback result.
func outputFeedbackSingle(cmd *cobra.Command, ref string, lore *recall.Lore) error {
	if outputJSON {
		return outputAsJSON(cmd, map[string]interface{}{
			"ref":              ref,
			"id":               lore.ID,
			"confidence":       lore.Confidence,
			"validation_count": lore.ValidationCount,
		})
	}

	out := cmd.OutOrStdout()
	printSuccess(out, "Feedback applied to %s", ref)
	fmt.Fprintf(out, "  ID: %s\n", lore.ID)
	fmt.Fprintf(out, "  Confidence: %.2f\n", lore.Confidence)
	fmt.Fprintf(out, "  Validation count: %d\n", lore.ValidationCount)
	return nil
}

// outputFeedbackBatch prints batch feedback results.
func outputFeedbackBatch(cmd *cobra.Command, result *recall.FeedbackResult) error {
	if outputJSON {
		return outputAsJSON(cmd, result)
	}

	out := cmd.OutOrStdout()

	if len(result.Updated) == 0 {
		printWarning(out, "No lore entries were updated.")
		return nil
	}

	printSuccess(out, "Updated %d entries:", len(result.Updated))
	for _, update := range result.Updated {
		direction := "→"
		if update.Current > update.Previous {
			direction = "↑"
		} else if update.Current < update.Previous {
			direction = "↓"
		}
		idShort := update.ID
		if len(idShort) > 8 {
			idShort = idShort[:8]
		}
		fmt.Fprintf(out, "  %s: %.2f %s %.2f (validated: %d)\n",
			idShort, update.Previous, direction, update.Current, update.ValidationCount)
	}
	return nil
}

// SyncPushResult for JSON output.
type SyncPushResult struct {
	Pushed     int   `json:"pushed"`
	Remaining  int   `json:"remaining"`
	DurationMs int64 `json:"duration_ms"`
}

// outputSyncPush prints push sync results.
func outputSyncPush(cmd *cobra.Command, before, after *recall.StoreStats, duration time.Duration) error {
	pushed := 0
	if before != nil && after != nil {
		pushed = before.PendingSync - after.PendingSync
	}

	if outputJSON {
		remaining := 0
		if after != nil {
			remaining = after.PendingSync
		}
		return outputAsJSON(cmd, SyncPushResult{
			Pushed:     pushed,
			Remaining:  remaining,
			DurationMs: duration.Milliseconds(),
		})
	}

	out := cmd.OutOrStdout()
	printSuccess(out, "Push complete (took %s)", duration.Round(time.Millisecond))
	if pushed > 0 {
		fmt.Fprintf(out, "  Pushed %d entries\n", pushed)
	}
	if after != nil && after.PendingSync > 0 {
		printWarning(out, "Remaining in queue: %d", after.PendingSync)
	}
	return nil
}

// SyncBootstrapResult for JSON output.
type SyncBootstrapResult struct {
	LoreCount  int   `json:"lore_count"`
	DurationMs int64 `json:"duration_ms"`
}

// outputSyncBootstrap prints bootstrap results.
func outputSyncBootstrap(cmd *cobra.Command, stats *recall.StoreStats, duration time.Duration) error {
	if outputJSON {
		count := 0
		if stats != nil {
			count = stats.LoreCount
		}
		return outputAsJSON(cmd, SyncBootstrapResult{
			LoreCount:  count,
			DurationMs: duration.Milliseconds(),
		})
	}

	out := cmd.OutOrStdout()
	printSuccess(out, "Bootstrap complete (took %s)", duration.Round(time.Millisecond))
	if stats != nil {
		fmt.Fprintf(out, "  Local lore count: %d\n", stats.LoreCount)
	}
	return nil
}

// outputSessionLore prints session lore list.
func outputSessionLore(cmd *cobra.Command, lore []recall.SessionLore) error {
	if outputJSON {
		return outputAsJSON(cmd, lore)
	}

	out := cmd.OutOrStdout()

	if len(lore) == 0 {
		printWarning(out, "No lore surfaced in current session.")
		printMuted(out, "(Tip: Use 'query' to surface lore, then 'session' to list it)")
		return nil
	}

	printInfo(out, "Session lore (%d entries):", len(lore))
	fmt.Fprintln(out)
	for _, l := range lore {
		fmt.Fprintf(out, "[%s] %s (confidence: %.2f)\n", l.SessionRef, l.Category, l.Confidence)
		fmt.Fprintf(out, "    %s\n", l.Content)
		fmt.Fprintf(out, "    ID: %s\n\n", l.ID)
	}
	return nil
}
