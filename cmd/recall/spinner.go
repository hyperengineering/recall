package main

import (
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Spinner configuration constants
const (
	spinnerFrameWidth = 2                      // Unicode braille characters render ~2 columns
	spinnerAnimDelay  = 80 * time.Millisecond  // Animation frame delay
	spinnerClearPad   = 5                      // Extra clearance for terminal variations
)

// simpleSpinner is a lightweight spinner for async operations.
// It uses a goroutine with atomic bool for thread-safe stop signaling.
type simpleSpinner struct {
	frames   []string
	current  int
	message  string
	done     atomic.Bool
	w        io.Writer
	clearLen int // length needed to clear the line
}

func newSimpleSpinner(w io.Writer, message string) *simpleSpinner {
	return &simpleSpinner{
		frames:  []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		message: message,
		w:       w,
		// Calculate clear length: spinner width + space + message
		clearLen: spinnerFrameWidth + 1 + len(message),
	}
}

func (s *simpleSpinner) Start() {
	if !isTTY() {
		fmt.Fprintf(s.w, "%s...\n", s.message)
		return
	}

	go func() {
		spinnerStyle := lipgloss.NewStyle().Foreground(colorPrimary)
		for !s.done.Load() {
			frame := s.frames[s.current%len(s.frames)]
			fmt.Fprintf(s.w, "\r%s %s", spinnerStyle.Render(frame), s.message)
			s.current++
			time.Sleep(spinnerAnimDelay)
		}
	}()
}

func (s *simpleSpinner) Stop() {
	s.done.Store(true)
	if isTTY() {
		// Clear the spinner line using calculated length plus safety margin
		clearStr := "\r" + strings.Repeat(" ", s.clearLen+spinnerClearPad) + "\r"
		fmt.Fprint(s.w, clearStr)
	}
}

// runWithSpinner runs an operation with a spinner, showing progress.
// The spinner animates while the operation runs and clears when complete.
func runWithSpinner(w io.Writer, message string, operation func() error) error {
	spin := newSimpleSpinner(w, message)
	spin.Start()
	err := operation()
	spin.Stop()
	return err
}
