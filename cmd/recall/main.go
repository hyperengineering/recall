package main

import (
	"os"
)

func main() {
	// Initialize styled help after all commands are registered
	initHelp(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		// Print styled error with API key scrubbing (defense in depth)
		outputError(os.Stderr, err)
		os.Exit(1)
	}
}
