package recall

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// DebugLogger provides debug logging for Recall operations.
// When enabled, it logs all Engram API communications including
// requests, responses, and full error details.
type DebugLogger struct {
	mu      sync.Mutex
	enabled bool
	writer  io.Writer
}

// NewDebugLogger creates a new debug logger.
// If logPath is empty, logs to stderr.
func NewDebugLogger(enabled bool, logPath string) (*DebugLogger, error) {
	var writer io.Writer = os.Stderr

	if enabled && logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("open debug log: %w", err)
		}
		writer = f
	}

	return &DebugLogger{
		enabled: enabled,
		writer:  writer,
	}, nil
}

// Close closes the debug logger if it's writing to a file.
func (l *DebugLogger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if closer, ok := l.writer.(io.Closer); ok && l.writer != os.Stderr {
		return closer.Close()
	}
	return nil
}

// Log writes a debug message if logging is enabled.
func (l *DebugLogger) Log(format string, args ...any) {
	if l == nil || !l.enabled {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(l.writer, "[%s] [RECALL DEBUG] %s\n", timestamp, msg)
}

// LogRequest logs an outgoing HTTP request.
func (l *DebugLogger) LogRequest(method, url string, body []byte) {
	if l == nil || !l.enabled {
		return
	}
	l.Log("REQUEST %s %s", method, url)
	if len(body) > 0 {
		l.Log("REQUEST BODY: %s", truncateForLog(string(body), 2000))
	}
}

// LogResponse logs an HTTP response.
func (l *DebugLogger) LogResponse(statusCode int, status string, body []byte) {
	if l == nil || !l.enabled {
		return
	}
	l.Log("RESPONSE %d %s", statusCode, status)
	if len(body) > 0 {
		l.Log("RESPONSE BODY: %s", truncateForLog(string(body), 4000))
	}
}

// LogError logs an error with full details.
func (l *DebugLogger) LogError(operation string, err error) {
	if l == nil || !l.enabled {
		return
	}
	l.Log("ERROR [%s]: %v", operation, err)
}

// LogSync logs sync operation details.
func (l *DebugLogger) LogSync(operation string, details string) {
	if l == nil || !l.enabled {
		return
	}
	l.Log("SYNC [%s]: %s", operation, details)
}

// truncateForLog truncates a string for logging purposes.
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + fmt.Sprintf("... [truncated, %d bytes total]", len(s))
}
