package main

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Build-time variables (set via ldflags)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type versionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Display the version, commit hash, build date, and runtime information.`,
	RunE:  runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func runVersion(cmd *cobra.Command, args []string) error {
	info := versionInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
		Go:      runtime.Version(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}

	if outputJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "recall %s\n", info.Version)
	fmt.Fprintf(cmd.OutOrStdout(), "  commit: %s\n", info.Commit)
	fmt.Fprintf(cmd.OutOrStdout(), "  built:  %s\n", info.Date)
	fmt.Fprintf(cmd.OutOrStdout(), "  go:     %s\n", info.Go)
	fmt.Fprintf(cmd.OutOrStdout(), "  os:     %s/%s\n", info.OS, info.Arch)
	return nil
}
