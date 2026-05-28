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

// GetRecentCommits returns the last n commit summaries (one-line format).
// If n <= 0, it returns an empty string. If the repository has no commits yet
// or git log fails for any other reason, it returns an empty string rather than
// an error — missing history should not block the caller.
func GetRecentCommits(ctx context.Context, n int) string {
	if n <= 0 {
		return ""
	}
	cmd := exec.CommandContext(ctx, "git", "log", fmt.Sprintf("-%d", n), "--oneline", "--no-decorate")
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return ""
	}

	return out.String()
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
