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

	if strings.HasPrefix(rev, "-") {
		return "", errors.New("resolve revision: revision must not start with -")
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

// IsRootCommit reports whether sha is the root commit: it has no parent to
// anchor an interactive rebase or an amend interaction on. The revision is
// expected to resolve (callers resolve first); a failed parent probe then
// means the root commit rather than a bad revision.
func IsRootCommit(ctx context.Context, sha string) (bool, error) {
	if _, err := runGit(ctx, "rev-parse", "--verify", sha+"^"); err != nil {
		return true, nil
	}

	return false, nil
}

// checkHasParent errors when sha is the root commit, which has no parent
// to anchor an interactive rebase on.
func checkHasParent(ctx context.Context, sha string) error {
	root, err := IsRootCommit(ctx, sha)
	if err != nil {
		return err
	}

	if root {
		return fmt.Errorf("reword %s: cannot reword the root commit", sha)
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

	cmd.Env = overrideEnv(os.Environ(),
		"GIT_SEQUENCE_EDITOR=sh "+seqPath,
		"GIT_EDITOR=sh "+edPath,
		"GUD_NEWMSG="+msgPath,
		"GIT_TERMINAL_PROMPT=0")

	if err := runRebase(ctx, cmd, sha); err != nil {
		return err
	}

	return nil
}

// overrideEnv returns base with the keys in overrides replaced: existing
// entries for those keys are dropped before appending. Without filtering,
// append(os.Environ(), ...) duplicates a key the user already has set (e.g.
// GIT_SEQUENCE_EDITOR), and libc getenv in the git grandchild returns the
// FIRST match — the user's value would win over ours. Entries without "="
// carry no key and are always kept.
func overrideEnv(base []string, overrides ...string) []string {
	keys := make(map[string]bool, len(overrides))
	for _, o := range overrides {
		if k, _, ok := strings.Cut(o, "="); ok {
			keys[k] = true
		}
	}

	out := make([]string, 0, len(base)+len(overrides))

	for _, e := range base {
		if k, _, ok := strings.Cut(e, "="); ok && keys[k] {
			continue
		}

		out = append(out, e)
	}

	return append(out, overrides...)
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
		if abortOut, abortErr := abort.Output(); abortErr != nil {
			return fmt.Errorf("reword %s: %w\n%s (abort: %v: %s)", sha, err, out.String(), abortErr, abortOut)
		}

		return fmt.Errorf("reword %s: %w\n%s", sha, err, out.String())
	}

	return nil
}
