package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gud/internal/git"
)

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
