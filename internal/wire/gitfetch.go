package wire

// Real git-backed Fetcher over shared blobless mirrors.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
// A nil store is a programming error; it is reported as an ErrInternal
// error, never a panic, so miswiring surfaces as a test failure or a
// returned error instead of crashing the CLI.
func NewFetcher(store *Store) (Fetcher, error) {
	if store == nil {
		return nil, fmt.Errorf("wire: nil store: %w", ErrInternal)
	}

	return &gitFetcher{store: store, fetched: make(map[string]bool)}, nil
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

	if err := verifyFolder(tctx, mirror, source, source.Ref); err != nil {
		return Resolution{}, err
	}

	return Resolution{Commit: source.Ref, Ref: source.Ref, Subpath: source.Subpath, Kind: RefKindCommit}, nil
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

	// A generic repository URL carries no explicit ref; resolve against
	// the remote's default branch reported as HEAD.
	if source.Ref == headRef {
		return resolveDefaultRef(tctx, mirror, refs, source)
	}

	known, commits := shortRefs(refs)

	actualRef, actualPath, ok := ResolveRef(known, source.Ref, source.Subpath)
	if !ok {
		return Resolution{}, fmt.Errorf("ref %q: %w", source.Ref, ErrUnknownRef)
	}

	if actualPath == "" {
		return Resolution{}, fmt.Errorf(
			"ref %q: %w: %s; append the subfolder path after the reference",
			actualRef, ErrBadURL, WholeRepositoryGuidance,
		)
	}

	resolved := source
	resolved.Ref = actualRef
	resolved.Subpath = actualPath

	commit := commits[actualRef]
	if err := verifyFolder(tctx, mirror, resolved, commit); err != nil {
		return Resolution{}, err
	}

	return Resolution{Commit: commit, Ref: actualRef, Subpath: actualPath, Kind: refKindOf(refs, actualRef)}, nil
}

// resolveDefaultRef resolves a reference-less source against the remote's
// default branch, which the mirror reports as HEAD.
func resolveDefaultRef(
	ctx context.Context, mirror string, refs map[string]string, source SourceRef,
) (Resolution, error) {
	commit := defaultBranchSHA(refs)
	if commit == "" {
		return Resolution{}, fmt.Errorf("ref %q: %w", source.Ref, ErrUnknownRef)
	}

	if source.Subpath == "" {
		return Resolution{}, fmt.Errorf(
			"ref %q: %w: %s; append the subfolder path after the reference",
			source.Ref, ErrBadURL, WholeRepositoryGuidance,
		)
	}

	if err := verifyFolder(ctx, mirror, source, commit); err != nil {
		return Resolution{}, err
	}

	return Resolution{Commit: commit, Ref: source.Ref, Subpath: source.Subpath, Kind: RefKindBranch}, nil
}

// refKindOf reports whether the resolved short name matched a tag: a
// refs/tags/ entry (plain or peeled annotated) pins, while heads and
// remote-tracking names float as branches.
func refKindOf(refs map[string]string, actualRef string) RefKind {
	if _, ok := refs["refs/tags/"+actualRef]; ok {
		return RefKindTag
	}

	if _, ok := refs["refs/tags/"+actualRef+"^{}"]; ok {
		return RefKindTag
	}

	return RefKindBranch
}

// defaultBranchSHA reports the remote's default branch tip. The mirror's
// local HEAD is frozen at clone time, so the remote-tracking HEAD wins when
// the listing carries one; a live listing that reports a bare HEAD falls back.
func defaultBranchSHA(refs map[string]string) string {
	if sha := refs[remoteTrackingPrefix+headRef]; sha != "" {
		return sha
	}

	return refs[headRef]
}

// verifyFolder confirms source names an existing folder at commit, echoing
// the parsed identity in either failure.
func verifyFolder(ctx context.Context, mirror string, source SourceRef, commit string) error {
	if !git.TreeExists(ctx, mirror, commit, source.Subpath) {
		return fmt.Errorf("%s: %w", source.Display(), ErrMissingPath)
	}

	if !git.TreeIsDir(ctx, mirror, commit, source.Subpath) {
		return fmt.Errorf("%s is a file, not a fetchable folder: %w", source.Display(), ErrMissingPath)
	}

	return nil
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
		if errors.Is(err, git.ErrSubpathNotDir) {
			return 0, fmt.Errorf("materialize %s: subpath %q is not a fetchable folder: %w",
				source.Display(), res.Subpath, ErrMissingPath)
		}

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
// commit SHAs. Upstream-derived refs are collected first: a cache's local
// refs/heads entries are frozen at clone time and never advance, so they
// must not shadow the remote-tracking copy of the same branch. Sorting the
// names removes map-order nondeterminism from branch/tag collisions too.
func shortRefs(refs map[string]string) (known []string, commits map[string]string) {
	commits = make(map[string]string)

	fulls := slices.Sorted(maps.Keys(refs))

	addRefs(commits, refs, fulls, func(full string) bool {
		return !strings.HasPrefix(full, "refs/heads/")
	})
	addRefs(commits, refs, fulls, func(full string) bool {
		return strings.HasPrefix(full, "refs/heads/")
	})

	for short := range commits {
		known = append(known, short)
	}

	return known, commits
}

// addRefs records the refs selected by keep. A peeled (^{}) tag SHA replaces
// the tag object's SHA; a name recorded earlier is never overwritten.
func addRefs(commits, refs map[string]string, fulls []string, keep func(string) bool) {
	for _, full := range fulls {
		if !keep(full) {
			continue
		}

		short, isPeeled := peelRef(full)
		if short == "" {
			continue
		}

		if isPeeled || commits[short] == "" {
			commits[short] = refs[full]
		}
	}
}

// peelRef strips refs/heads/ or refs/tags/ prefixes, reporting peeled
// annotated tags separately so their commit SHA wins over the tag object.
func peelRef(full string) (string, bool) {
	for _, prefix := range []string{"refs/heads/", "refs/tags/", remoteTrackingPrefix} {
		rest, ok := strings.CutPrefix(full, prefix)
		if !ok {
			continue
		}

		// Remote-tracking HEAD symrefs (refs/remotes/origin/HEAD) are not
		// branches or tags; defaultBranchSHA reads that entry directly
		// because it is the only HEAD a fetch keeps current.
		if rest == headRef {
			return "", false
		}

		return strings.CutSuffix(rest, "^{}")
	}

	return "", false
}

// remoteTrackingPrefix matches the ref namespace of a cloned mirror,
// whose branches are only visible as remote-tracking refs. LsRemote runs
// against the mirror, not the upstream URL, so without this prefix the
// longest-match table would be empty for every named ref (FR-003a).
const remoteTrackingPrefix = "refs/remotes/origin/"

// upstream wraps a transport failure as ErrUpstream, preserving detail.
// Both the operation context and the sentinel stay in the errors.Is chain.
func upstream(op string, err error) error {
	return fmt.Errorf("%s: %w: %w", op, err, ErrUpstream)
}
