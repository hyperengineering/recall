package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

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
	fmt.Fprintf(out, "Recorded: %s\n", lore.ID)
	fmt.Fprintf(out, "Category: %s\n", lore.Category)
	fmt.Fprintf(out, "Confidence: %.2f\n", lore.Confidence)
	if lore.Context != "" {
		fmt.Fprintf(out, "Context: %s\n", lore.Context)
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
	fmt.Fprintf(w, "Error: %s\n", msg)
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
