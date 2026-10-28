package core

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gud/internal/git"
	"gud/internal/request"

	"github.com/spf13/pflag"
)

// isolateConfig points HOME and GUD_CONFIG_PATH at the test temp dir so the
// mediator cannot pick up a real user-level gud config or API key. Ambient
// git context (GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE) is unset so git
// invocations resolve the test's working directory; git rejects an empty
// value for these variables, so they must be removed, not set to "".
// GIT_CONFIG_GLOBAL/GIT_CONFIG_SYSTEM redirect the global and system git
// configs to temp files so git commit signatures cannot depend on the
// developer's identity, aliases, or hooks.
//
// Safe because these flow tests are sequential: parallel test bodies in the
// package cannot run while a sequential test is executing.
func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GUD_CONFIG_PATH", "")
	t.Setenv("GOOGLE_API_KEY", "")

	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "global-gitconfig"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(t.TempDir(), "system-gitconfig"))

	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"} {
		orig, had := os.LookupEnv(key)
		if !had {
			continue
		}

		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}

		t.Cleanup(func() {
			//nolint:usetesting // t.Setenv cannot restore a variable that was unset; os.Setenv is required here.
			if err := os.Setenv(key, orig); err != nil {
				t.Errorf("restore %s: %v", key, err)
			}
		})
	}
}

// setTestStreams redirects command output and gives Execute explicit empty
// arguments. SetArgs(nil) would fall back to os.Args[1:] (the go-test flags),
// and cobra flag "Changed" state is sticky across parses on the shared
// rootCmd, so previously parsed flags must be cleared too.
func setTestStreams(t *testing.T) {
	t.Helper()

	outBuf, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs([]string{})
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		_ = f.Value.Set(f.DefValue)
	})
	rootCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		_ = f.Value.Set(f.DefValue)
	})
	t.Cleanup(func() {
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
	})
}

// TestRunGenerate_NoStagedChanges verifies the primary validation path: with a
// valid repo but nothing staged, the default command fails fast with the
// actionable "no staged changes" error before any client
// initialisation.
//
// The test runs inside a throwaway repo (newCoreHistoryTestRepo chdirs into
// it) so it asserts an empty index we control — without that it reads the
// package's own repository index and fails whenever the gud checkout happens
// to have staged changes.
func TestRunGenerate_NoStagedChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	isolateConfig(t)

	newCoreHistoryTestRepo(t)

	setTestStreams(t)

	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		t.Logf("DEBUG persistent flag %s=%q", f.Name, f.Value.String())
	})
	rootCmd.Flags().VisitAll(func(f *pflag.Flag) {
		t.Logf("DEBUG local flag %s=%q", f.Name, f.Value.String())
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("runGenerate with no staged changes: expected error, got nil")
	}

	if !strings.Contains(err.Error(), "no staged changes") {
		t.Errorf("error = %v, want 'no staged changes' guidance", err)
	}
}

// TestStagedChangeEvidenceBelowThresholdRename is the normal-path end to
// end: a rename read by git as delete+add still resolves both names through
// the real staged probe, and a description naming both is grounded — even
// when the submitted diff is cut before the rename record.
func TestStagedChangeEvidenceBelowThresholdRename(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	isolateConfig(t)
	newCoreHistoryTestRepo(t)
	ctx := t.Context()

	if err := os.WriteFile("old_name.go", []byte("package old\n"), 0600); err != nil {
		t.Fatalf("write old_name.go: %v", err)
	}

	run := func(args ...string) {
		t.Helper()

		if out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", ".")
	run("commit", "-q", "-m", "add old_name.go")
	run("mv", "old_name.go", "new_name.go")
	// Below-threshold similarity: zero shared content so the rendered diff
	// reads delete+add rather than a rename.
	const unrelated = "package totally_unrelated_content_zero_overlap_here\n"

	if err := os.WriteFile("new_name.go", []byte(unrelated), 0600); err != nil {
		t.Fatalf("write new_name.go: %v", err)
	}

	run("add", "-A")

	// Query the probe directly and through the resolver: both must report
	// the rename's two sides.
	paths, err := git.StagedChangePaths(ctx)
	if err != nil {
		t.Fatalf("StagedChangePaths() = %v", err)
	}

	for _, want := range []string{"old_name.go", "new_name.go"} {
		if !slices.Contains(paths, want) {
			t.Fatalf("StagedChangePaths() = %v, want %q", paths, want)
		}
	}

	change, _, err := resolveStagedChange(ctx, &AppContext{})
	if err != nil {
		t.Fatalf("resolveStagedChange() = %v", err)
	}

	msg := "Rename old file\n\nRename old_name.go to new_name.go."
	if got := request.VerifyMessage(msg, change, 72); len(got) != 0 {
		t.Errorf("VerifyMessage(delete+add diff) = %v, want clean", got)
	}

	// A true 100% rename with the diff cut before the rename record: the
	// same Paths still ground the pre-image.
	const identical = "package old\n"
	if err := os.WriteFile("new_name.go", []byte(identical), 0600); err != nil {
		t.Fatalf("restore new_name.go: %v", err)
	}

	run("add", "-A")

	// Re-resolve after restoring identical content: the rendered 100%
	// rename diff is cut below, so only the probe can ground the pre-image.
	change, _, err = resolveStagedChange(ctx, &AppContext{})
	if err != nil {
		t.Fatalf("resolveStagedChange() = %v", err)
	}

	cut := cutDiffBefore(t, change.Diff, "rename from")
	cutChange := request.Change{Diff: cut, Paths: change.Paths}

	if got := request.VerifyMessage(msg, cutChange, 72); len(got) != 0 {
		t.Errorf("VerifyMessage(cut diff) = %v, want clean (probe supplies pre-image)", got)
	}
}

// cutDiffBefore truncates diff before the first line with the given prefix.
func cutDiffBefore(t *testing.T, diff, prefix string) string {
	t.Helper()

	for line := range strings.Lines(diff) {
		if strings.HasPrefix(line, prefix) {
			if before, _, ok := strings.Cut(diff, line); ok {
				return before
			}
		}
	}

	t.Fatalf("diff has no %q record to cut", prefix)

	return ""
}

// TestRunGenerate_StagedWithoutAPIKey verifies the second validation path:
// with changes staged but no API key configured, generation fails with an
// API-key error instead of entering the interactive review loop.
func TestRunGenerate_StagedWithoutAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	isolateConfig(t)

	newCoreHistoryTestRepo(t)

	if err := os.WriteFile("staged.txt", []byte("change\n"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	add := exec.CommandContext(t.Context(), "git", "add", "staged.txt")
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}

	setTestStreams(t)

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("runGenerate without API key: expected error, got nil")
	}

	if !strings.Contains(strings.ToLower(err.Error()), "api key") &&
		!strings.Contains(strings.ToLower(err.Error()), "key") {
		t.Errorf("error = %v, want API key guidance", err)
	}
}

// TestTruncationNotice verifies FR-007/SC-009 at the interactive entry point:
// when the staged change exceeds git.MaxDiffBytes the developer is told the
// description would come from a partial change. The notice is printed at
// diff-acquisition time (before client initialisation), so this test runs
// with no API key: the notice must appear in the command output while the
// API-key error still surfaces unchanged — the notice informs, it never
// alters the flow.
func TestTruncationNotice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	isolateConfig(t)

	newCoreHistoryTestRepo(t)

	big := make([]byte, 0, 300<<10)
	for len(big) < 300<<10 {
		big = append(big, "oversized staged line for truncation disclosure\n"...)
	}

	if err := os.WriteFile("big.txt", big, 0600); err != nil {
		t.Fatalf("write big.txt: %v", err)
	}

	add := exec.CommandContext(t.Context(), "git", "add", "big.txt")
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}

	setTestStreams(t)

	outBuf := &strings.Builder{}
	rootCmd.SetOut(outBuf)

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("runGenerate with oversized staged change and no API key: expected error, got nil")
	}

	if !strings.Contains(strings.ToLower(err.Error()), "api key") {
		t.Errorf("error = %v, want unchanged API key guidance", err)
	}

	if got := outBuf.String(); !strings.Contains(got, "partial change") {
		t.Errorf("output = %q, want one-line partial-change truncation notice", got)
	}
}
