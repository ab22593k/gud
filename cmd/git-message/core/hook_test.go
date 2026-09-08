package core

import (
	"context"
	"os"
	"path/filepath"
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
