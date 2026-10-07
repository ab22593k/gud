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

// This file holds the git-wire transport primitives: cloning blobless
// mirrors, resolving remote refs, and exporting subfolder archives. All git
// spawning stays in this package (see exec.go); callers pass validated
// operands and a context that bounds the subprocess.
//
// Every network-capable helper disables interactive credential prompts
// (GIT_TERMINAL_PROMPT=0) so auth failures surface as errors instead of
// hanging the CLI waiting on stdin.

// ErrSubpathNotDir marks a populated worktree subset that is not a
// directory: the subpath is missing or names a file. Callers map it to
// their missing-path failure class instead of a transport failure.
var ErrSubpathNotDir = errors.New("git: subpath not a directory in commit")

// wireCmd builds a git command that never prompts for credentials.
func wireCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := newGitCmd(ctx, args...)

	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	return cmd
}

// runMirror runs git with prompt disabled, returning trimmed combined output
// on success and a diagnostics-bearing error on failure.
func runMirror(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := wireCmd(ctx, args...)

	if dir != "" {
		cmd.Dir = dir
	}

	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v: %w\n%s", args, err, out.String())
	}

	return strings.TrimSpace(out.String()), nil
}

// CloneMirror clones url as a blobless, no-checkout mirror into dir. Only
// commits and trees cross the network; file blobs arrive later on demand.
// The caller must ensure dir does not exist.
func CloneMirror(ctx context.Context, url, dir string) error {
	if _, err := runMirror(ctx, "", "clone", "--filter=blob:none", "--no-checkout", "--", url, dir); err != nil {
		return fmt.Errorf("clone mirror: %w", err)
	}

	return nil
}

// FetchMirror fetches all refs of the mirror at dir from its origin.
func FetchMirror(ctx context.Context, dir string) error {
	if _, err := runMirror(ctx, dir, "fetch", "origin"); err != nil {
		return fmt.Errorf("fetch mirror: %w", err)
	}

	return nil
}

// LsRemote lists refs as full-name → SHA for a remote URL or a local repo
// path. An empty map with a nil error means the listing succeeded but is
// empty; the caller distinguishes unknown refs from transport failures.
func LsRemote(ctx context.Context, url string) (map[string]string, error) {
	out, err := runMirror(ctx, "", "ls-remote", "--", url)
	if err != nil {
		return nil, fmt.Errorf("list remote refs: %w", err)
	}

	refs := make(map[string]string)

	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		sha, name, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok || sha == "" || name == "" {
			continue
		}

		refs[name] = sha
	}

	return refs, nil
}

// CommitExists reports whether rev resolves to an object in the repo at dir.
// A missing object surfaces as a non-zero git exit, not a Go error.
func CommitExists(ctx context.Context, dir, rev string) bool {
	return wireCmd(ctx, "-C", dir, "cat-file", "-e", rev).Run() == nil
}

// AddSparseWorktree attaches a detached, unpopulated worktree at path for
// the mirror at commit, then populates only subpath via sparse-checkout.
// The ordering is load-bearing for transfer proportionality: the worktree
// is added with --no-checkout FIRST and the sparse set applied SECOND, so
// only the requested subset ever materializes (a final checkout applies the
// patterns). Both operands are safe by construction — path is a caller-minted
// temp dir, commit a resolved hex SHA — and subpath passes behind --.
func AddSparseWorktree(ctx context.Context, mirror, path, commit, subpath string) error {
	if strings.HasPrefix(commit, "-") {
		return fmt.Errorf("refusing suspicious commit %q", commit)
	}

	if _, err := runMirror(ctx, mirror, "worktree", "add", "--detach", "--no-checkout", path, commit); err != nil {
		return fmt.Errorf("add worktree: %w", err)
	}

	if _, err := runMirror(ctx, path, "sparse-checkout", "set", "--", subpath); err != nil {
		return fmt.Errorf("set sparse checkout: %w", err)
	}

	if _, err := runMirror(ctx, path, "checkout", commit); err != nil {
		return fmt.Errorf("populate worktree: %w", err)
	}

	// A sparse set for a missing path succeeds silently, so verify the
	// population directly: the subpath must exist as a directory (a file
	// is not fetchable either). Clean up before reporting.
	st, err := os.Stat(filepath.Join(path, filepath.FromSlash(subpath)))
	if err != nil || !st.IsDir() {
		_ = RemoveWorktree(ctx, mirror, path)

		return fmt.Errorf("subpath %q not in %s as a directory: %w", subpath, commit, ErrSubpathNotDir)
	}

	return nil
}

// RemoveWorktree detaches and deletes the worktree at path.
func RemoveWorktree(ctx context.Context, mirror, path string) error {
	if _, err := runMirror(ctx, mirror, "worktree", "remove", "--force", "--", path); err != nil {
		return fmt.Errorf("remove worktree: %w", err)
	}

	return nil
}

// PruneWorktrees drops stale worktree metadata, e.g. from killed runs.
func PruneWorktrees(ctx context.Context, mirror string) error {
	if _, err := runMirror(ctx, mirror, "worktree", "prune"); err != nil {
		return fmt.Errorf("prune worktrees: %w", err)
	}

	return nil
}

// TreeExists reports whether subpath exists under commit in the repo at dir.
func TreeExists(ctx context.Context, dir, commit, subpath string) bool {
	return wireCmd(ctx, "-C", dir, "cat-file", "-e", commit+":"+subpath).Run() == nil
}

// TreeIsDir reports whether subpath exists as a directory (a tree object)
// under commit in the repo at dir. Files and missing paths both report
// false; existence alone is TreeExists. Operands are safe by the same
// construction as TreeExists: a resolved hex SHA plus validated segments.
func TreeIsDir(ctx context.Context, dir, commit, subpath string) bool {
	out, err := runMirror(ctx, dir, "cat-file", "-t", commit+":"+subpath)
	if err != nil {
		return false
	}

	return out == "tree"
}
