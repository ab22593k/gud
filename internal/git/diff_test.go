package git

import (
	"context"
	"testing"
)

func TestGetStagedDiff(t *testing.T) {
	ctx := context.Background()
	_, err := GetStagedDiff(ctx, ".")

	if err != nil {
		t.Errorf("GetStagedDiff() error = %v, want nil", err)
	}
}

func TestGetStagedDiff_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	diff, err := GetStagedDiff(ctx, ".")

	if err != nil {
		t.Fatalf("GetStagedDiff() unexpected error: %v", err)
	}

	t.Logf("Staged diff output:\n%s", diff)
}

func TestGetUnstagedDiff(t *testing.T) {
	ctx := context.Background()
	_, err := GetUnstagedDiff(ctx, ".")

	if err != nil {
		t.Errorf("GetUnstagedDiff() error = %v, want nil", err)
	}
}

func TestGetUnstagedDiff_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	diff, err := GetUnstagedDiff(ctx, ".")

	if err != nil {
		t.Fatalf("GetUnstagedDiff() unexpected error: %v", err)
	}

	t.Logf("Unstaged diff output:\n%s", diff)
}
