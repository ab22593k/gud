package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// GetStagedDiff returns the git diff of staged changes (git diff --cached).
func GetStagedDiff(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--", repoPath)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get staged diff: %w", err)
	}

	return out.String(), nil
}

// GetUnstagedDiff returns the git diff of unstaged changes (git diff).
func GetUnstagedDiff(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--", repoPath)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get unstaged diff: %w", err)
	}

	return out.String(), nil
}
