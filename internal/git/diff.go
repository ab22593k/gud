// Package git provides a thin wrapper around the git CLI for reading staged
// diffs, resolving history, rewriting commits, and installing git hooks.
package git

import (
	"context"
	"fmt"
	"strings"
)

// MaxRecentCommits is the maximum number of recent commits GetRecentCommits can
// request. This prevents accidentally dumping hundreds of commits into the prompt
// and wasting tokens.
const MaxRecentCommits = 50

// MaxDiffBytes bounds the staged diff held in memory and sent to the model.
// The staged diff is repository-controlled: without a cap a single large
// staged file inflates the heap (~4x the diff) and the API payload with no
// ceiling. The cap is enforced during the subprocess read (runGitCapped), not
// after, so the heap never holds the full output.
const MaxDiffBytes = 256 << 10

// flagOneline is git's one-line-per-commit log format flag.
const flagOneline = "--oneline"

// cmdLog is the git log subcommand name.
const cmdLog = "log"

// Commit runs git commit with the given message piped via stdin.
// It returns the commit hash (abbreviated) on success.
func Commit(ctx context.Context, message string) (string, error) {
	if _, err := runGitStdin(ctx, message, "commit", "-F", "-"); err != nil {
		return "", fmt.Errorf("git commit failed: %w", err)
	}

	return getHEADHash(ctx)
}

// GetRepoRoot returns the absolute path to the git repository root.
func GetRepoRoot(ctx context.Context) (string, error) {
	return runGitTrimmed(ctx, "rev-parse", "--show-toplevel")
}

// GetBranch returns the current git branch name, or empty string on detached
// HEAD or error. Callers should handle the empty result gracefully.
func GetBranch(ctx context.Context) string {
	branch := runGitQuietly(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "HEAD" {
		return "" // detached HEAD — no named branch
	}

	return branch
}

// getHEADHash returns the abbreviated hash of HEAD.
func getHEADHash(ctx context.Context) (string, error) {
	return runGitTrimmed(ctx, "rev-parse", "--short", "HEAD")
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

	return runGit(ctx, cmdLog, fmt.Sprintf("-%d", n), flagOneline, "--no-decorate")
}

// StagedChanges bundles the staged diff, a list of deleted file names, and
// whether the diff was cut at MaxDiffBytes. Deleted names come from the diff
// body on the fast path; when truncated they are re-listed by name so the
// caller still sees every deletion.
type StagedChanges struct {
	Diff      string
	Deleted   []string
	Truncated bool
}

// GetStagedChanges runs `git diff --cached` capped at MaxDiffBytes and
// returns the diff content plus the deleted file names. A truncation marker
// is appended to cut diffs so the model knows its input is partial.
func GetStagedChanges(ctx context.Context) (*StagedChanges, error) {
	diff, truncated, err := fetchStagedDiff(ctx)
	if err != nil {
		return nil, err
	}

	deleted := extractDeletedFiles(diff)
	if truncated {
		deleted = fetchDeletedNames(ctx, deleted)
	}

	return &StagedChanges{
		Diff:      diff,
		Deleted:   deleted,
		Truncated: truncated,
	}, nil
}

// fetchStagedDiff returns the staged diff capped at MaxDiffBytes. The boolean
// reports whether the diff was cut, in which case a marker is appended.
func fetchStagedDiff(ctx context.Context) (string, bool, error) {
	diff, truncated, err := runGitCapped(ctx, MaxDiffBytes, "diff", "--cached")
	if err != nil {
		return "", false, fmt.Errorf("failed to get staged changes: %w", err)
	}

	if truncated {
		diff += diffTruncatedNotice()
	}

	return diff, truncated, nil
}

// diffTruncatedNotice marks a cut diff so the model knows its input is
// partial instead of mistaking the cutoff for the end of the changes.
func diffTruncatedNotice() string {
	return fmt.Sprintf("\n[diff truncated by gud at %d bytes]\n", MaxDiffBytes)
}

// fetchDeletedNames lists staged deletions by name. It backs GetStagedChanges
// when the diff body was truncated, since the cutoff may have dropped the
// deletion entries the name parser reads.
func fetchDeletedNames(ctx context.Context, fallback []string) []string {
	out, _, err := runGitCapped(ctx, MaxDiffBytes, "diff", "--cached", "--name-only", "--diff-filter=D")
	if err != nil {
		return fallback
	}

	return splitNonEmptyLines(out)
}

// splitNonEmptyLines splits s on newlines, dropping blank entries.
func splitNonEmptyLines(s string) []string {
	var lines []string

	for line := range strings.SplitSeq(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}

// splitDiffEntries splits a multi-file diff into per-file entries.
func splitDiffEntries(diff string) []string {
	entries := strings.Split(diff, "\ndiff --git ")

	var result []string

	for i, e := range entries {
		if i == 0 {
			if strings.TrimSpace(e) != "" {
				result = append(result, e)
			}
		} else {
			result = append(result, "diff --git "+e)
		}
	}

	return result
}

// extractFilePath extracts the "+++ b/..." path from a diff entry.
func extractFilePath(entry string) string {
	for line := range strings.SplitSeq(entry, "\n") {
		if after, ok := strings.CutPrefix(line, "+++ b/"); ok {
			return after
		}
	}

	for line := range strings.SplitSeq(entry, "\n") {
		if after, ok := strings.CutPrefix(line, "--- a/"); ok {
			return after
		}
	}

	return ""
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
			if after, ok := strings.CutPrefix(prev, "--- a/"); ok {
				deleted = append(deleted, after)
			}
		}
	}

	return deleted
}
