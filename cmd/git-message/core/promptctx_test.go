package core

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gud/internal/git"
)

// TestRepositoryStateDiagnostics pins the D8 diagnostics contract (FR-030,
// SC-011): in a fresh unborn repository with a staged file, building the
// prompt context emits exactly one
// "commit-history context skipped: repository has no commits" record with
// reason=unborn branch, no "failed to get recent commits" record, and the
// returned context contains both the first-commit statement and the
// repository-overview fragment.
//
// Non-parallel: chdirs and replaces the process-wide default slog logger.
func TestRepositoryStateDiagnostics(t *testing.T) {
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

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0600); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}

	run("add", "a.txt")

	t.Chdir(dir)

	var buf bytes.Buffer

	prev := slog.Default()

	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	app := &AppContext{}

	got := buildPromptContextParallel(context.Background(), app, "diff --git a/a.txt b/a.txt", git.OperationNone)

	if !strings.Contains(got, firstCommitStatement) {
		t.Errorf("prompt context missing the first-commit statement, got:\n%s", got)
	}

	if !strings.Contains(got, "Repository:") {
		t.Errorf("prompt context missing the repository-overview fragment, got:\n%s", got)
	}

	out := buf.String()
	if n := strings.Count(out, "commit-history context skipped: repository has no commits"); n != 1 {
		t.Errorf("skip record count = %d, want exactly 1:\n%s", n, out)
	}

	// The slog text handler quotes the value (reason="unborn branch"), so
	// match the two halves rather than the raw key=value spelling.
	if !strings.Contains(out, "reason=") || !strings.Contains(out, "unborn branch") {
		t.Errorf("skip record missing reason=unborn branch:\n%s", out)
	}

	if strings.Contains(out, "failed to get recent commits") {
		t.Errorf("logged a failed history lookup, want the skip instead:\n%s", out)
	}
}

// TestUnbornRepoSkipsHistoryWithoutNotice pins the FR-030/Q3 user-facing
// contract at the default-run entry point: an unborn repository with a
// small staged change writes no notice to stdout or stderr. The description
// flow still needs a model client, and with no API key the run ends in the
// unchanged API-key error — so the assertion is that the skip adds no
// diagnostic-shaped output around that error, only the debug record T045's
// sibling assertion pins.
func TestUnbornRepoSkipsHistoryWithoutNotice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	isolateConfig(t)

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

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0600); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}

	run("add", "a.txt")

	t.Chdir(dir)
	setTestStreams(t)

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}

	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("default run in an unborn repo without API key: expected error, got nil")
	}

	if strings.Contains(outBuf.String(), "no commits") || strings.Contains(errBuf.String(), "no commits") {
		t.Errorf("default run printed a history-skip notice (out=%q err=%q), want diagnostics only",
			outBuf.String(), errBuf.String())
	}
}
