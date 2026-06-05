package git

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestGetStagedDiff(t *testing.T) {
	ctx := context.Background()
	_, err := GetStagedDiff(ctx)

	if err != nil {
		t.Errorf("GetStagedDiff() error = %v, want nil", err)
	}
}

func TestGetStagedDiff_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	diff, err := GetStagedDiff(ctx)

	if err != nil {
		t.Fatalf("GetStagedDiff() unexpected error: %v", err)
	}

	t.Logf("Staged diff output:\n%s", diff)
}

func TestGetUnstagedDiff(t *testing.T) {
	ctx := context.Background()
	_, err := GetUnstagedDiff(ctx)

	if err != nil {
		t.Errorf("GetUnstagedDiff() error = %v, want nil", err)
	}
}

func TestGetUnstagedDiff_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	diff, err := GetUnstagedDiff(ctx)

	if err != nil {
		t.Fatalf("GetUnstagedDiff() unexpected error: %v", err)
	}

	t.Logf("Unstaged diff output:\n%s", diff)
}

func TestGetRecentCommits(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		n    int
	}{
		{name: "zero requests empty", n: 0},
		{name: "negative requests empty", n: -3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetRecentCommits(ctx, tt.n)
			if got != "" {
				t.Errorf("GetRecentCommits(%d) = %q, want empty string", tt.n, got)
			}
			if err != nil {
				t.Errorf("GetRecentCommits(%d) unexpected error: %v", tt.n, err)
			}
		})
	}
}

func TestGetRecentCommits_NonEmpty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	got, err := GetRecentCommits(ctx, 5)
	if err != nil {
		t.Fatalf("GetRecentCommits(5) unexpected error: %v", err)
	}
	if got == "" {
		t.Fatal("GetRecentCommits(5) returned empty, expected at least one commit")
	}
	t.Logf("Recent commits:\n%s", got)
}

func TestCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Set up a temp repo with a staged file
	dir := t.TempDir()
	for _, cmd := range []string{
		"git init",
		"git config user.email test@example.com",
		"git config user.name Test",
	} {
		c := exec.Command("sh", "-c", cmd)
		c.Dir = dir
		if err := c.Run(); err != nil {
			t.Fatalf("%s failed: %v", cmd, err)
		}
	}
	if err := os.WriteFile(dir+"/file.go", []byte("package main\n"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = dir
	if err := addCmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer os.Chdir(origDir)

	ctx := context.Background()
	if err := Commit(ctx, "feat: initial commit\n\nAdd main package."); err != nil {
		t.Fatalf("Commit() unexpected error: %v", err)
	}

	// Verify the commit was created
	got, err := GetRecentCommits(ctx, 1)
	if err != nil {
		t.Fatalf("GetRecentCommits(1) unexpected error: %v", err)
	}
	if !strings.Contains(got, "feat: initial commit") {
		t.Errorf("Commit message not found in log, got: %s", got)
	}
}

func TestCommit_EmptyMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	for _, cmd := range []string{
		"git init",
		"git config user.email test@example.com",
		"git config user.name Test",
	} {
		c := exec.Command("sh", "-c", cmd)
		c.Dir = dir
		if err := c.Run(); err != nil {
			t.Fatalf("%s failed: %v", cmd, err)
		}
	}
	if err := os.WriteFile(dir+"/file.go", []byte("package main\n"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = dir
	if err := addCmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer os.Chdir(origDir)

	ctx := context.Background()
	err := Commit(ctx, "")
	if err == nil {
		t.Fatal("Commit() with empty message should return error")
	}
}

func TestGetRecentCommits_EmptyRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir to temp repo failed: %v", err)
	}
	defer os.Chdir(origDir)

	ctx := context.Background()
	got, err := GetRecentCommits(ctx, 5)
	if got != "" {
		t.Errorf("GetRecentCommits(5) on empty repo = %q, want empty string", got)
	}
	if err == nil {
		t.Errorf("GetRecentCommits(5) on empty repo should return an error")
	}
}
