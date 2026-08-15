// Package core provides the CLI command structure and workflow orchestration for git-message.
package core

import (
	"bufio"
	"context"
	"log/slog"
	"strings"
	"testing"

	"gud/internal/config"
)

func TestPromptAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty defaults to commit", input: "\n", want: actionCommit},
		{name: "y maps to commit", input: "y\n", want: actionCommit},
		{name: "yes maps to commit", input: "yes\n", want: actionCommit},
		{name: "commit maps to commit", input: "commit\n", want: actionCommit},
		{name: "c maps to commit", input: "c\n", want: actionCommit},
		{name: "r maps to regenerate", input: "r\n", want: actionRegenerate},
		{name: "regenerate maps to regenerate", input: "regenerate\n", want: actionRegenerate},
		{name: "e maps to edit", input: "e\n", want: actionEdit},
		{name: "edit maps to edit", input: "edit\n", want: actionEdit},
		{name: "a maps to abort", input: "a\n", want: actionAbort},
		{name: "abort maps to abort", input: "abort\n", want: actionAbort},
		{name: "q maps to abort", input: "q\n", want: actionAbort},
		{name: "quit maps to abort", input: "quit\n", want: actionAbort},
		{name: "EOF returns abort", input: "", want: actionAbort},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			got := promptAction(scanner, discardWriter{})
			if got != tt.want {
				t.Errorf("promptAction(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildHistoryContext_Disabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		app  *AppContext
	}{
		// Explicit History=0 disables history context.
		{name: "zero history", app: &AppContext{cfg: config.Config{History: config.Ptr(0)}}},
		{name: "negative history", app: &AppContext{cfg: config.Config{History: config.Ptr(-1)}}},
		// Unset History (nil) also disables history context.
		{name: "unset history", app: &AppContext{cfg: config.Config{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			got := buildHistoryContext(ctx, tt.app, "")
			if got != "" {
				t.Errorf("buildHistoryContext(%+v) = %q, want empty string", tt.app.Config(), got)
			}
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  slog.Level
	}{
		{name: "debug", input: "debug", want: slog.LevelDebug},
		{name: "debug uppercase", input: "DEBUG", want: slog.LevelDebug},
		{name: "debug padded", input: " debug ", want: slog.LevelDebug},
		{name: "info", input: "info", want: slog.LevelInfo},
		{name: "warn", input: "warn", want: slog.LevelWarn},
		{name: "error", input: "error", want: slog.LevelError},
		{name: "empty defaults to info", input: "", want: slog.LevelInfo},
		{name: "unknown defaults to info", input: "trace", want: slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := parseLogLevel(tt.input); got != tt.want {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// discardWriter is an io.Writer that discards all writes.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
