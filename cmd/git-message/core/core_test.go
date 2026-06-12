package core

import (
	"bufio"
	"context"
	"strings"
	"testing"
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
		cfg  Config
	}{
		{name: "zero history", cfg: Config{History: 0}},
		{name: "negative history", cfg: Config{History: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildHistoryContext(context.Background(), tt.cfg)
			if got != "" {
				t.Errorf("buildHistoryContext() = %q, want empty string", got)
			}
		})
	}
}

func TestAppendDeletedContext(t *testing.T) {
	t.Parallel()

	sampleDiff := "diff --git a/foo.go b/foo.go" +
		"\nindex abc..def 100644\n--- a/foo.go\n+++ b/foo.go" +
		"\n@@ -1 +1 @@\n-package old\n+package new\n"

	keepDiff := "diff --git a/keep.go b/keep.go" +
		"\nindex abc..def 100644\n--- a/keep.go\n+++ b/keep.go" +
		"\n@@ -1 +1 @@\n-package old\n+package new\n"

	tests := []struct {
		name    string
		diff    string
		deleted string
		want    string
	}{
		{
			name:    "no deleted files returns diff unchanged",
			diff:    sampleDiff,
			deleted: "",
			want:    sampleDiff,
		},
		{
			name:    "deleted files appended as section",
			diff:    keepDiff,
			deleted: "file.go\nold.go\n",
			want: keepDiff +
				"\n\nDeleted files:\nfile.go\nold.go\n",
		},
		{
			name:    "whitespace-only deleted returns diff unchanged",
			diff:    sampleDiff,
			deleted: "  \n\t\n  ",
			want:    sampleDiff,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := appendDeletedContext(tt.diff, tt.deleted)
			if got != tt.want {
				t.Errorf("appendDeletedContext() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
