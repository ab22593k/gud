package wire

// Real git-backed Fetcher over shared blobless mirrors.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	git "gud/internal/git"
)

// Network timeouts for mirror operations. Ref resolution itself runs
// against the local mirror; cloning and exporting move trees and blobs.
const (
	mirrorSyncTimeout = 5 * time.Minute
	localReadTimeout  = 30 * time.Second
	// materializeBudget bounds one copied subfolder (abuse guard; typical
	// folders are orders of magnitude smaller per the 50 MB assumption).
	materializeBudget = 2 << 30
)

// gitFetcher implements Fetcher with one blobless mirror per repository,
// shared across checkouts. It is not safe for concurrent use; one command
// run owns one instance.
type gitFetcher struct {
	store   *Store
	fetched map[string]bool
}

// NewFetcher returns the real git-backed Fetcher rooted at store.
func NewFetcher(store *Store) Fetcher {
	if store == nil {
		panic("wire: nil store")
	}

	return &gitFetcher{store: store, fetched: make(map[string]bool)}
}

func (f *gitFetcher) Resolve(ctx context.Context, source SourceRef) (Resolution, error) {
	mirror, err := f.ensureMirror(ctx, source)
	if err != nil {
		return Resolution{}, err
	}

	if IsPinnedSHA(source.Ref) {
		return f.resolvePinned(ctx, mirror, source)
	}

	return f.resolveNamed(ctx, mirror, source)
}

// resolvePinned verifies a full-SHA ref and its subpath exist locally.
func (f *gitFetcher) resolvePinned(ctx context.Context, mirror string, source SourceRef) (Resolution, error) {
	tctx, cancel := context.WithTimeout(ctx, localReadTimeout)
	defer cancel()

	if !git.CommitExists(tctx, mirror, source.Ref) {
		return Resolution{}, fmt.Errorf("commit %s: %w", ShortSHA(source.Ref), ErrUnknownRef)
	}

	if !git.TreeExists(tctx, mirror, source.Ref, source.Subpath) {
		return Resolution{}, fmt.Errorf("%s: %w", source.Display(), ErrMissingPath)
	}

	return Resolution{Commit: source.Ref, Ref: source.Ref, Subpath: source.Subpath}, nil
}

// resolveNamed matches the greedy ref against the mirror's branches and
// tags (longest match wins for slashed refs), then verifies the subpath.
func (f *gitFetcher) resolveNamed(ctx context.Context, mirror string, source SourceRef) (Resolution, error) {
	tctx, cancel := context.WithTimeout(ctx, localReadTimeout)
	defer cancel()

	refs, err := git.LsRemote(tctx, mirror)
	if err != nil {
		return Resolution{}, upstream("list refs", err)
	}

	known, commits := shortRefs(refs)

	actualRef, actualPath, ok := ResolveRef(known, source.Ref, source.Subpath)
	if !ok {
		return Resolution{}, fmt.Errorf("ref %q: %w", source.Ref, ErrUnknownRef)
	}

	if actualPath == "" {
		return Resolution{}, fmt.Errorf("ref %q points at the repository root: %w", actualRef, ErrMissingPath)
	}

	commit := commits[actualRef]

	if !git.TreeExists(tctx, mirror, commit, actualPath) {
		display := source.Host + "/" + source.Owner + "/" + source.Repo + "@" + actualRef + ":" + actualPath

		return Resolution{}, fmt.Errorf("%s: %w", display, ErrMissingPath)
	}

	return Resolution{Commit: commit, Ref: actualRef, Subpath: actualPath}, nil
}

// Materialize populates dir with the resolved subfolder, returning the
// regular-file count. The subset is acquired through a literal sparse
// checkout: an ephemeral linked worktree populated for exactly res.Subpath,
// copied out, then removed — no residue survives success or failure.
func (f *gitFetcher) Materialize(ctx context.Context, source SourceRef, res Resolution, dir string) (int, error) {
	if err := checkResPath(res.Subpath); err != nil {
		return 0, err
	}

	mirror, err := f.ensureMirror(ctx, source)
	if err != nil {
		return 0, err
	}

	wt, err := f.store.WorktreeDir()
	if err != nil {
		return 0, err
	}

	defer func() {
		if rerr := git.RemoveWorktree(ctx, mirror, wt); rerr != nil {
			slog.Debug("wire worktree remove failed", "dir", wt, "err", rerr)
		}
	}()

	tctx, cancel := context.WithTimeout(ctx, mirrorSyncTimeout)
	defer cancel()

	if err := git.AddSparseWorktree(tctx, mirror, wt, res.Commit, res.Subpath); err != nil {
		return 0, upstream("materialize "+source.Display(), err)
	}

	n, err := CopyTree(filepath.Join(wt, filepath.FromSlash(res.Subpath)), dir, materializeBudget)
	if err != nil {
		return 0, fmt.Errorf("copy out %s: %w", source.Display(), err)
	}

	return n, nil
}

// checkResPath rejects resolved subpaths that cannot safely join onto a
// worktree directory. Parse validation normally prevents these; resolved
// names additionally incorporate remote ref data, so verify at use.
func checkResPath(subpath string) error {
	for el := range strings.SplitSeq(subpath, "/") {
		if el == "" || el == "." || el == ".." {
			return fmt.Errorf("unsafe subpath %q: %w", subpath, ErrMissingPath)
		}
	}

	return nil
}

// ensureMirror clones the mirror on first use and fetches it once per run.
func (f *gitFetcher) ensureMirror(ctx context.Context, source SourceRef) (string, error) {
	mirror := f.store.MirrorDir(source.Host, source.Owner, source.Repo)

	if f.fetched[mirror] {
		return mirror, nil
	}

	tctx, cancel := context.WithTimeout(ctx, mirrorSyncTimeout)
	defer cancel()

	if _, err := os.Stat(mirror); err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect mirror: %w", err)
		}

		if err := git.CloneMirror(tctx, source.CloneURL(), mirror); err != nil {
			return "", upstream("clone "+source.Owner+"/"+source.Repo, err)
		}
	} else if err := git.FetchMirror(tctx, mirror); err != nil {
		return "", upstream("fetch "+source.Owner+"/"+source.Repo, err)
	}

	if err := git.PruneWorktrees(tctx, mirror); err != nil {
		slog.Debug("wire worktree prune failed", "mirror", mirror, "err", err)
	}

	f.fetched[mirror] = true

	return mirror, nil
}

// shortRefs reduces full ref names to branch/tag short names mapped to
// commit SHAs, preferring peeled (^{}) SHAs for annotated tags and
// skipping non-branch entries such as HEAD symrefs.
func shortRefs(refs map[string]string) (known []string, commits map[string]string) {
	commits = make(map[string]string)

	for full, sha := range refs {
		short, isPeeled := peelRef(full)
		if short == "" {
			continue
		}

		if isPeeled || commits[short] == "" {
			commits[short] = sha
		}
	}

	for short := range commits {
		known = append(known, short)
	}

	return known, commits
}

// peelRef strips refs/heads/ or refs/tags/ prefixes, reporting peeled
// annotated tags separately so their commit SHA wins over the tag object.
func peelRef(full string) (string, bool) {
	for _, prefix := range []string{"refs/heads/", "refs/tags/"} {
		if rest, ok := strings.CutPrefix(full, prefix); ok {
			return strings.CutSuffix(rest, "^{}")
		}
	}

	return "", false
}

// upstream wraps a transport failure as ErrUpstream, preserving detail.
// Both the operation context and the sentinel stay in the errors.Is chain.
func upstream(op string, err error) error {
	return fmt.Errorf("%s: %w: %w", op, err, ErrUpstream)
}
