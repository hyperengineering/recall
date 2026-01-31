package main

import (
	"strings"
	"testing"
)

// setMockTTY sets the TTY override for tests and returns a cleanup function.
// The cleanup function restores the TTY override to nil, allowing real TTY detection.
func setMockTTY(value bool) func() {
	testIsTTYMutex.Lock()
	testIsTTYOverride = &value
	testIsTTYMutex.Unlock()
	return func() {
		testIsTTYMutex.Lock()
		testIsTTYOverride = nil
		testIsTTYMutex.Unlock()
	}
}

// ============================================================================
// Task 1: Table rendering tests
// ============================================================================

func TestRenderTable_TTY_WithHeaders(t *testing.T) {
	cleanup := setMockTTY(true)
	defer cleanup()

	headers := []string{"NAME", "COUNT", "STATUS"}
	rows := [][]string{
		{"alpha", "10", "active"},
		{"beta", "20", "inactive"},
	}

	result := renderTable(headers, rows)

	// Should contain all headers
	if !strings.Contains(result, "NAME") {
		t.Error("result should contain NAME header")
	}
	if !strings.Contains(result, "COUNT") {
		t.Error("result should contain COUNT header")
	}
	if !strings.Contains(result, "STATUS") {
		t.Error("result should contain STATUS header")
	}

	// Should contain all data
	if !strings.Contains(result, "alpha") {
		t.Error("result should contain 'alpha'")
	}
	if !strings.Contains(result, "beta") {
		t.Error("result should contain 'beta'")
	}

	// TTY mode should have styled output (contains box-drawing characters)
	if !strings.ContainsAny(result, "─│╭╮╰╯├┼┤┬┴") {
		t.Error("TTY output should contain border characters")
	}
}

func TestRenderTable_NonTTY_PlainText(t *testing.T) {
	cleanup := setMockTTY(false)
	defer cleanup()

	headers := []string{"NAME", "COUNT"}
	rows := [][]string{
		{"alpha", "10"},
		{"beta", "20"},
	}

	result := renderTable(headers, rows)

	// Should contain all headers and data
	if !strings.Contains(result, "NAME") {
		t.Error("result should contain NAME header")
	}
	if !strings.Contains(result, "alpha") {
		t.Error("result should contain 'alpha'")
	}

	// Non-TTY should NOT have box-drawing characters
	if strings.ContainsAny(result, "─│╭╮╰╯├┼┤┬┴") {
		t.Error("non-TTY output should NOT contain border characters")
	}
}

func TestRenderTable_EmptyRows(t *testing.T) {
	cleanup := setMockTTY(true)
	defer cleanup()

	headers := []string{"NAME", "COUNT"}
	var rows [][]string

	result := renderTable(headers, rows)

	// Should contain headers without crashing
	if !strings.Contains(result, "NAME") {
		t.Error("result should contain header even with empty rows")
	}
}

func TestRenderTable_LongContent(t *testing.T) {
	cleanup := setMockTTY(true)
	defer cleanup()

	longContent := strings.Repeat("x", 100)
	headers := []string{"NAME"}
	rows := [][]string{{longContent}}

	result := renderTable(headers, rows)

	// Should handle long content without crashing
	if !strings.Contains(result, "NAME") {
		t.Error("result should contain header")
	}
	// Content should be present (may be truncated or wrapped)
	if !strings.Contains(result, "x") {
		t.Error("result should contain content")
	}
}

func TestRenderTable_EmptyHeaders(t *testing.T) {
	cleanup := setMockTTY(true)
	defer cleanup()

	var headers []string
	rows := [][]string{{"data1", "data2"}}

	// Should not panic and should return empty string for empty headers
	result := renderTable(headers, rows)

	// Empty headers should return empty string
	if result != "" {
		t.Error("empty headers should return empty string")
	}
}

func TestRenderTable_RowsLongerThanHeaders(t *testing.T) {
	cleanup := setMockTTY(true)
	defer cleanup()

	headers := []string{"COL1"}
	rows := [][]string{{"a", "b", "c"}} // More columns than headers

	// Should not panic, extra columns ignored
	result := renderTable(headers, rows)

	if !strings.Contains(result, "COL1") {
		t.Error("should contain header")
	}
	if !strings.Contains(result, "a") {
		t.Error("should contain first cell")
	}
}

// ============================================================================
// Task 1: Panel rendering tests
// ============================================================================

func TestRenderPanel_TTY_WithTitle(t *testing.T) {
	cleanup := setMockTTY(true)
	defer cleanup()

	result := renderPanel("Statistics", "Lore count: 42\nPending: 5")

	// Should contain title
	if !strings.Contains(result, "Statistics") {
		t.Error("result should contain title")
	}

	// Should contain content
	if !strings.Contains(result, "Lore count: 42") {
		t.Error("result should contain content")
	}

	// TTY mode should have border
	if !strings.ContainsAny(result, "─│╭╮╰╯") {
		t.Error("TTY panel should have border characters")
	}
}

func TestRenderPanel_TTY_NoTitle(t *testing.T) {
	cleanup := setMockTTY(true)
	defer cleanup()

	result := renderPanel("", "Just content here")

	if !strings.Contains(result, "Just content here") {
		t.Error("result should contain content")
	}
}

func TestRenderPanel_NonTTY_PlainText(t *testing.T) {
	cleanup := setMockTTY(false)
	defer cleanup()

	result := renderPanel("Statistics", "Lore count: 42")

	// Should contain title and content
	if !strings.Contains(result, "Statistics") {
		t.Error("result should contain title")
	}
	if !strings.Contains(result, "Lore count: 42") {
		t.Error("result should contain content")
	}

	// Non-TTY should NOT have border characters
	if strings.ContainsAny(result, "─│╭╮╰╯") {
		t.Error("non-TTY panel should NOT have border characters")
	}
}

func TestRenderPanel_LongContent(t *testing.T) {
	cleanup := setMockTTY(true)
	defer cleanup()

	longContent := strings.Repeat("line\n", 50)
	result := renderPanel("Long Panel", longContent)

	// Should handle long content without crashing
	if !strings.Contains(result, "Long Panel") {
		t.Error("result should contain title")
	}
}

// ============================================================================
// Task 5: Error panel tests
// ============================================================================

func TestRenderErrorPanel_AllSections(t *testing.T) {
	cleanup := setMockTTY(true)
	defer cleanup()

	result := renderErrorPanel(
		"Invalid store ID",
		"contains uppercase characters",
		"Use lowercase alphanumeric with hyphens",
	)

	// Should contain error message
	if !strings.Contains(result, "Invalid store ID") {
		t.Error("result should contain error message")
	}

	// Should contain context
	if !strings.Contains(result, "uppercase") {
		t.Error("result should contain context")
	}

	// Should contain suggestion
	if !strings.Contains(result, "lowercase") {
		t.Error("result should contain suggestion")
	}

	// TTY mode should have border
	if !strings.ContainsAny(result, "─│╭╮╰╯") {
		t.Error("TTY error panel should have border")
	}
}

func TestRenderErrorPanel_ErrorOnly(t *testing.T) {
	cleanup := setMockTTY(true)
	defer cleanup()

	result := renderErrorPanel("Something went wrong", "", "")

	if !strings.Contains(result, "Something went wrong") {
		t.Error("result should contain error message")
	}
}

func TestRenderErrorPanel_NonTTY(t *testing.T) {
	cleanup := setMockTTY(false)
	defer cleanup()

	result := renderErrorPanel(
		"Error message",
		"Some context",
		"Try this instead",
	)

	// Should contain all parts
	if !strings.Contains(result, "Error message") {
		t.Error("result should contain error")
	}
	if !strings.Contains(result, "Context:") {
		t.Error("result should contain context label")
	}
	if !strings.Contains(result, "Suggestion:") {
		t.Error("result should contain suggestion label")
	}

	// Non-TTY should NOT have border characters
	if strings.ContainsAny(result, "─│╭╮╰╯") {
		t.Error("non-TTY error panel should NOT have borders")
	}
}

// ============================================================================
// Task 6: Confirmation prompt tests
// ============================================================================

func TestRenderConfirmation_TTY(t *testing.T) {
	cleanup := setMockTTY(true)
	defer cleanup()

	result := renderConfirmation(
		"This will delete all data",
		"Type 'yes' to confirm:",
	)

	// Should contain warning
	if !strings.Contains(result, "delete all data") {
		t.Error("result should contain warning")
	}

	// Should contain prompt
	if !strings.Contains(result, "Type 'yes'") {
		t.Error("result should contain prompt")
	}

	// TTY mode should have separator
	if !strings.Contains(result, "─") {
		t.Error("TTY confirmation should have separator line")
	}
}

func TestRenderConfirmation_NonTTY(t *testing.T) {
	cleanup := setMockTTY(false)
	defer cleanup()

	result := renderConfirmation(
		"Warning message",
		"Confirm:",
	)

	if !strings.Contains(result, "Warning message") {
		t.Error("result should contain warning")
	}
	if !strings.Contains(result, "Confirm:") {
		t.Error("result should contain prompt")
	}
}

// ============================================================================
// Integration tests for TTY/non-TTY modes
// ============================================================================

func TestStoreListOutput_TTY_HasBorders(t *testing.T) {
	cleanup := setMockTTY(true)
	defer cleanup()

	headers := []string{"STORE ID", "DESCRIPTION", "LORE COUNT", "UPDATED"}
	rows := [][]string{
		{"default", "Default store", "10", "1h ago"},
	}

	result := renderTable(headers, rows)

	// TTY mode should have box-drawing borders
	if !strings.Contains(result, "╭") || !strings.Contains(result, "╯") {
		t.Error("TTY table output should have rounded borders")
	}
	if !strings.Contains(result, "STORE ID") {
		t.Error("TTY table should contain headers")
	}
}

func TestStoreListOutput_NonTTY_NoBorders(t *testing.T) {
	cleanup := setMockTTY(false)
	defer cleanup()

	headers := []string{"STORE ID", "DESCRIPTION", "LORE COUNT", "UPDATED"}
	rows := [][]string{
		{"default", "Default store", "10", "1h ago"},
	}

	result := renderTable(headers, rows)

	// Non-TTY mode should NOT have box-drawing borders
	if strings.Contains(result, "╭") || strings.Contains(result, "│") {
		t.Error("non-TTY table output should NOT have borders")
	}
	if !strings.Contains(result, "STORE ID") {
		t.Error("non-TTY table should still contain headers")
	}
}

func TestStatsPanel_TTY_HasBorder(t *testing.T) {
	cleanup := setMockTTY(true)
	defer cleanup()

	content := "Lore count: 42\nPending sync: 5"
	result := renderPanel("Local Store Statistics", content)

	if !strings.Contains(result, "╭") {
		t.Error("TTY panel should have border")
	}
	if !strings.Contains(result, "Local Store Statistics") {
		t.Error("TTY panel should have title")
	}
}

func TestStatsPanel_NonTTY_NoBorder(t *testing.T) {
	cleanup := setMockTTY(false)
	defer cleanup()

	content := "Lore count: 42\nPending sync: 5"
	result := renderPanel("Local Store Statistics", content)

	if strings.Contains(result, "╭") {
		t.Error("non-TTY panel should NOT have border")
	}
	if !strings.Contains(result, "Local Store Statistics") {
		t.Error("non-TTY panel should have title")
	}
	if !strings.Contains(result, "Lore count: 42") {
		t.Error("non-TTY panel should have content")
	}
}
