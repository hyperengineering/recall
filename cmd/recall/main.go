package main

import (
	"os"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		// Cobra already prints the error, but we re-print through outputError
		// to ensure API keys are scrubbed from any error messages (defense in depth)
		outputError(os.Stderr, err)
		os.Exit(1)
	}
}
