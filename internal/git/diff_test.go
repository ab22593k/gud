package git

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestGetStagedDiff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := GetStagedDiff(ctx)

	if err != nil {
		t.Errorf("GetStagedDiff() error = %v, want nil", err)
	}
}

func TestGetStagedDiff_Integration(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	ctx := context.Background()
	_, err := GetUnstagedDiff(ctx)

	if err != nil {
		t.Errorf("GetUnstagedDiff() error = %v, want nil", err)
	}
}

func TestGetUnstagedDiff_Integration(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
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
	defer func() { _ = os.Chdir(origDir) }()

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
	defer func() { _ = os.Chdir(origDir) }()

	ctx := context.Background()
	err := Commit(ctx, "")
	if err == nil {
		t.Fatal("Commit() with empty message should return error")
	}
}

func TestGetStagedDeletedFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	got, err := GetStagedDeletedFiles(ctx)
	if err != nil {
		t.Errorf("GetStagedDeletedFiles() error = %v, want nil", err)
	}
	if got != "" {
		t.Logf("GetStagedDeletedFiles() returned (expected if no staged deletions): %q", got)
	}
}

func TestGetStagedDeletedFiles_WithDeletion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	for _, cmd := range []string{
		"git init",
		"git config user.email test@example.com",
		"git config user.name Test",
	} { //nolint:goconst
		c := exec.Command("sh", "-c", cmd)
		c.Dir = dir
		if err := c.Run(); err != nil {
			t.Fatalf("%s failed: %v", cmd, err)
		}
	}
	if err := os.WriteFile(dir+"/file.go", []byte("package main\n"), 0644); err != nil {
		t.Fatalf("write file.go failed: %v", err)
	}
	if err := os.WriteFile(dir+"/keep.go", []byte("package main\n"), 0644); err != nil {
		t.Fatalf("write keep.go failed: %v", err)
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
	defer func() { _ = os.Chdir(origDir) }()

	ctx := context.Background()

	// First commit both files
	if err := Commit(ctx, "initial commit\n\nAdd file.go and keep.go."); err != nil {
		t.Fatalf("initial Commit() failed: %v", err)
	}

	// Delete one file and stage the deletion
	if err := os.Remove("file.go"); err != nil {
		t.Fatalf("remove file.go failed: %v", err)
	}
	rmCmd := exec.Command("git", "rm", "file.go")
	rmCmd.Dir = dir
	if err := rmCmd.Run(); err != nil {
		t.Fatalf("git rm file.go failed: %v", err)
	}

	// Modify the kept file to also have some non-deletion diff
	if err := os.WriteFile("keep.go", []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("write keep.go failed: %v", err)
	}
	addAgain := exec.Command("git", "add", "keep.go")
	addAgain.Dir = dir
	if err := addAgain.Run(); err != nil {
		t.Fatalf("git add keep.go failed: %v", err)
	}

	// Verify GetStagedDeletedFiles returns the deleted file name
	deleted, err := GetStagedDeletedFiles(ctx)
	if err != nil {
		t.Fatalf("GetStagedDeletedFiles() unexpected error: %v", err)
	}
	if !strings.Contains(deleted, "file.go") {
		t.Errorf("GetStagedDeletedFiles() = %q, want to contain %q", deleted, "file.go")
	}
	if strings.Contains(deleted, "keep.go") {
		t.Errorf("GetStagedDeletedFiles() = %q, should NOT contain %q", deleted, "keep.go")
	}

	// Verify GetStagedDiff does NOT contain the deleted file's content
	diff, err := GetStagedDiff(ctx)
	if err != nil {
		t.Fatalf("GetStagedDiff() unexpected error: %v", err)
	}
	if strings.Contains(diff, "package main") && strings.Contains(diff, "file.go") {
		t.Errorf("GetStagedDiff() should NOT contain deleted file content, got:\n%s", diff)
	}
	if !strings.Contains(diff, "keep.go") {
		t.Errorf("GetStagedDiff() should contain changes to keep.go, got:\n%s", diff)
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
	defer func() { _ = os.Chdir(origDir) }()

	ctx := context.Background()
	got, err := GetRecentCommits(ctx, 5)
	if got != "" {
		t.Errorf("GetRecentCommits(5) on empty repo = %q, want empty string", got)
	}
	if err == nil {
		t.Errorf("GetRecentCommits(5) on empty repo should return an error")
	}
}
