package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// MaxRecentCommits is the maximum number of recent commits GetRecentCommits can
// request. This prevents accidentally dumping hundreds of commits into the prompt
// and wasting tokens.
const MaxRecentCommits = 50

// GetStagedDiff returns the git diff of staged changes, excluding deleted and renamed file content.
func GetStagedDiff(ctx context.Context) (string, error) {
	return runGitDiff(ctx, "diff", "--cached", "--diff-filter=dr")
}

// GetUnstagedDiff returns the git diff of unstaged changes, excluding deleted and renamed file content.
func GetUnstagedDiff(ctx context.Context) (string, error) {
	return runGitDiff(ctx, "diff", "--diff-filter=dr")
}

// GetStagedDeletedFiles returns the names of files deleted in staged changes (no content).
func GetStagedDeletedFiles(ctx context.Context) (string, error) {
	return runGitDiff(ctx, "diff", "--cached", "--diff-filter=D", "--name-only")
}

// Commit runs git commit with the given message piped via stdin.
func Commit(ctx context.Context, message string) error {
	cmd := exec.CommandContext(ctx, "git", "commit", "-F", "-")
	cmd.Stdin = bytes.NewBufferString(message)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git commit failed: %w\n%s", err, out.String())
	}

	return nil
}

// GetRecentCommits returns the last n commit summaries (one-line format).
// If n <= 0, it returns an empty string with no error.
// It returns an error if git log fails (e.g. the repository has no commits yet).
// n is capped at MaxRecentCommits to prevent excessive git log queries.
func GetRecentCommits(ctx context.Context, n int) (string, error) {
	if n <= 0 {
		return "", nil
	}
	if n > MaxRecentCommits {
		n = MaxRecentCommits
	}
	cmd := exec.CommandContext(ctx, "git", "log", fmt.Sprintf("-%d", n), "--oneline", "--no-decorate")
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get recent commits: %w", err)
	}

	return out.String(), nil
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
