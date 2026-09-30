package core

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
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
		!strings.Contains(out, "git message persona save") ||
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

// TestComposePromptDiff verifies default exclusion of removed content: the
// composed prompt keeps added/modified hunks, drops deleted-file removed
// lines and rename hunks, and still names the removed paths. Opt-in restores
// the unfiltered diff.
func TestComposePromptDiff(t *testing.T) {
	t.Parallel()

	modifiedBlock := "diff --git a/keep.go b/keep.go\n" +
		"index 111..222 100644\n" +
		"--- a/keep.go\n" +
		"+++ b/keep.go\n" +
		"@@ -1 +1 @@\n" +
		"-a\n" +
		"+b\n"
	raw := modifiedBlock +
		"diff --git a/gone.go b/gone.go\n" +
		"deleted file mode 100644\n" +
		"index 333..000\n" +
		"--- a/gone.go\n" +
		"+++ /dev/null\n" +
		"-doomed content\n" +
		"diff --git a/old.txt b/new.txt\n" +
		"similarity index 100%\n" +
		"rename from old.txt\n" +
		"rename to new.txt\n"

	t.Run("default excludes removed content but keeps names", func(t *testing.T) {
		t.Parallel()

		got := composePromptDiff(raw, false)
		if !strings.Contains(got, "+b\n") {
			t.Errorf("composed prompt missing modified hunks:\n%s", got)
		}

		if strings.Contains(got, "doomed content") {
			t.Errorf("composed prompt leaks deleted content:\n%s", got)
		}

		if strings.Contains(got, "rename from old.txt") {
			t.Errorf("composed prompt leaks rename hunks:\n%s", got)
		}

		if !strings.Contains(got, "gone.go") {
			t.Errorf("composed prompt missing deleted name:\n%s", got)
		}

		if !strings.Contains(got, "old.txt -> new.txt") {
			t.Errorf("composed prompt missing rename pair:\n%s", got)
		}
	})

	t.Run("opt-in restores full content and keeps names", func(t *testing.T) {
		t.Parallel()

		want := raw + "\n\nDeleted files:\ngone.go\n\n\nRenamed files:\nold.txt -> new.txt\n"
		if got := composePromptDiff(raw, true); got != want {
			t.Errorf("composePromptDiff(include=true):\n got: %q\nwant: %q", got, want)
		}
	})

	t.Run("names-only input stays non-empty", func(t *testing.T) {
		t.Parallel()

		onlyDeleted := "diff --git a/gone.go b/gone.go\n" +
			"deleted file mode 100644\n" +
			"index 333..000\n" +
			"--- a/gone.go\n" +
			"+++ /dev/null\n" +
			"-x\n"
		if got := composePromptDiff(onlyDeleted, false); strings.TrimSpace(got) == "" {
			t.Error("names-only prompt must be non-empty so generation proceeds from names")
		}
	})
}

// newPromptDiffTestRepo builds a throwaway repo with one commit and chdirs
// into it. Tests using it skip in short mode like the other repo tests.
func newPromptDiffTestRepo(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		//nolint:gosec // test-only git invocation with fixed repo-local args
		cmd := exec.CommandContext(context.Background(), "git", args...)

		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")

	if err := os.WriteFile(dir+"/keep.go", []byte("package main\n"), 0600); err != nil {
		t.Fatalf("write keep.go: %v", err)
	}

	if err := os.WriteFile(dir+"/doomed.go", []byte("package doomed\n"), 0600); err != nil {
		t.Fatalf("write doomed.go: %v", err)
	}

	if err := os.WriteFile(dir+"/moved.txt", []byte("identical content for rename detection\n"), 0600); err != nil {
		t.Fatalf("write moved.txt: %v", err)
	}

	run("add", ".")
	run("commit", "-q", "-m", "init")

	t.Chdir(dir)
}

// TestGetStagedDiffOrError_ExcludesRemovedContent stages a modification, a
// deletion, and a pure rename, then verifies the prompt diff excludes removed
// lines while naming the removed paths.
func TestGetStagedDiffOrError_ExcludesRemovedContent(t *testing.T) {
	newPromptDiffTestRepo(t)

	run := func(args ...string) {
		t.Helper()
		//nolint:gosec // test-only git invocation with fixed repo-local args
		cmd := exec.CommandContext(context.Background(), "git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	if err := os.WriteFile("keep.go", []byte("package main\n\n// changed\n"), 0600); err != nil {
		t.Fatalf("write keep.go: %v", err)
	}

	run("add", "keep.go")
	run("rm", "-q", "doomed.go")
	run("mv", "moved.txt", "renamed.txt")

	got, err := getStagedDiffOrError(context.Background(), false)
	if err != nil {
		t.Fatalf("getStagedDiffOrError() error = %v", err)
	}

	if !strings.Contains(got, "// changed") {
		t.Errorf("prompt missing modified hunks:\n%s", got)
	}

	if strings.Contains(got, "package doomed") {
		t.Errorf("prompt leaks deleted content:\n%s", got)
	}

	if !strings.Contains(got, "doomed.go") {
		t.Errorf("prompt missing deleted name:\n%s", got)
	}

	if !strings.Contains(got, "moved.txt -> renamed.txt") {
		t.Errorf("prompt missing rename pair:\n%s", got)
	}
}

// TestGetStagedDiffOrError_NamesOnlyProceeds verifies a deletion-only stage
// does not take the "no staged changes" error path: names are valid prompt
// content.
func TestGetStagedDiffOrError_NamesOnlyProceeds(t *testing.T) {
	newPromptDiffTestRepo(t)

	run := func(args ...string) {
		t.Helper()
		//nolint:gosec // test-only git invocation with fixed repo-local args
		cmd := exec.CommandContext(context.Background(), "git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("rm", "-q", "doomed.go")

	got, err := getStagedDiffOrError(context.Background(), false)
	if err != nil {
		t.Fatalf("names-only stage must not error, got: %v", err)
	}

	if !strings.Contains(got, "doomed.go") {
		t.Errorf("names-only prompt missing deleted name:\n%s", got)
	}

	if strings.Contains(got, "package doomed") {
		t.Errorf("names-only prompt leaks deleted content:\n%s", got)
	}
}
