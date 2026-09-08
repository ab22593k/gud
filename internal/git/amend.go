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

// ResolveRevision resolves rev (e.g. "HEAD~2") to a full commit SHA.
func ResolveRevision(ctx context.Context, rev string) (string, error) {
	if strings.TrimSpace(rev) == "" {
		return "", fmt.Errorf("resolve revision: empty revision")
	}
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", rev+"^{commit}")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("resolve revision %q: %w", rev, err)
	}
	return strings.TrimSpace(out.String()), nil
}

// GetCommitDiff returns the patch of exactly one commit, never a range.
func GetCommitDiff(ctx context.Context, sha string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "show", "--format=", "--patch",
		"--no-decorate", "--no-ext-diff", sha, "--")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("show commit %s: %w", sha, err)
	}
	return out.String(), nil
}

// GetCommitMessage returns the full message body of one commit.
func GetCommitMessage(ctx context.Context, sha string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "-1", "--format=%B", sha)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("read message of %s: %w", sha, err)
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

// IsMergeCommit reports whether sha has more than one parent.
func IsMergeCommit(ctx context.Context, sha string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--parents", "-n", "1", sha)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("parents of %s: %w", sha, err)
	}
	return len(strings.Fields(out.String())) > 2, nil
}

// IsCleanTree reports whether there are no staged or unstaged tracked changes.
// Untracked files are ignored so a scratch file cannot block a reword.
func IsCleanTree(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain", "--untracked-files=no")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("status: %w", err)
	}
	return strings.TrimSpace(out.String()) == "", nil
}

// AmendHead replaces the HEAD message, returning the new short hash.
func AmendHead(ctx context.Context, message string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "commit", "--amend", "-F", "-")
	cmd.Stdin = bytes.NewBufferString(message)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git commit --amend failed: %w\n%s", err, out.String())
	}
	return getHEADHash(ctx)
}

// checkHasParent errors when sha is the root commit, which has no parent
// to anchor an interactive rebase on.
func checkHasParent(ctx context.Context, sha string) error {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", sha+"^")
	if err := cmd.Run(); err != nil {
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
	if err := os.WriteFile(seqPath, []byte(seqScript), 0700); err != nil {
		return fmt.Errorf("reword sequence editor: %w", err)
	}
	edPath := filepath.Join(dir, "editor.sh")
	edScript := "#!/bin/sh\ncat \"$GUD_NEWMSG\" > \"$1\"\n"
	if err := os.WriteFile(edPath, []byte(edScript), 0700); err != nil {
		return fmt.Errorf("reword editor: %w", err)
	}
	//nolint:gosec // G204: fixed "git" binary; sha was resolved via rev-parse.
	cmd := exec.CommandContext(ctx, "git", "rebase", "-i", sha+"^")
	cmd.Env = append(os.Environ(),
		"GIT_SEQUENCE_EDITOR="+seqPath,
		"GIT_EDITOR="+edPath,
		"GUD_NEWMSG="+msgPath,
		"GIT_TERMINAL_PROMPT=0")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		abort := exec.CommandContext(context.Background(), "git", "rebase", "--abort")
		_, _ = abort.Output()
		return fmt.Errorf("reword %s: %w\n%s", sha, err, out.String())
	}
	return nil
}
