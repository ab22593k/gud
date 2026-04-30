package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// GetStagedDiff returns the git diff of staged changes (git diff --cached).
func GetStagedDiff(ctx context.Context) (string, error) {
	return runGitDiff(ctx, "diff", "--cached")
}

// GetUnstagedDiff returns the git diff of unstaged changes (git diff).
func GetUnstagedDiff(ctx context.Context) (string, error) {
	return runGitDiff(ctx, "diff")
}

// runGitDiff runs a git diff command with the given arguments and returns the output.
// It is the single point of implementation for git diff operations in this package.
func runGitDiff(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get diff: %w", err)
	}

	return out.String(), nil
}
