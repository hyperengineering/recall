package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
)

// Brand color palette
var (
	// Primary Brand Colors (Neural Green)
	colorPrimary      = lipgloss.Color("#40A967") // Neural Green - main brand
	colorPrimaryLight = lipgloss.Color("#5FC284") // Light Neural Green - highlights
	colorPrimaryDark  = lipgloss.Color("#31804E") // Dark Neural Green - active states

	// Neutral Colors
	colorText  = lipgloss.Color("#F2F3F3") // Mux White - primary text
	colorMuted = lipgloss.Color("240")     // Muted gray for secondary text

	// State Colors
	colorSuccess = lipgloss.Color("#22C55E") // Success green
	colorWarning = lipgloss.Color("#F59E0B") // Warning amber
	colorError   = lipgloss.Color("#EF4444") // Error red
)

// Styles
var (
	successStyle = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	warningStyle = lipgloss.NewStyle().Foreground(colorWarning).Bold(true)
	infoStyle    = lipgloss.NewStyle().Foreground(colorPrimary) // Uses brand color
	mutedStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	labelStyle   = lipgloss.NewStyle().Foreground(colorPrimaryLight).Bold(true)
	valueStyle   = lipgloss.NewStyle().Foreground(colorText)
)

// Icons
const (
	iconSuccess = "✓"
	iconError   = "✗"
	iconWarning = "⚠"
	iconInfo    = "●"
)

// isTTY returns true if stdout is a terminal
func isTTY() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// printStyled prints a message with an icon, applying style only in TTY mode
func printStyled(w io.Writer, icon string, style lipgloss.Style, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if isTTY() {
		fmt.Fprintf(w, "%s %s\n", style.Render(icon), msg)
	} else {
		fmt.Fprintf(w, "%s %s\n", icon, msg)
	}
}

// printSuccess prints a success message with green checkmark
func printSuccess(w io.Writer, format string, args ...interface{}) {
	printStyled(w, iconSuccess, successStyle, format, args...)
}

// printError prints an error message with red X
func printError(w io.Writer, format string, args ...interface{}) {
	printStyled(w, iconError, errorStyle, format, args...)
}

// printWarning prints a warning message with amber warning sign
func printWarning(w io.Writer, format string, args ...interface{}) {
	printStyled(w, iconWarning, warningStyle, format, args...)
}

// printInfo prints an info message with brand-colored dot
func printInfo(w io.Writer, format string, args ...interface{}) {
	printStyled(w, iconInfo, infoStyle, format, args...)
}

// printMuted prints muted/secondary text
func printMuted(w io.Writer, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if isTTY() {
		fmt.Fprintln(w, mutedStyle.Render(msg))
	} else {
		fmt.Fprintln(w, msg)
	}
}

// printLabel prints a styled label
func printLabel(w io.Writer, label string) {
	if isTTY() {
		fmt.Fprint(w, labelStyle.Render(label))
	} else {
		fmt.Fprint(w, label)
	}
}

// renderMarkdown renders markdown content with glamour
func renderMarkdown(content string) string {
	if !isTTY() {
		return content
	}

	// Check if content looks like it has markdown
	if !hasMarkdown(content) {
		return content
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)
	if err != nil {
		return content
	}

	rendered, err := renderer.Render(content)
	if err != nil {
		return content
	}

	return strings.TrimSpace(rendered)
}

// hasMarkdown checks if content contains markdown-like syntax
// Ordered from most specific to least to reduce false positives
func hasMarkdown(content string) bool {
	markers := []string{
		"```",    // code blocks (most specific)
		"## ",    // headers
		"# ",     // headers
		"**",     // bold
		"1. ",    // numbered lists
		"- ",     // list items
		"* ",     // list items
		"](http", // links with URL (more specific than just `](`)
		"`",      // inline code (last - most prone to false positives)
	}
	for _, marker := range markers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}
