package core

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// isolateConfig points HOME and GUD_CONFIG_PATH at the test temp dir so the
// mediator cannot pick up a real user-level gud config or API key.
func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GUD_CONFIG_PATH", "")
	t.Setenv("GOOGLE_API_KEY", "")
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
// actionable "no staged changes" error before any HelixDB probe or client
// initialisation.
func TestRunGenerate_NoStagedChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	isolateConfig(t)

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
