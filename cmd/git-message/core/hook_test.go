package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gud/internal/git"
	"gud/internal/profile"

	"github.com/spf13/cobra"
)

// TestHookModeToleratesUncachedProfile verifies that hook mode never blocks a
// git commit because a configured profile is not cached. The tolerant
// constructor must succeed and keep the profile name (whose content degrades
// to "" via resolveProfileContent), while the strict constructor used by
// normal mode still rejects the missing profile.
func TestHookModeToleratesUncachedProfile(t *testing.T) {
	orig := profileManager

	t.Cleanup(func() { profileManager = orig })
	profileManager = profile.NewManagerWithDir(t.TempDir())

	t.Setenv("GUD_CONFIG_PATH", t.TempDir()+"/nonexistent.json")

	cmd := &cobra.Command{}
	addPersistentFlags(cmd)
	// Parse flags so cobra merges the persistent flags into cmd.Flags(),
	// mirroring how configFromCmd observes them during a real execution.
	if err := cmd.ParseFlags([]string{"--profile", "nonexistent-slug-12345"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	// Tolerant constructor (hook mode) must succeed despite the uncached profile.
	app, err := NewAppContextTolerant(cmd)
	if err != nil {
		t.Fatalf("NewAppContextTolerant with uncached profile: %v", err)
	}

	if app == nil {
		t.Fatal("NewAppContextTolerant returned nil app")
	}

	if got := app.Config().Profile; got != "nonexistent-slug-12345" {
		t.Errorf("Profile = %q, want %q", got, "nonexistent-slug-12345")
	}
	// Content resolution degrades gracefully, matching generate.go behaviour.
	if got := resolveProfileContent(string(app.Config().Profile)); got != "" {
		t.Errorf("resolveProfileContent() = %q, want ''", got)
	}

	// Strict constructor (normal mode) must still reject the missing profile.
	if _, err := NewAppContext(cmd); err == nil {
		t.Error("NewAppContext with uncached profile should return an error")
	}
}

func TestHasMeaningfulContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "empty string", text: "", want: false},
		{name: "only blank lines", text: "\n\n  \n", want: false},
		{name: "only git comments", text: "# Please enter commit message\n# Lines starting with # are comments", want: false},
		{name: "single real line", text: "feat: add login", want: true},
		{name: "real line with leading text", text: "  feat: add login\n", want: true},
		{name: "comments then content", text: "# Please enter commit message\nfeat: add login\n# more comments", want: true},
		{name: "content then comments", text: "fix: resolve crash\n# Co-authored-by: someone@example.com", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := hasMeaningfulContent(tt.text)
			if got != tt.want {
				t.Errorf("hasMeaningfulContent(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

// hookTestEnv isolates the process from the developer's real environment:
// HOME keeps ~/.config/gud/hooks out of the real home directory, and
// GIT_CONFIG_GLOBAL redirects the global git config to a temp file.
func hookTestEnv(t *testing.T) context.Context {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("GUD_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "global-gitconfig"))

	return context.Background()
}

// TestHookInstallUninstallGlobalHooksPath verifies that a global install
// wires core.hooksPath to the gud hooks directory (idempotently) and a
// global uninstall clears both the hook file and the config value.
func TestHookInstallUninstallGlobalHooksPath(t *testing.T) {
	ctx := hookTestEnv(t)

	if err := runHookInstall(ctx, true); err != nil {
		t.Fatalf("runHookInstall(global): %v", err)
	}

	hookDir, err := git.GetHookDir(true)
	if err != nil {
		t.Fatalf("GetHookDir: %v", err)
	}

	hookPath := filepath.Join(hookDir, string(git.PrepareCommitMsg))
	if _, err := os.Stat(hookPath); err != nil {
		t.Errorf("hook file should exist at %s: %v", hookPath, err)
	}

	got, err := git.GetGlobalHooksPath(ctx)
	if err != nil {
		t.Fatalf("GetGlobalHooksPath: %v", err)
	}

	if got != hookDir {
		t.Errorf("core.hooksPath = %q, want %q", got, hookDir)
	}

	// Reinstall must stay idempotent.
	if err := runHookInstall(ctx, true); err != nil {
		t.Fatalf("reinstall: %v", err)
	}

	if err := runHookUninstall(ctx, true); err != nil {
		t.Fatalf("runHookUninstall(global): %v", err)
	}

	got, err = git.GetGlobalHooksPath(ctx)
	if err != nil {
		t.Fatalf("GetGlobalHooksPath after uninstall: %v", err)
	}

	if got != "" {
		t.Errorf("core.hooksPath = %q after uninstall, want empty", got)
	}

	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Errorf("hook file should be removed from %s", hookPath)
	}
}

// TestHookInstallKeepsForeignHooksPath verifies that install does not
// override a global core.hooksPath pointing somewhere else: the user's own
// hook configuration wins and gud's hook simply is not activated.
func TestHookInstallKeepsForeignHooksPath(t *testing.T) {
	ctx := hookTestEnv(t)

	const foreign = "/usr/local/other-hooks"
	if err := git.SetGlobalHooksPath(ctx, foreign); err != nil {
		t.Fatalf("SetGlobalHooksPath: %v", err)
	}

	if err := runHookInstall(ctx, true); err != nil {
		t.Fatalf("runHookInstall(global): %v", err)
	}

	got, err := git.GetGlobalHooksPath(ctx)
	if err != nil {
		t.Fatalf("GetGlobalHooksPath: %v", err)
	}

	if got != foreign {
		t.Errorf("core.hooksPath = %q, want foreign value %q preserved", got, foreign)
	}
}

// TestBuildHookPromptDiff verifies hook-mode prompt construction: removed
// hunks excluded by default with names retained, full content under opt-in,
// and names-only stages reported as content (never silently skipped).
func TestBuildHookPromptDiff(t *testing.T) {
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

	ctx := context.Background()

	prompt, ok, err := buildHookPromptDiff(ctx, false)
	if err != nil {
		t.Fatalf("buildHookPromptDiff(include=false) error = %v", err)
	}

	if !ok {
		t.Fatal("buildHookPromptDiff(include=false) reports no content for a staged change")
	}

	if !strings.Contains(prompt, "// changed") {
		t.Errorf("hook prompt missing modified hunks:\n%s", prompt)
	}

	if strings.Contains(prompt, "package doomed") {
		t.Errorf("hook prompt leaks deleted content:\n%s", prompt)
	}

	if !strings.Contains(prompt, "doomed.go") {
		t.Errorf("hook prompt missing deleted name:\n%s", prompt)
	}

	if !strings.Contains(prompt, "moved.txt -> renamed.txt") {
		t.Errorf("hook prompt missing rename pair:\n%s", prompt)
	}

	full, ok, err := buildHookPromptDiff(ctx, true)
	if err != nil {
		t.Fatalf("buildHookPromptDiff(include=true) error = %v", err)
	}

	if !ok {
		t.Fatal("buildHookPromptDiff(include=true) reports no content for a staged change")
	}

	if !strings.Contains(full, "package doomed") {
		t.Errorf("opt-in hook prompt missing deleted content:\n%s", full)
	}

	// Deletion-only stage: names are content, so the hook must proceed.
	run("reset", "-q")
	run("rm", "-q", "doomed.go")

	namesOnly, ok, err := buildHookPromptDiff(ctx, false)
	if err != nil {
		t.Fatalf("buildHookPromptDiff(names-only) error = %v", err)
	}

	if !ok {
		t.Error("names-only stage must report content so the hook does not silently skip")
	}

	if !strings.Contains(namesOnly, "doomed.go") {
		t.Errorf("names-only hook prompt missing deleted name:\n%s", namesOnly)
	}

	// Truly empty stage: no content, hook stays silent.
	run("reset", "-q")
	run("checkout", "-q", "--", ".")

	if _, ok, err := buildHookPromptDiff(ctx, false); err != nil || ok {
		t.Errorf("empty stage: buildHookPromptDiff = (_, %v, %v), want (_, false, nil)", ok, err)
	}
}
