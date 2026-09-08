package core

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"gud/internal/profile"
)

func TestResolveProfileContent_EmptyProfile(t *testing.T) {
	t.Parallel()

	if got := resolveProfileContent(""); got != "" {
		t.Errorf("resolveProfileContent('') = %q, want ''", got)
	}
}

func TestResolveProfileContent_NotFound(t *testing.T) {
	orig := profileManager

	t.Cleanup(func() { profileManager = orig })

	profileManager = profile.NewManagerWithDir(t.TempDir())
	if got := resolveProfileContent("nonexistent"); got != "" {
		t.Errorf("resolveProfileContent('nonexistent') = %q, want ''", got)
	}
}

// TestResolveProfileContent_UncachedWarns verifies that a configured but
// uncached profile logs a warning (not just debug) with an actionable hint,
// so hook-mode degradation is surfaced to users instead of hiding silently.
func TestResolveProfileContent_UncachedWarns(t *testing.T) {
	orig := profileManager

	t.Cleanup(func() { profileManager = orig })
	profileManager = profile.NewManagerWithDir(t.TempDir())

	// Capture slog output via a custom default logger.
	var buf bytes.Buffer

	origDefault := slog.Default()
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(origDefault) })

	got := resolveProfileContent("nonexistent")
	if got != "" {
		t.Errorf("resolveProfileContent('nonexistent') = %q, want ''", got)
	}

	out := buf.String()
	if !strings.Contains(out, "level=WARN") {
		t.Errorf("expected WARN level log, got:\n%s", out)
	}

	if !strings.Contains(out, "not cached") ||
		!strings.Contains(out, "git message profile save") ||
		!strings.Contains(out, "nonexistent") {
		t.Errorf("expected warning with profile name and actionable hint, got:\n%s", out)
	}
}

func TestResolveProfileContent_Found(t *testing.T) {
	orig := profileManager

	t.Cleanup(func() { profileManager = orig })

	tmpDir := t.TempDir()

	m := profile.NewManagerWithDir(tmpDir)
	if err := m.Save("test-agent", profile.Profile{
		Content: "You are a test agent.",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	profileManager = m

	got := resolveProfileContent("test-agent")
	if got != "You are a test agent." {
		t.Errorf("resolveProfileContent() = %q, want 'You are a test agent.'", got)
	}
}

func TestAppendDeletedContext(t *testing.T) {
	t.Parallel()

	const diff = "--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new"

	tests := []struct {
		name    string
		diff    string
		deleted string
		want    string
	}{
		{
			name:    "no deleted files returns diff unchanged",
			diff:    diff,
			deleted: "",
			want:    diff,
		},
		{
			name:    "whitespace-only deleted returns diff unchanged",
			diff:    diff,
			deleted: "  \n\t\n  ",
			want:    diff,
		},
		{
			name:    "single deleted file appended",
			diff:    "diff --git a/old.go b/new.go",
			deleted: "old.go\n",
			want:    "diff --git a/old.go b/new.go\n\nDeleted files:\nold.go\n",
		},
		{
			name:    "multiple deleted files each on own line",
			diff:    "--- a/a.go\n+++ b/b.go",
			deleted: "a.go\nb.go\n",
			want:    "--- a/a.go\n+++ b/b.go\n\nDeleted files:\na.go\nb.go\n",
		},
		{
			name:    "filenames with whitespace are trimmed",
			diff:    "diff:",
			deleted: "  file.go  \n\tcmd/main.go\n",
			want:    "diff:\n\nDeleted files:\nfile.go\ncmd/main.go\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := appendDeletedContext(tt.diff, tt.deleted)
			if got != tt.want {
				t.Errorf("appendDeletedContext():\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}

// TestJoinContexts verifies the join used to assemble the final prompt
// context from staged-diff and repo/history context sections.
func TestJoinContexts(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want string
	}{
		{
			name: "both empty yields empty",
			a:    "",
			b:    "",
			want: "",
		},
		{
			name: "empty a returns b",
			a:    "",
			b:    "repo context",
			want: "repo context",
		},
		{
			name: "empty b returns a",
			a:    "staged diff",
			b:    "",
			want: "staged diff",
		},
		{
			name: "both non-empty joined by blank line",
			a:    "staged diff",
			b:    "repo context",
			want: "staged diff\n\nrepo context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := joinContexts(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("joinContexts():\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}
