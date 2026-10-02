package git

import (
	"context"
	"os"
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

	got, err := GlobalHooksPath(ctx)
	if err != nil {
		t.Fatalf("GlobalHooksPath on unset key: %v", err)
	}

	if got != "" {
		t.Errorf("GlobalHooksPath() = %q on unset key, want empty", got)
	}

	if err := UnsetGlobalHooksPath(ctx); err != nil {
		t.Errorf("UnsetGlobalHooksPath on unset key should be a no-op, got %v", err)
	}

	const want = "/home/tester/.config/gud/hooks"
	if err := SetGlobalHooksPath(ctx, want); err != nil {
		t.Fatalf("SetGlobalHooksPath: %v", err)
	}

	got, err = GlobalHooksPath(ctx)
	if err != nil {
		t.Fatalf("GlobalHooksPath after set: %v", err)
	}

	if got != want {
		t.Errorf("GlobalHooksPath() = %q, want %q", got, want)
	}

	if err := UnsetGlobalHooksPath(ctx); err != nil {
		t.Fatalf("UnsetGlobalHooksPath: %v", err)
	}

	got, err = GlobalHooksPath(ctx)
	if err != nil {
		t.Fatalf("GlobalHooksPath after unset: %v", err)
	}

	if got != "" {
		t.Errorf("GlobalHooksPath() = %q after unset, want empty", got)
	}
}

// TestHookDir verifies the hook directory selection: the repo-local
// .git/hooks, and the global directory under gud's XDG config home
// (~/.config/gud/hooks), which matches the global config location.
func TestHookDir(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	tests := []struct {
		name    string
		global  bool
		want    string
		wantErr bool
	}{
		{
			name:   "local returns .git/hooks",
			global: false,
			want:   filepath.Join(".git", "hooks"),
		},
		{
			name:   "global returns ~/.config/gud/hooks",
			global: true,
			want:   filepath.Join(home, ".config", "gud", "hooks"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := HookDir(tt.global)
			if (err != nil) != tt.wantErr {
				t.Fatalf("HookDir(global=%v) error = %v, wantErr %v", tt.global, err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("HookDir(global=%v) = %q, want %q", tt.global, got, tt.want)
			}
		})
	}
}
