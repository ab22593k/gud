// Package core provides the CLI command structure and workflow orchestration for git-message.
package core

import (
	"bufio"
	"context"
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
		cfg  config.Config
	}{
		{name: "zero history", cfg: config.Config{History: 0}},
		{name: "negative history", cfg: config.Config{History: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			got := buildHistoryContext(ctx, tt.cfg)
			if got != "" {
				t.Errorf("buildHistoryContext(%+v) = %q, want empty string", tt.cfg, got)
			}
		})
	}
}

// discardWriter is an io.Writer that discards all writes.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
