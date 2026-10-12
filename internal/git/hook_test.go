package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallHook(t *testing.T) {
	t.Parallel()

	// Use a known binary path for testing.
	testBinary := "/usr/local/bin/gud"

	tests := []struct {
		name         string
		hookType     HookType
		binaryPath   string
		wantErr      bool
		validateHook func(t *testing.T, hookPath string)
	}{
		{
			name:       "install prepare-commit-msg hook",
			hookType:   PrepareCommitMsg,
			binaryPath: testBinary,
			validateHook: func(t *testing.T, hookPath string) {
				t.Helper()

				if _, err := os.Stat(hookPath); os.IsNotExist(err) {
					t.Errorf("hook file should exist at %s", hookPath)
				}

				info, err := os.Stat(hookPath)
				if err != nil {
					t.Fatalf("failed to stat hook file: %v", err)
				}

				if info.Mode()&0111 == 0 {
					t.Errorf("hook file should be executable")
				}

				content, err := os.ReadFile(hookPath)
				if err != nil {
					t.Fatalf("failed to read hook file: %v", err)
				}

				quotedBinary := `'` + testBinary + `'`
				if !strings.Contains(string(content), quotedBinary+" hook run") {
					t.Errorf("hook should call %s hook run, got:\n%s", testBinary, string(content))
				}
			},
		},
		{
			name:       "install hook with single-quoted binary path escaped",
			hookType:   PrepareCommitMsg,
			binaryPath: `/usr/local/gud' -c "rm -rf /" `,
			validateHook: func(t *testing.T, hookPath string) {
				t.Helper()

				content, err := os.ReadFile(hookPath)
				if err != nil {
					t.Fatalf("failed to read hook file: %v", err)
				}
				// The embedded single quote must be shell-escaped so the
				// hook still invokes exactly the intended binary.
				want := `'/usr/local/gud'\'' -c "rm -rf /" ' hook run "$1"`
				if !strings.Contains(string(content), want) {
					t.Errorf("hook should shell-escape embedded single quotes, got:\n%s", string(content))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()

			hookDir := filepath.Join(tmpDir, ".git", "hooks")
			if err := os.MkdirAll(hookDir, 0755); err != nil {
				t.Fatalf("failed to create hooks dir: %v", err)
			}

			err := InstallHook(hookDir, tt.hookType, tt.binaryPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("InstallHook() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if tt.validateHook != nil {
				hookPath := filepath.Join(hookDir, string(tt.hookType))
				tt.validateHook(t, hookPath)
			}
		})
	}
}

func TestUninstallHook(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	hookDir := filepath.Join(tmpDir, ".git", "hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}

	hookPath := filepath.Join(hookDir, string(PrepareCommitMsg))

	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho test"), 0600); err != nil {
		t.Fatalf("failed to write hook file: %v", err)
	}

	err := UninstallHook(hookDir, PrepareCommitMsg)
	if err != nil {
		t.Fatalf("UninstallHook() unexpected error: %v", err)
	}

	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Errorf("hook file should be removed")
	}
}

// TestGlobalHooksPathRoundtrip verifies the global core.hooksPath helpers
// against a hermetic git config (GIT_CONFIG_GLOBAL) so the developer's real
// global config is never touched. Covers the unset (""), set, and
// idempotent-unset-of-unset-key transitions.
//
// Not parallel: GIT_CONFIG_GLOBAL must be set via t.Setenv, which is
// incompatible with t.Parallel.
func TestGlobalHooksPathRoundtrip(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "global-gitconfig"))

	ctx := context.Background()

	got, err := GetGlobalHooksPath(ctx)
	if err != nil {
		t.Fatalf("GetGlobalHooksPath on unset key: %v", err)
	}

	if got != "" {
		t.Errorf("GetGlobalHooksPath() = %q on unset key, want empty", got)
	}

	if err := UnsetGlobalHooksPath(ctx); err != nil {
		t.Errorf("UnsetGlobalHooksPath on unset key should be a no-op, got %v", err)
	}

	const want = "/home/tester/.config/gud/hooks"
	if err := SetGlobalHooksPath(ctx, want); err != nil {
		t.Fatalf("SetGlobalHooksPath: %v", err)
	}

	got, err = GetGlobalHooksPath(ctx)
	if err != nil {
		t.Fatalf("GetGlobalHooksPath after set: %v", err)
	}

	if got != want {
		t.Errorf("GetGlobalHooksPath() = %q, want %q", got, want)
	}

	if err := UnsetGlobalHooksPath(ctx); err != nil {
		t.Fatalf("UnsetGlobalHooksPath: %v", err)
	}

	got, err = GetGlobalHooksPath(ctx)
	if err != nil {
		t.Fatalf("GetGlobalHooksPath after unset: %v", err)
	}

	if got != "" {
		t.Errorf("GetGlobalHooksPath() = %q after unset, want empty", got)
	}
}

// TestGetHookDir verifies the hook directory selection: the repo-local hooks
// dir resolved through git (absolute, worktree aware), and the global
// directory under gud's XDG config home (~/.config/gud/hooks), which matches
// the global config location.
//
// Not parallel: the local cases chdir into fixture repos (t.Chdir is
// incompatible with t.Parallel).
func TestGetHookDir(t *testing.T) {
	ctx := context.Background()

	t.Run("global returns ~/.config/gud/hooks", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("failed to get home directory: %v", err)
		}

		got, err := GetHookDir(ctx, true)
		if err != nil {
			t.Fatalf("GetHookDir(global) error = %v", err)
		}

		if want := filepath.Join(home, ".config", "gud", "hooks"); got != want {
			t.Errorf("GetHookDir(global) = %q, want %q", got, want)
		}
	})

	t.Run("local from repo root returns gitdir hooks", func(t *testing.T) {
		dir := t.TempDir()
		initHookTestRepo(t, dir)
		t.Chdir(dir)

		got, err := GetHookDir(ctx, false)
		if err != nil {
			t.Fatalf("GetHookDir(local) error = %v", err)
		}

		if want := filepath.Join(dir, ".git", "hooks"); got != want {
			t.Errorf("GetHookDir(local) = %q, want %q", got, want)
		}
	})

	// Regression lock: a relative .git/hooks invoked from repo/sub/dir
	// created a stray ./.git/hooks under cwd instead of the repo's hooks.
	t.Run("local from subdirectory returns repo hooks", func(t *testing.T) {
		dir := t.TempDir()
		initHookTestRepo(t, dir)

		sub := filepath.Join(dir, "repo", "sub", "dir")
		if err := os.MkdirAll(sub, 0750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		t.Chdir(sub)

		got, err := GetHookDir(ctx, false)
		if err != nil {
			t.Fatalf("GetHookDir(local) error = %v", err)
		}

		if want := filepath.Join(dir, ".git", "hooks"); got != want {
			t.Errorf("GetHookDir(local) from subdir = %q, want %q", got, want)
		}

		if !filepath.IsAbs(got) {
			t.Errorf("GetHookDir(local) = %q, want absolute path", got)
		}
	})

	// In a worktree .git is a file pointing at the worktree git dir; the
	// hooks must land there, not in a relative .git/hooks.
	t.Run("local in worktree returns worktree gitdir hooks", func(t *testing.T) {
		dir := t.TempDir()
		initHookTestRepo(t, dir)

		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v1\n"), 0600); err != nil {
			t.Fatalf("write: %v", err)
		}

		gitIn(t, dir, "add", ".")
		gitIn(t, dir, "commit", "-q", "-m", "init")

		wt := filepath.Join(t.TempDir(), "wt")
		gitIn(t, dir, "worktree", "add", wt)
		t.Cleanup(func() { gitIn(t, dir, "worktree", "remove", "--force", wt) })

		t.Chdir(wt)

		got, err := GetHookDir(ctx, false)
		if err != nil {
			t.Fatalf("GetHookDir(local) error = %v", err)
		}

		want := filepath.Join(gitIn(t, wt, "rev-parse", "--absolute-git-dir"), "hooks")
		if got != want {
			t.Errorf("GetHookDir(local) in worktree = %q, want %q", got, want)
		}
	})

	t.Run("local outside repo errors", func(t *testing.T) {
		t.Chdir(t.TempDir())

		if _, err := GetHookDir(ctx, false); err == nil {
			t.Error("GetHookDir(local) outside repo = nil error, want failure")
		}
	})
}

// initHookTestRepo initialises an empty git repo with local identity so hook
// tests can resolve its git dir without touching the developer's config.
func initHookTestRepo(t *testing.T, dir string) {
	t.Helper()

	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "config", "user.name", "Test")
	gitIn(t, dir, "config", "user.email", "test@example.com")
}

// gitIn runs git args in dir and fails the test on error, returning trimmed
// stdout for oracle queries like rev-parse.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()

	//nolint:gosec // test-only git invocation with fixed repo-local args
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}

	return strings.TrimSpace(string(out))
}
