package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
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
	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)
	labelStyle = lipgloss.NewStyle().Foreground(colorPrimaryLight).Bold(true)
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
	if isTTYCheck() {
		_, _ = fmt.Fprintf(w, "%s %s\n", style.Render(icon), msg)
	} else {
		_, _ = fmt.Fprintf(w, "%s %s\n", icon, msg)
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
	if isTTYCheck() {
		_, _ = fmt.Fprintln(w, mutedStyle.Render(msg))
	} else {
		_, _ = fmt.Fprintln(w, msg)
	}
}

// renderMarkdown renders markdown content with glamour
func renderMarkdown(content string) string {
	if !isTTYCheck() {
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

// testIsTTYOverride allows tests to override TTY detection
var (
	testIsTTYOverride *bool
	testIsTTYMutex    sync.RWMutex
)

// isTTYCheck returns TTY status, allowing test override
func isTTYCheck() bool {
	testIsTTYMutex.RLock()
	override := testIsTTYOverride
	testIsTTYMutex.RUnlock()
	if override != nil {
		return *override
	}
	return isTTY()
}

// ============================================================================
// Table Styles (Task 1)
// ============================================================================

var (
	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(colorPrimaryLight).
				Bold(true)

	tableCellStyle = lipgloss.NewStyle().
			Foreground(colorText)

	// tableCellMutedStyle is available for muted table cells (e.g., timestamps, secondary data).
	tableCellMutedStyle = lipgloss.NewStyle().
				Foreground(colorMuted)
)

// ============================================================================
// Panel Styles (Task 1)
// ============================================================================

var (
	panelBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorMuted).
				Padding(0, 1)

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(colorPrimaryLight).
			Bold(true)

	errorPanelBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorError).
				Padding(0, 1)

	// warningPanelBorderStyle is available for warning panels (not currently used).
	warningPanelBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorWarning).
				Padding(0, 1)
)

// ============================================================================
// Confirmation Styles (Task 6)
// ============================================================================

var (
	confirmSeparatorStyle = lipgloss.NewStyle().
				Foreground(colorWarning).
				Bold(true)
)

// ============================================================================
// Helper Functions
// ============================================================================

// renderTable renders a table with styled headers.
// Returns plain text in non-TTY mode.
func renderTable(headers []string, rows [][]string) string {
	if !isTTYCheck() {
		return renderPlainTable(headers, rows)
	}

	// Handle empty headers edge case
	if len(headers) == 0 {
		return ""
	}

	// Build table using lipgloss
	var sb strings.Builder

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = runewidth.StringWidth(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && runewidth.StringWidth(cell) > widths[i] {
				widths[i] = runewidth.StringWidth(cell)
			}
		}
	}

	// Build border strings
	topBorder := "╭"
	midBorder := "├"
	botBorder := "╰"
	for i, w := range widths {
		seg := strings.Repeat("─", w+2)
		if i < len(widths)-1 {
			topBorder += seg + "┬"
			midBorder += seg + "┼"
			botBorder += seg + "┴"
		} else {
			topBorder += seg + "╮"
			midBorder += seg + "┤"
			botBorder += seg + "╯"
		}
	}

	borderStyle := lipgloss.NewStyle().Foreground(colorMuted)

	// Top border
	sb.WriteString(borderStyle.Render(topBorder))
	sb.WriteString("\n")

	// Header row
	sb.WriteString(borderStyle.Render("│"))
	for i, h := range headers {
		// Pad the header text manually, then style it
		padded := fmt.Sprintf(" %-*s ", widths[i], h)
		sb.WriteString(tableHeaderStyle.Render(padded))
		sb.WriteString(borderStyle.Render("│"))
	}
	sb.WriteString("\n")

	// Middle border
	sb.WriteString(borderStyle.Render(midBorder))
	sb.WriteString("\n")

	// Data rows
	for _, row := range rows {
		sb.WriteString(borderStyle.Render("│"))
		for i := 0; i < len(headers); i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			// Pad the cell text manually, then style it
			padded := fmt.Sprintf(" %-*s ", widths[i], cell)
			sb.WriteString(tableCellStyle.Render(padded))
			sb.WriteString(borderStyle.Render("│"))
		}
		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(borderStyle.Render(botBorder))
	sb.WriteString("\n")

	return sb.String()
}

// renderPlainTable renders a table without styling for non-TTY
func renderPlainTable(headers []string, rows [][]string) string {
	// Handle empty headers edge case
	if len(headers) == 0 {
		return ""
	}

	var sb strings.Builder

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = runewidth.StringWidth(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && runewidth.StringWidth(cell) > widths[i] {
				widths[i] = runewidth.StringWidth(cell)
			}
		}
	}

	// Format string
	fmtParts := make([]string, len(widths))
	for i, w := range widths {
		fmtParts[i] = fmt.Sprintf("%%-%ds", w+2)
	}
	format := strings.Join(fmtParts, "") + "\n"

	// Render header
	headerArgs := make([]interface{}, len(headers))
	for i, h := range headers {
		headerArgs[i] = h
	}
	sb.WriteString(fmt.Sprintf(format, headerArgs...))

	// Render rows
	for _, row := range rows {
		rowArgs := make([]interface{}, len(headers))
		for i := 0; i < len(headers); i++ {
			if i < len(row) {
				rowArgs[i] = row[i]
			} else {
				rowArgs[i] = ""
			}
		}
		sb.WriteString(fmt.Sprintf(format, rowArgs...))
	}

	return sb.String()
}

// renderPanel renders content in a bordered panel.
// Returns plain text with title prefix in non-TTY mode.
func renderPanel(title, content string) string {
	if !isTTYCheck() {
		if title != "" {
			return fmt.Sprintf("%s\n%s", title, content)
		}
		return content
	}

	var sb strings.Builder
	if title != "" {
		sb.WriteString(panelTitleStyle.Render(title))
		sb.WriteString("\n")
	}
	sb.WriteString(panelBorderStyle.MaxWidth(80).Render(content))
	return sb.String()
}

// renderErrorPanel renders a structured error with context and suggestion.
func renderErrorPanel(errMsg, context, suggestion string) string {
	if !isTTYCheck() {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%s %s\n", iconError, errMsg))
		if context != "" {
			sb.WriteString(fmt.Sprintf("  Context: %s\n", context))
		}
		if suggestion != "" {
			sb.WriteString(fmt.Sprintf("  Suggestion: %s\n", suggestion))
		}
		return sb.String()
	}

	var lines []string
	lines = append(lines, errorStyle.Render(iconError+" "+errMsg))
	if context != "" {
		lines = append(lines, mutedStyle.Render("  "+context))
	}
	if suggestion != "" {
		lines = append(lines, infoStyle.Render("  → "+suggestion))
	}

	content := strings.Join(lines, "\n")
	return errorPanelBorderStyle.MaxWidth(80).Render(content)
}

// Confirmation separator width bounds
const (
	minSeparatorWidth = 40
	maxSeparatorWidth = 60
)

// renderConfirmation renders a confirmation prompt with visual separation.
func renderConfirmation(warning, prompt string) string {
	if !isTTYCheck() {
		return fmt.Sprintf("%s\n%s", warning, prompt)
	}

	// Calculate separator width based on warning length
	sepWidth := len(warning)
	if sepWidth < minSeparatorWidth {
		sepWidth = minSeparatorWidth
	}
	if sepWidth > maxSeparatorWidth {
		sepWidth = maxSeparatorWidth
	}
	separator := strings.Repeat("─", sepWidth)
	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		confirmSeparatorStyle.Render(separator),
		warningStyle.Render(iconWarning+" "+warning),
		"",
		prompt,
	)
}
