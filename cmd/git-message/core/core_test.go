// Package core provides the CLI command structure and workflow orchestration for git-message.
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

const testModelName = "claude-3"
const testWantAssistedBy = "feat: add foo\n\nAssisted-by: claude-3\n"

func TestAppendAssistedBy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		msg       string
		modelName string
		want      string
	}{
		{
			name:      "appends trailer to plain message",
			msg:       "feat: add foo",
			modelName: testModelName,
			want:      testWantAssistedBy,
		},
		{
			name:      "appends trailer to message with body",
			msg:       "feat: add foo\n\nThis is the body.",
			modelName: testModelName,
			want:      "feat: add foo\n\nThis is the body.\n\nAssisted-by: claude-3\n",
		},
		{
			name:      "trims trailing newlines before appending",
			msg:       "feat: add foo\n\n",
			modelName: "gpt-4",
			want:      "feat: add foo\n\nAssisted-by: gpt-4\n",
		},
		{
			name:      "trims multiple trailing newlines",
			msg:       "feat: add foo\n\n\n\n",
			modelName: "gpt-4",
			want:      "feat: add foo\n\nAssisted-by: gpt-4\n",
		},
		{
			name:      "idempotent — already has trailer",
			msg:       "feat: add foo\n\nAssisted-by: claude-3",
			modelName: testModelName,
			want:      testWantAssistedBy,
		},
		{
			name:      "idempotent — already has trailer with trailing newline",
			msg:       testWantAssistedBy,
			modelName: testModelName,
			want:      testWantAssistedBy,
		},
		{
			name:      "idempotent — different model appends new trailer",
			msg:       "feat: add foo\n\nAssisted-by: old-model\n",
			modelName: "new-model",
			// TrimRight removes the trailing \n, then \n\n is added before the new trailer.
			// Result: one blank line between old-model and the new trailer.
			want: "feat: add foo\n\nAssisted-by: old-model\n\nAssisted-by: new-model\n",
		},
		{
			name:      "message has other trailer",
			msg:       "feat: add foo\n\nSigned-off-by: Alice <alice@example.com>",
			modelName: testModelName,
			// The Signed-off-by line has no trailing newline, so TrimRight is a no-op.
			// \n\n is added before the new trailer.
			want: "feat: add foo\n\nSigned-off-by: Alice <alice@example.com>\n\nAssisted-by: claude-3\n",
		},
		{
			name:      "empty message",
			msg:       "",
			modelName: testModelName,
			want:      "\n\nAssisted-by: claude-3\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := appendAssistedBy(tt.msg, tt.modelName)
			if got != tt.want {
				t.Errorf("appendAssistedBy() = %q, want %q", got, tt.want)
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
		{
			name:    "malicious filename with embedded newlines is sanitized",
			diff:    sampleDiff,
			deleted: "clean.go\n\ninjected line\n\nanother.go\n",
			want: sampleDiff +
				"\n\nDeleted files:\nclean.go\ninjected line\nanother.go\n",
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
