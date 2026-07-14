package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
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
// It returns the commit hash (abbreviated) on success.
func Commit(ctx context.Context, message string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "commit", "-F", "-")
	cmd.Stdin = bytes.NewBufferString(message)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git commit failed: %w\n%s", err, out.String())
	}

	return getHEADHash(ctx)
}

// GetAuthor returns the git user name in "Name <email>" format.
// On error, it returns an empty string — callers should handle gracefully.
func GetAuthor(ctx context.Context) string {
	name, err := runGitConfig(ctx, "user.name")
	if err != nil {
		return ""
	}
	email, err := runGitConfig(ctx, "user.email")
	if err != nil {
		return strings.TrimSpace(name)
	}

	return strings.TrimSpace(name) + " <" + strings.TrimSpace(email) + ">"
}

// runGitConfig runs git config --get <key> and returns the value.
func runGitConfig(ctx context.Context, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "config", "--get", key)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(out), nil
}

// GetRepoRoot returns the absolute path to the git repository root.
func GetRepoRoot(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get repo root: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// getHEADHash returns the abbreviated hash of HEAD.
func getHEADHash(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get head hash: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
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

// StagedChanges bundles the full staged diff and a list of deleted file names,
// all retrieved from a single git subprocess call.
type StagedChanges struct {
	Diff    string
	Deleted []string
}

// GetStagedChanges runs a single `git diff --cached` subprocess (without any
// diff-filter) and returns both the full diff content and a list of deleted
// file names parsed from the output. Using a single subprocess instead of two
// (GetStagedDiff + GetStagedDeletedFiles) reduces subprocess overhead.
func GetStagedChanges(ctx context.Context) (*StagedChanges, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached")
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get staged changes: %w", err)
	}

	diff := out.String()
	return &StagedChanges{
		Diff:    diff,
		Deleted: extractDeletedFiles(diff),
	}, nil
}

// extractDeletedFiles parses the output of `git diff --cached` and returns the
// names of files that were deleted (indicated by "+++ /dev/null").
func extractDeletedFiles(diff string) []string {
	var deleted []string
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		// A deleted file has the form:
		//   --- a/path/to/file
		//   +++ /dev/null
		if strings.HasPrefix(line, "+++ /dev/null") && i > 0 {
			prev := lines[i-1]
			if strings.HasPrefix(prev, "--- a/") {
				deleted = append(deleted, strings.TrimPrefix(prev, "--- a/"))
			}
		}
	}
	return deleted
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
