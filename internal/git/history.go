package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GetUpstreamBranch returns the configured upstream of the current branch
// (e.g. "origin/main"), or "" when the branch has no upstream or HEAD is
// detached. Callers fall back to recent-commit history when it is empty.
func GetUpstreamBranch(ctx context.Context) string {
	return runGitOutput(ctx, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
}

// GetTopicHistory returns one-line summaries of commits on the current branch
// since it diverged from upstream — the range merge-base(HEAD, upstream)..HEAD
// — capped at n entries. When paths is non-empty the log is limited to commits
// touching those files, so the history mirrors the staged scope instead of the
// whole repository. It returns an error when the upstream cannot be resolved
// or has no merge base with HEAD.
func GetTopicHistory(ctx context.Context, upstream string, n int, paths []string) (string, error) {
	if n <= 0 || upstream == "" {
		return "", nil
	}

	if n > MaxRecentCommits {
		n = MaxRecentCommits
	}

	root, err := GetRepoRoot(ctx)
	if err != nil {
		return "", fmt.Errorf("get repo root: %w", err)
	}

	// Run the log from the repo root so pathspecs are interpreted relative to
	// the repository, matching the root-relative paths of `git diff --cached`.
	mergeBase, err := runGitDir(ctx, root, "merge-base", "HEAD", upstream)
	if err != nil || strings.TrimSpace(mergeBase) == "" {
		return "", fmt.Errorf("merge base with %s: %w", upstream, err)
	}

	args := []string{cmdLog, flagOneline, "--no-decorate", fmt.Sprintf("-%d", n), strings.TrimSpace(mergeBase) + "..HEAD"}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}

	out, err := runGitDir(ctx, root, args...)
	if err != nil {
		return "", fmt.Errorf("failed to get topic commits: %w", err)
	}

	return out, nil
}

// runGitDir runs a git command with its working directory set to dir, so
// relative arguments (such as log pathspecs) resolve against dir rather than
// the caller's cwd.
func runGitDir(ctx context.Context, dir string, args ...string) (string, error) {
	//nolint:gosec // G204: binary is the fixed "git" command; arguments come from
	// internal callers, not user input.
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var out bytes.Buffer

	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v: %w", args, err)
	}

	return strings.TrimSpace(out.String()), nil
}

// ExtractChangedPaths returns the paths of files changed in a diff, derived
// from the "+++ b/<path>" headers (the post-image path, so renames resolve to
// the new name). Deletions ("+++ /dev/null") and duplicate paths are skipped.
// It is best-effort: git-quoted paths with embedded special characters are
// unquoted only at the outer level.
func ExtractChangedPaths(diff string) []string {
	var paths []string

	seen := make(map[string]bool)

	for line := range strings.SplitSeq(diff, "\n") {
		rest, ok := strings.CutPrefix(line, "+++ ")
		if !ok {
			continue
		}
		// git wraps paths with special characters in quotes: +++ "b/my file".
		if len(rest) >= 2 && strings.HasPrefix(rest, `"`) && strings.HasSuffix(rest, `"`) {
			rest = rest[1 : len(rest)-1]
		}

		rest, ok = strings.CutPrefix(rest, "b/")
		if !ok || rest == "" || rest == "/dev/null" {
			continue
		}

		if seen[rest] {
			continue
		}

		seen[rest] = true
		paths = append(paths, rest)
	}

	return paths
}
