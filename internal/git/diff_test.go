package git

import (
	"context"
	"os"
	"os/exec"
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
			got := GetRecentCommits(ctx, tt.n)
			if got != "" {
				t.Errorf("GetRecentCommits(%d) = %q, want empty string", tt.n, got)
			}
		})
	}
}

func TestGetRecentCommits_NonEmpty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	got := GetRecentCommits(ctx, 5)
	if got == "" {
		t.Fatal("GetRecentCommits(5) returned empty, expected at least one commit")
	}
	t.Logf("Recent commits:\n%s", got)
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
	got := GetRecentCommits(ctx, 5)
	if got != "" {
		t.Errorf("GetRecentCommits(5) on empty repo = %q, want empty string", got)
	}
}
