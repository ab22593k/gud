package git

import (
	"os"
	"path/filepath"
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
				if !contains(string(content), "git message") {
					t.Errorf("hook should call git message")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			hookDir := filepath.Join(tmpDir, ".git", "hooks")
			os.MkdirAll(hookDir, 0755)

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

func TestRunHookMode(t *testing.T) {
	tests := []struct {
		name     string
		msgFile  string
		content  string
		wantErr  bool
		validate func(t *testing.T, msgFile string)
	}{
		{
			name:    "writes message to file",
			content: "feat: add new feature",
			validate: func(t *testing.T, msgFile string) {
				content, err := os.ReadFile(msgFile)
				if err != nil {
					t.Fatalf("failed to read msg file: %v", err)
				}
				if string(content) != "feat: add new feature" {
					t.Errorf("got %q, want %q", string(content), "feat: add new feature")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := filepath.Join(t.TempDir(), "COMMIT_MSG")
			if tt.content != "" {
				os.WriteFile(tmpFile, []byte("initial content"), 0644)
			}
			t.Skip("skipping RunHookMode test - needs valid API key")
		})
	}
}

func TestUninstallHook(t *testing.T) {
	tmpDir := t.TempDir()
	hookDir := filepath.Join(tmpDir, ".git", "hooks")
	os.MkdirAll(hookDir, 0755)
	hookPath := filepath.Join(hookDir, string(PrepareCommitMsg))

	os.WriteFile(hookPath, []byte("#!/bin/sh\necho test"), 0755)

	err := UninstallHook(hookDir, PrepareCommitMsg)
	if err != nil {
		t.Fatalf("UninstallHook() unexpected error: %v", err)
	}

	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Errorf("hook file should be removed")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && contains(s[1:], substr) || len(s) >= len(substr) && s[0:len(substr)] == substr)
}
