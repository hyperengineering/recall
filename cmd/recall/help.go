package main

import (
	"text/template"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// Help styling with brand colors (reuses colors from styles.go)
var (
	helpHeaderStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	helpCmdStyle    = lipgloss.NewStyle().Foreground(colorPrimaryLight)
)

// Template functions for styled help
var helpTemplateFuncs = template.FuncMap{
	"header": func(s string) string {
		if isTTY() {
			return helpHeaderStyle.Render(s)
		}
		return s
	},
	"cmd": func(s string) string {
		if isTTY() {
			return helpCmdStyle.Render(s)
		}
		return s
	},
	"muted": func(s string) string {
		if isTTY() {
			return mutedStyle.Render(s) // Reuse mutedStyle from styles.go
		}
		return s
	},
}

// Custom help template with styling
const helpTemplate = `{{with .Long}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{header "Usage:"}}
  {{cmd .CommandPath}}{{if .HasAvailableSubCommands}} {{muted "[command]"}}{{end}}{{if .HasAvailableFlags}} {{muted "[flags]"}}{{end}}

{{end}}{{if .HasAvailableSubCommands}}{{header "Commands:"}}
{{range .Commands}}{{if .IsAvailableCommand}}  {{cmd (rpad .Name .NamePadding)}} {{.Short}}
{{end}}{{end}}
{{end}}{{if .HasAvailableLocalFlags}}{{header "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}

{{end}}{{if .HasAvailableInheritedFlags}}{{header "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}

{{end}}{{if .HasAvailableSubCommands}}{{muted "Use"}} {{cmd (printf "%s [command] --help" .CommandPath)}} {{muted "for more information."}}
{{end}}`

// initHelp sets up custom styled help for all commands
func initHelp(cmd *cobra.Command) {
	// Add template functions
	for name, fn := range helpTemplateFuncs {
		cobra.AddTemplateFunc(name, fn)
	}

	// Apply template recursively to all commands
	applyHelpTemplate(cmd)
}

// applyHelpTemplate recursively sets the help template on a command and all subcommands
func applyHelpTemplate(cmd *cobra.Command) {
	cmd.SetHelpTemplate(helpTemplate)
	for _, subCmd := range cmd.Commands() {
		applyHelpTemplate(subCmd)
	}
}
