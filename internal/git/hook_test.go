package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallHook(t *testing.T) {
	tests := []struct {
		name         string
		hookType     HookType
		wantErr      bool
		validateHook func(t *testing.T, hookPath string)
	}{
		{
			name:     "install prepare-commit-msg hook",
			hookType: PrepareCommitMsg,
			validateHook: func(t *testing.T, hookPath string) {
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
				if !strings.Contains(string(content), "gud hook run") {
					t.Errorf("hook should call gud hook run")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			hookDir := filepath.Join(tmpDir, ".git", "hooks")
			if err := os.MkdirAll(hookDir, 0755); err != nil {
				t.Fatalf("failed to create hooks dir: %v", err)
			}

			err := InstallHook(hookDir, tt.hookType)
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
	tmpDir := t.TempDir()
	hookDir := filepath.Join(tmpDir, ".git", "hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}
	hookPath := filepath.Join(hookDir, string(PrepareCommitMsg))

	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
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
