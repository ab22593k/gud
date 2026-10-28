package core

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

// TestResolveChangePaths pins the D9 branch-helper contract: a probe that
// succeeds yields its paths unchanged and runs once; a probe that fails
// yields nil, runs once, and emits exactly one debug record naming the shape
// and the cause. The helper never returns an error or a notice — the probe is
// optional evidence, so its failure degrades to the diff-derived set.
func TestResolveChangePaths(t *testing.T) {
	t.Run("success passes paths unchanged", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32

		got := resolveChangePaths(func() ([]string, error) {
			calls.Add(1)

			return []string{"old_name.go", "new_name.go"}, nil
		})

		if calls.Load() != 1 {
			t.Errorf("probe calls = %d, want 1", calls.Load())
		}

		if len(got) != 2 || got[0] != "old_name.go" || got[1] != "new_name.go" {
			t.Errorf("resolveChangePaths() = %v, want the probe result unchanged", got)
		}
	})

	t.Run("failure yields nil with one debug record", func(t *testing.T) {
		var buf bytes.Buffer

		restore := captureSlogDebug(&buf)
		defer restore()

		var calls atomic.Int32

		boom := errors.New("boom")

		got := resolveChangePaths(func() ([]string, error) {
			calls.Add(1)

			return nil, boom
		})

		if calls.Load() != 1 {
			t.Errorf("probe calls = %d, want 1", calls.Load())
		}

		if got != nil {
			t.Errorf("resolveChangePaths() = %v, want nil (diff-derived fallback)", got)
		}

		records := strings.Count(buf.String(), "change-path probe failed; using diff-derived paths")
		if records != 1 {
			t.Errorf("debug records = %d, want exactly 1:\n%s", records, buf.String())
		}

		if !strings.Contains(buf.String(), "boom") {
			t.Errorf("debug record does not carry the cause:\n%s", buf.String())
		}
	})
}

// captureSlogDebug installs a LevelDebug text handler over buf as the default
// logger and returns a restore function for cleanup. Tests using it are
// sequential: the default logger is process-global.
func captureSlogDebug(buf *bytes.Buffer) func() {
	prev := slog.Default()

	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	return func() { slog.SetDefault(prev) }
}

// TestStagedChangeProbeOnce pins the staged resolver: the diff comes from
// the existing staged-diff acquisition (deleted-context appended), the paths
// from the injected staged probe exactly once, and a probe error yields
// Paths == nil with the same diff — never an error, never a notice.
func TestStagedChangeProbeOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	isolateConfig(t)
	newCoreHistoryTestRepo(t)

	ctx := context.Background()

	// Stage a change so the staged-diff acquisition has evidence: the
	// fixture repo commits everything.
	if err := os.WriteFile("staged.txt", []byte("staged\n"), 0600); err != nil {
		t.Fatalf("write staged.txt: %v", err)
	}

	runStaged := func(args ...string) {
		t.Helper()

		if out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runStaged("add", "staged.txt")

	app := &AppContext{}

	var calls atomic.Int32

	app.stagedPathsFn = func(context.Context) ([]string, error) {
		calls.Add(1)

		return []string{"file1.txt"}, nil
	}

	change, truncated, err := resolveStagedChange(ctx, app)
	if err != nil {
		t.Fatalf("resolveStagedChange() = %v", err)
	}

	if calls.Load() != 1 {
		t.Errorf("staged probe calls = %d, want 1", calls.Load())
	}

	if len(change.Paths) != 1 || change.Paths[0] != "file1.txt" {
		t.Errorf("Change.Paths = %v, want the probe result", change.Paths)
	}

	if truncated {
		t.Error("resolveStagedChange() truncated = true, want false (small fixture diff)")
	}

	// The Diff is the submitted evidence: staged diff with deleted context
	// appended, unchanged by path resolution.
	wantDiff, _, _ := getStagedDiffOrError(ctx)
	if change.Diff != wantDiff {
		t.Errorf("Change.Diff differs from the submitted staged diff:\n got: %q\nwant: %q", change.Diff, wantDiff)
	}

	// A failing probe degrades to nil paths with the same diff.
	app.stagedPathsFn = func(context.Context) ([]string, error) {
		return nil, errors.New("boom")
	}

	change, _, err = resolveStagedChange(ctx, app)
	if err != nil {
		t.Fatalf("resolveStagedChange(probe error) = %v, want nil (fallback, not failure)", err)
	}

	if change.Paths != nil {
		t.Errorf("Change.Paths = %v, want nil on probe error", change.Paths)
	}

	if change.Diff != wantDiff {
		t.Errorf("Change.Diff changed on probe error:\n got: %q\nwant: %q", change.Diff, wantDiff)
	}
}

// TestAppContextChangePaths pins the accessor branch point: injected probe
// functions are used as-is, and nil fields fall back to the git probes.
func TestAppContextChangePaths(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	isolateConfig(t)
	newCoreHistoryTestRepo(t)

	ctx := context.Background()

	injected := &AppContext{
		stagedPathsFn: func(context.Context) ([]string, error) {
			return []string{"injected.go"}, nil
		},
		commitPathsFn: func(context.Context, string) ([]string, error) {
			return []string{"injected-commit.go"}, nil
		},
	}

	if got, err := injected.stagedChangePaths(ctx); err != nil || len(got) != 1 || got[0] != "injected.go" {
		t.Errorf("stagedChangePaths(injected) = (%v, %v), want ([injected.go], nil)", got, err)
	}

	if got, err := injected.commitChangePaths(ctx, "HEAD"); err != nil || len(got) != 1 || got[0] != "injected-commit.go" {
		t.Errorf("commitChangePaths(injected) = (%v, %v), want ([injected-commit.go], nil)", got, err)
	}

	// Nil fields fall back to the git probes against the real staged index.
	fallback := &AppContext{}

	if err := os.WriteFile("fallback.txt", []byte("fallback\n"), 0600); err != nil {
		t.Fatalf("write fallback.txt: %v", err)
	}

	if out, err := exec.CommandContext(ctx, "git", "add", "fallback.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	got, err := fallback.stagedChangePaths(ctx)
	if err != nil {
		t.Fatalf("stagedChangePaths(fallback) = %v", err)
	}

	// The fixture repo has committed everything: the staged index is clean,
	// so the fallback probe returns an empty (non-nil) slice. Either way it
	// must not error.
	_ = got

	// A resolve of an injected probe produces a fully injected Change.
	change, _, err := resolveStagedChange(ctx, injected)
	if err != nil {
		t.Fatalf("resolveStagedChange(injected) = %v", err)
	}

	if len(change.Paths) != 1 || change.Paths[0] != "injected.go" {
		t.Errorf("Change.Paths = %v, want [injected.go]", change.Paths)
	}
}

// TestResolveChangePathsNeverFails locks the diagnostics-only fallback: no
// probe outcome ever surfaces as an error or a user-facing notice from the
// resolver itself — callers proceed with diff-derived paths.
func TestResolveChangePathsNeverFails(t *testing.T) {
	t.Parallel()

	if got := resolveChangePaths(func() ([]string, error) { return nil, errors.New("boom") }); got != nil {
		t.Errorf("resolveChangePaths(error) = %v, want nil", got)
	}

	// A nil slice result with nil error is passed through, not upgraded.
	if got := resolveChangePaths(func() ([]string, error) { return nil, nil }); got != nil {
		t.Errorf("resolveChangePaths(nil, nil) = %v, want nil", got)
	}
}
