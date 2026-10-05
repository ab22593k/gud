package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ResolveRevision resolves rev (e.g. "HEAD~2") to a full commit SHA.
func ResolveRevision(ctx context.Context, rev string) (string, error) {
	if strings.TrimSpace(rev) == "" {
		return "", errors.New("resolve revision: empty revision")
	}

	return runGitTrimmed(ctx, "rev-parse", "--verify", rev+"^{commit}")
}

// GetCommitDiff returns the patch of exactly one commit, never a range.
func GetCommitDiff(ctx context.Context, sha string) (string, error) {
	return runGit(ctx, "show", "--format=", "--patch",
		"--no-decorate", "--no-ext-diff", sha, "--")
}

// GetCommitMessage returns the full message body of one commit.
func GetCommitMessage(ctx context.Context, sha string) (string, error) {
	out, err := runGit(ctx, cmdLog, "-1", "--format=%B", sha)
	if err != nil {
		return "", err
	}

	return strings.TrimRight(out, "\n"), nil
}

// IsMergeCommit reports whether sha has more than one parent.
func IsMergeCommit(ctx context.Context, sha string) (bool, error) {
	out, err := runGit(ctx, "rev-list", "--parents", "-n", "1", sha)
	if err != nil {
		return false, err
	}

	return len(strings.Fields(out)) > 2, nil
}

// IsCleanTree reports whether there are no staged or unstaged tracked changes.
// Untracked files are ignored so a scratch file cannot block a reword.
func IsCleanTree(ctx context.Context) (bool, error) {
	out, err := runGit(ctx, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(out) == "", nil
}

// AmendHead replaces the HEAD message, returning the new short hash.
func AmendHead(ctx context.Context, message string) (string, error) {
	if _, err := runGitStdin(ctx, message, "commit", "--amend", "-F", "-"); err != nil {
		return "", fmt.Errorf("git commit --amend failed: %w", err)
	}

	return getHEADHash(ctx)
}

// checkHasParent errors when sha is the root commit, which has no parent
// to anchor an interactive rebase on.
func checkHasParent(ctx context.Context, sha string) error {
	if _, err := runGit(ctx, "rev-parse", "--verify", sha+"^"); err != nil {
		return fmt.Errorf("reword %s: cannot reword the root commit: %w", sha, err)
	}

	return nil
}

// RewordCommit rewrites the message of a non-HEAD, non-root, non-merge commit
// via a scripted interactive rebase. The todo's first line is the target
// (rebase -i sha^ lists it first), so the sequence editor only touches line 1.
// On any rebase failure it aborts the rebase before returning the error.
func RewordCommit(ctx context.Context, sha, message string) error {
	if err := checkHasParent(ctx, sha); err != nil {
		return err
	}

	merge, err := IsMergeCommit(ctx, sha)
	if err != nil {
		return err
	}

	if merge {
		return fmt.Errorf("reword %s: merge commits are not supported", sha)
	}

	dir, err := os.MkdirTemp("", "gud-reword-*")
	if err != nil {
		return fmt.Errorf("reword temp dir: %w", err)
	}

	defer func() { _ = os.RemoveAll(dir) }()

	msgPath := filepath.Join(dir, "newmsg")

	if err := os.WriteFile(msgPath, []byte(message), 0600); err != nil {
		return fmt.Errorf("reword message file: %w", err)
	}

	seqPath := filepath.Join(dir, "seq-editor.sh")
	seqScript := "#!/bin/sh\ntmp=\"$1.tmp\"\nsed '1s/^pick /reword /' \"$1\" > \"$tmp\" && mv \"$tmp\" \"$1\"\n"

	if err := os.WriteFile(seqPath, []byte(seqScript), 0600); err != nil {
		return fmt.Errorf("reword sequence editor: %w", err)
	}

	edPath := filepath.Join(dir, "editor.sh")
	edScript := "#!/bin/sh\ncat \"$GUD_NEWMSG\" > \"$1\"\n"

	if err := os.WriteFile(edPath, []byte(edScript), 0600); err != nil {
		return fmt.Errorf("reword editor: %w", err)
	}

	cmd := newGitCmd(ctx, "rebase", "-i", sha+"^")

	cmd.Env = append(os.Environ(),
		"GIT_SEQUENCE_EDITOR=sh "+seqPath,
		"GIT_EDITOR=sh "+edPath,
		"GUD_NEWMSG="+msgPath,
		"GIT_TERMINAL_PROMPT=0")

	if err := runRebase(ctx, cmd, sha); err != nil {
		return err
	}

	return nil
}

// runRebase runs the interactive rebase and, on failure, aborts it before
// returning so the repository is never left mid-rebase. The abort deliberately
// ignores the caller's context: it must still run when the caller's context is
// already cancelled, which is the usual reason the rebase failed.
func runRebase(ctx context.Context, cmd *exec.Cmd, sha string) error {
	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		//nolint:contextcheck // intentional Background use: the abort must
		// survive a cancelled caller context to leave the repo clean.
		abort := newGitCmd(context.Background(), "rebase", "--abort")
		_, _ = abort.Output()

		return fmt.Errorf("reword %s: %w\n%s", sha, err, out.String())
	}

	return nil
}
