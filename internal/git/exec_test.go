package git

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestRunGitOutput pins the stream-separated exec helper the D9 probe needs:
// stdout is captured alone, stderr rides the wrapped error, and a success
// preserves exact bytes (a warning must never corrupt a NUL record stream).
func TestRunGitOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()

		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")

	t.Chdir(dir)

	ctx := context.Background()

	// A failing query returns empty stdout and an error carrying git's
	// stderr, proving the streams are not folded together.
	out, err := runGitOutput(ctx, "show", "does-not-exist")
	if err == nil {
		t.Fatal("runGitOutput(show does-not-exist) = nil, want error")
	}

	if len(out) != 0 {
		t.Errorf("runGitOutput(failing) stdout = %q, want empty", out)
	}

	if !containsStderr(err, "does-not-exist") && !containsStderr(err, "unknown revision") {
		t.Errorf("error = %v, want git's stderr", err)
	}

	exitErr, ok := errors.AsType[*exec.ExitError](err)
	if !ok || exitErr == nil {
		t.Errorf("error = %T (%v), want the exit error wrapped", err, err)
	}

	// A successful NUL-delimited query preserves the exact bytes: no stderr
	// is appended to stdout.
	if err := os.WriteFile("a b.txt", []byte("x\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	run("add", "a b.txt")

	out, err = runGitOutput(ctx, "diff", "--cached", "--name-status", "-z")
	if err != nil {
		t.Fatalf("runGitOutput(diff --cached --name-status -z) = %v", err)
	}

	if want := []byte("A\x00a b.txt\x00"); !bytes.Equal(out, want) {
		t.Errorf("runGitOutput() = %q, want %q (exact NUL bytes)", out, want)
	}
}

func containsStderr(err error, s string) bool {
	for err != nil {
		if strings.Contains(err.Error(), s) {
			return true
		}

		err = errors.Unwrap(err)
	}

	return false
}
