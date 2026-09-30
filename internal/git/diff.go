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

// flagOneline is git's one-line-per-commit log format flag.
const flagOneline = "--oneline"

// cmdLog is the git log subcommand name.
const cmdLog = "log"

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
//
// The returned string is the Helix tenant key (repo_path/tenantId): callers
// must persist and query with this exact string. A rename, move, or
// symlink alias of the checkout produces a different key and therefore a
// separate tenant with no access to prior memory.
func GetRepoRoot(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get repo root: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// GetBranch returns the current git branch name, or empty string on detached
// HEAD or error. Callers should handle the empty result gracefully.
func GetBranch(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}

	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		return "" // detached HEAD — no named branch
	}

	return branch
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

	cmd := exec.CommandContext(ctx, "git", cmdLog, fmt.Sprintf("-%d", n), flagOneline, "--no-decorate")

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
			if after, ok := strings.CutPrefix(prev, "--- a/"); ok {
				deleted = append(deleted, after)
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

// RenamedFile is a staged rename old→new path pair whose content hunks are
// excluded from the prompt by default.
type RenamedFile struct {
	OldPath string
	NewPath string
}

// FilterRemovedContent splits a unified diff (`git diff --cached` or
// `git show --patch` output) into per-file blocks and, unless include is
// true, drops the content blocks of deleted files and renames. It returns
// the kept diff and, in diff order, the removed references: deleted paths
// and rename old→new pairs. Removed references are reported in both modes
// so callers can name removed paths even when their content is included;
// every dropped block yields exactly one reference. With include=true the
// kept diff is the input unchanged.
func FilterRemovedContent(diff string, include bool) (kept string, deleted []string, renamed []RenamedFile) {
	if diff == "" {
		return "", nil, nil
	}

	blocks := splitDiffBlocks(diff)

	var keptBlocks []string

	for _, b := range blocks {
		if from, to, ok := parseRenameBlock(b); ok {
			renamed = append(renamed, RenamedFile{OldPath: from, NewPath: to})

			if include {
				keptBlocks = append(keptBlocks, b)
			}

			continue
		}

		if path, ok := parseDeletedBlock(b); ok {
			deleted = append(deleted, path)

			if include {
				keptBlocks = append(keptBlocks, b)
			}

			continue
		}

		keptBlocks = append(keptBlocks, b)
	}

	if len(deleted) == 0 && len(renamed) == 0 {
		return diff, nil, nil
	}

	if include {
		return diff, deleted, renamed
	}

	return joinDiffBlocks(blocks, keptBlocks), deleted, renamed
}

// splitDiffBlocks splits diff at `diff --git ` boundaries, preserving any
// preamble (e.g. `git show` headers) as the first block. Diffs without
// `diff --git ` headers (bare `---`/`+++` fixtures) form a single block.
func splitDiffBlocks(diff string) []string {
	const header = "diff --git "

	var blocks []string

	start := 0

	for {
		idx := indexLinePrefix(diff, header, start)
		if idx < 0 {
			break
		}

		if idx > start {
			blocks = append(blocks, diff[start:idx])
		}

		start = idx

		next := indexLinePrefix(diff, header, idx+len(header))
		if next < 0 {
			blocks = append(blocks, diff[start:])

			return blocks
		}

		blocks = append(blocks, diff[start:next])
		start = next
	}

	if len(blocks) == 0 {
		return []string{diff}
	}

	return blocks
}

// joinDiffBlocks rebuilds the diff from kept blocks. Blocks carry their
// original separators, so joining is plain concatenation.
func joinDiffBlocks(all, kept []string) string {
	keep := make(map[string]int, len(kept))
	for _, b := range kept {
		keep[b]++
	}

	var out strings.Builder

	for _, b := range all {
		if keep[b] > 0 {
			keep[b]--

			out.WriteString(b)
		}
	}

	return out.String()
}

// indexLinePrefix returns the byte index of the first line at or after start
// beginning with prefix, or -1. Matching is line-anchored so hunk bodies
// mentioning the prefix mid-line are ignored.
func indexLinePrefix(s, prefix string, start int) int {
	for i := start; i < len(s); {
		lineStart := i
		if lineStart == 0 || s[lineStart-1] == '\n' {
			if strings.HasPrefix(s[lineStart:], prefix) {
				return lineStart
			}
		}

		next := strings.IndexByte(s[i:], '\n')
		if next < 0 {
			return -1
		}

		i += next + 1
	}

	return -1
}

// parseRenameBlock reports whether block is a rename (has `rename from/to`
// headers) and returns the old and new paths.
func parseRenameBlock(block string) (from, to string, ok bool) {
	lines := strings.Split(block, "\n")

	for _, line := range lines {
		if after, found := strings.CutPrefix(line, "rename from "); found {
			from = strings.TrimSpace(after)
		}

		if after, found := strings.CutPrefix(line, "rename to "); found {
			to = strings.TrimSpace(after)
		}
	}

	if from == "" || to == "" {
		return "", "", false
	}

	return from, to, true
}

// parseDeletedBlock reports whether block deletes a file and returns its
// path. Detection covers text deletions (`+++ /dev/null`, `deleted file
// mode`) and binary deletions (`Binary files ... and /dev/null differ`).
func parseDeletedBlock(block string) (string, bool) {
	lines := strings.Split(block, "\n")

	for i, line := range lines {
		if strings.HasPrefix(line, "+++ /dev/null") && i > 0 {
			if after, ok := strings.CutPrefix(lines[i-1], "--- a/"); ok {
				return strings.TrimSpace(after), true
			}
		}

		if strings.HasPrefix(line, "deleted file mode ") {
			if path := deletedBlockPath(block); path != "" {
				return path, true
			}
		}

		if strings.HasPrefix(line, "Binary files ") && strings.HasSuffix(line, " and /dev/null differ") {
			if path := deletedBlockPath(block); path != "" {
				return path, true
			}
		}
	}

	return "", false
}

// deletedBlockPath resolves a deleted file's path from its block, preferring
// the `--- a/<path>` line and falling back to the `diff --git a/<p> b/<q>`
// header's old path.
func deletedBlockPath(block string) string {
	lines := strings.Split(block, "\n")

	for i, line := range lines {
		if strings.HasPrefix(line, "+++ /dev/null") && i > 0 {
			if after, ok := strings.CutPrefix(lines[i-1], "--- a/"); ok {
				return strings.TrimSpace(after)
			}
		}
	}

	for _, line := range lines {
		if after, ok := strings.CutPrefix(line, "diff --git a/"); ok {
			if cut, _, found := strings.Cut(after, " b/"); found {
				return strings.TrimSpace(cut)
			}

			return strings.TrimSpace(after)
		}
	}

	return ""
}
