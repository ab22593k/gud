package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Operation identifies the in-progress git operation that the next commit
// completes. OperationNone means an ordinary commit.
type Operation string

const (
	OperationNone       Operation = ""            // ordinary commit from staged changes
	OperationMerge      Operation = "merge"       // completing an in-progress merge
	OperationCherryPick Operation = "cherry-pick" // completing a cherry-pick stop
	OperationRevert     Operation = "revert"      // completing a revert stop
	OperationRebase     Operation = "rebase"      // continuing a rebase stop (pick/reword/edit/conflict)
	OperationSquash     Operation = "squash"      // interactive-rebase squash stop
	OperationFixup      Operation = "fixup"       // interactive-rebase fixup stop
)

// DetectOperation inspects git state files to determine the operation the
// current commit completes. It is worktree aware (state files live in the
// worktree's own git dir) and never errors: any failure to read git state
// degrades to OperationNone so normal generation is unaffected.
func DetectOperation(ctx context.Context) Operation {
	dir, err := gitDir(ctx)
	if err != nil {
		return OperationNone
	}

	if fileExists(filepath.Join(dir, "MERGE_HEAD")) {
		return OperationMerge
	}
	if fileExists(filepath.Join(dir, "CHERRY_PICK_HEAD")) {
		return OperationCherryPick
	}
	if fileExists(filepath.Join(dir, "REVERT_HEAD")) {
		return OperationRevert
	}
	if dirExists(filepath.Join(dir, "rebase-merge")) {
		return rebaseOperation(filepath.Join(dir, "rebase-merge"), filepath.Join(dir, "SQUASH_MSG"))
	}
	if dirExists(filepath.Join(dir, "rebase-apply")) {
		return OperationRebase
	}

	return OperationNone
}

// rebaseOperation classifies an interactive-rebase stop. When git stops it
// moves the command being processed into the `done` file, so the last command
// word distinguishes a squash or fixup stop from an ordinary pick/reword/edit
// stop. SQUASH_MSG is a fallback signal: git writes it during squash stops.
func rebaseOperation(rebaseDir, squashMsgPath string) Operation {
	if cmd := currentRebaseCommand(rebaseDir); cmd != "" {
		switch {
		case strings.HasPrefix(cmd, "squash"):
			return OperationSquash
		case strings.HasPrefix(cmd, "fixup"):
			return OperationFixup
		}
	}
	if fileExists(squashMsgPath) {
		return OperationSquash
	}

	return OperationRebase
}

// currentRebaseCommand returns the command word ("pick", "squash", "fixup",
// "reword", ...) of the rebase command currently being processed, or "" if it
// cannot be determined. It reads the last non-comment command of the `done`
// file first; when that is unavailable it falls back to the first command of
// the remaining `git-rebase-todo`.
func currentRebaseCommand(rebaseDir string) string {
	if cmd := lastDoneCommand(rebaseDir); cmd != "" {
		return cmd
	}

	data, err := os.ReadFile(filepath.Join(rebaseDir, "git-rebase-todo"))
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if fields := strings.Fields(trimmed); len(fields) > 0 {
			return fields[0]
		}
	}

	return ""
}

// lastDoneCommand returns the command word of the last non-comment command in
// the interactive-rebase `done` file, or "".
func lastDoneCommand(rebaseDir string) string {
	data, err := os.ReadFile(filepath.Join(rebaseDir, "done"))
	if err != nil {
		return ""
	}

	last := ""
	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		last = trimmed
	}
	if last == "" {
		return ""
	}
	if fields := strings.Fields(last); len(fields) > 0 {
		return fields[0]
	}

	return ""
}

// PreparedMessage returns the commit message git has already prepared for the
// in-progress operation — the message git would show in the editor when the
// stop is completed — or "" when none can be determined. It is best-effort and
// never errors; callers treat "" as "fall back to normal generation".
//
// For merge and squash stops git writes dedicated state files (MERGE_MSG,
// SQUASH_MSG). For cherry-pick, revert and rebase stops the intended message
// is derived from the commit recorded in the corresponding *HEAD file, since
// COMMIT_EDITMSG is not populated until the stop is actually committed.
func PreparedMessage(ctx context.Context, op Operation) string {
	dir, err := gitDir(ctx)
	if err != nil {
		return ""
	}

	switch op {
	case OperationMerge:
		return stripCommentLines(readTrimmed(filepath.Join(dir, "MERGE_MSG")))

	case OperationSquash:
		return stripCommentLines(readTrimmed(filepath.Join(dir, "SQUASH_MSG")))

	case OperationCherryPick:
		return commitMessage(ctx, "CHERRY_PICK_HEAD")

	case OperationRevert:
		return revertMessage(ctx)

	case OperationRebase, OperationFixup:
		return rebaseMessage(ctx, dir)
	}

	return ""
}

// commitMessage returns the full message (%B) of the given revision, or "".
func commitMessage(ctx context.Context, rev string) string {
	return runGitOutput(ctx, "log", "-1", "--format=%B", rev)
}

// revertMessage reconstructs git's default revert message:
//
//	Revert "<subject>"
//
//	This reverts commit <full sha>.
func revertMessage(ctx context.Context) string {
	sha := runGitOutput(ctx, "rev-parse", "REVERT_HEAD")
	subject := runGitOutput(ctx, "log", "-1", "--format=%s", "REVERT_HEAD")
	if sha == "" || subject == "" {
		return ""
	}

	return strings.TrimSpace(fmt.Sprintf("Revert %q\n\nThis reverts commit %s.\n", subject, sha))
}

// rebaseMessage returns the message of the commit being replayed at a rebase
// stop. Interactive and merge-backend rebases record the commit in REBASE_HEAD;
// the am backend records it in rebase-apply/original-commit.
func rebaseMessage(ctx context.Context, dir string) string {
	if ref := readTrimmed(filepath.Join(dir, "REBASE_HEAD")); ref != "" {
		if msg := commitMessage(ctx, ref); msg != "" {
			return msg
		}
	}
	if ref := readTrimmed(filepath.Join(dir, "rebase-apply", "original-commit")); ref != "" {
		return commitMessage(ctx, ref)
	}

	return ""
}

// gitDir returns the absolute path of the current git directory, worktree
// aware: the same base git uses for MERGE_HEAD, CHERRY_PICK_HEAD, rebase-merge
// and friends, so per-worktree state is read from the right place.
func gitDir(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--absolute-git-dir").Output()
	if err != nil {
		return "", fmt.Errorf("get git dir: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// runGitOutput runs a git command and returns its trimmed stdout, or "" on
// error. It is used for read-only queries whose failure should degrade
// gracefully.
func runGitOutput(ctx context.Context, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", args...)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return ""
	}

	return strings.TrimSpace(out.String())
}

// stripCommentLines removes git comment lines (a leading # after trimming)
// from a prepared message, mirroring the cleanup git applies when a commit is
// finalised. It is used for MERGE_MSG and SQUASH_MSG, whose templates
// interleave the actual message with #-prefixed explanatory comments.
func stripCommentLines(s string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

func readTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)

	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.IsDir()
}
