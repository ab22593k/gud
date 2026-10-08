package wire

// Update orchestration: refresh a checkout from its registry entry.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// UpdateOptions tunes Update. Fetcher must be non-nil; Force discards local
// modifications instead of refusing with ErrDiverged. RegistryPath is the
// explicit run-level registry file holding the target's entry.
type UpdateOptions struct {
	Fetcher      Fetcher
	Force        bool
	RegistryPath string
}

// Update brings target up to date with its tracked source, working from the
// registry entry alone. Unchanged checkouts report "Already up to date"
// without rewriting files. A missing target directory reports diverged.
// The new state is staged aside and swapped in, so a failed update leaves
// local files and the registry untouched.
func Update(ctx context.Context, opts UpdateOptions, target string) (string, error) {
	if opts.Fetcher == nil {
		return "", fmt.Errorf("update %s: nil fetcher", target)
	}

	if opts.RegistryPath == "" {
		return "", fmt.Errorf("update %s: no registry path", target)
	}

	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", target, err)
	}

	target = abs

	reg, err := LoadRegistry(opts.RegistryPath)
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	key, err := KeyFor(opts.RegistryPath, target)
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	entry, err := reg.Lookup(key)
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	if _, err := os.Stat(target); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("update %s: target directory missing: %w", target, ErrDiverged)
		}

		return "", fmt.Errorf("update %s: %w", target, err)
	}

	live, err := HashDir(target)
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	res, err := opts.Fetcher.Resolve(ctx, entry.Source())
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	if res.Commit == entry.ResolvedCommit {
		return updateSameCommit(ctx, opts, entry, res, live, target)
	}

	if live == entry.ExportHash {
		return updateClean(ctx, opts, entry, res, target)
	}

	if opts.Force {
		return updateClean(ctx, opts, entry, res, target)
	}

	return mergeAttempt(ctx, opts.Fetcher, opts.RegistryPath, entry, res, target)
}

// updateSameCommit handles an update when upstream has not moved: a clean
// checkout is already current, a diverged one has nothing to merge with.
func updateSameCommit(
	ctx context.Context,
	opts UpdateOptions,
	entry RegistryEntry,
	res Resolution,
	live, target string,
) (string, error) {
	if live == entry.ExportHash {
		slog.Debug("wire up to date", "dir", target, "commit", res.Commit)

		return fmt.Sprintf("Already up to date (%s).", ShortSHA(res.Commit)), nil
	}

	if opts.Force {
		return updateClean(ctx, opts, entry, res, target)
	}

	return "", fmt.Errorf("update %s: %w", target, ErrDiverged)
}

// updateClean replaces the checkout with the resolved upstream state,
// discarding local edits only under explicit force.
func updateClean(
	ctx context.Context,
	opts UpdateOptions,
	entry RegistryEntry,
	res Resolution,
	target string,
) (string, error) {
	files, err := swapCheckout(ctx, opts.Fetcher, entry.Source(), res, target, opts.RegistryPath)
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	slog.Debug("wire updated", "dir", target, "commit", res.Commit, "files", files)

	return fmt.Sprintf("Updated %s to %s (%d files).", target, ShortSHA(res.Commit), files), nil
}

// mergeAttempt merges a moved upstream into a diverged checkout. Base (the
// recorded commit) and new (the resolved commit) materialize to temp dirs;
// every path classifies before anything writes, so a conflict leaves the
// target and its registry entry untouched.
func mergeAttempt(
	ctx context.Context,
	fetcher Fetcher,
	registryPath string,
	entry RegistryEntry,
	res Resolution,
	target string,
) (string, error) {
	source := entry.Source()

	key, err := KeyFor(registryPath, target)
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	baseRes := Resolution{Commit: entry.ResolvedCommit, Ref: entry.Ref, Subpath: entry.Subpath}

	snaps, cleanup, err := stageMergeSnaps(ctx, fetcher, source, baseRes, res, target)
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	defer cleanup()

	take, del, conflicts := classify(snaps.base, snaps.local, snaps.new)

	if len(conflicts) > 0 {
		return "", fmt.Errorf("update %s: conflicting files:\n  %s: %w",
			target, strings.Join(conflicts, "\n  "), ErrDiverged)
	}

	summary, err := applyMerged(source, res, registryPath, key, target,
		snaps.newDir, take, del, keptLocal(snaps.base, snaps.local, conflicts))
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	return summary, nil
}

// mergeSnaps bundles the three snapshots a merge classifies plus the staged
// new-state directory the merge applies from.
type mergeSnaps struct {
	base   map[string]snapFile
	local  map[string]snapFile
	new    map[string]snapFile
	newDir string
}

// stageMergeSnaps materializes base and new commits to temp dirs and
// snapshots base, local, and new states. The caller owns cleanup.
func stageMergeSnaps(
	ctx context.Context,
	fetcher Fetcher,
	source SourceRef,
	baseRes, res Resolution,
	target string,
) (mergeSnaps, func(), error) {
	fail := func(err error) (mergeSnaps, func(), error) {
		return mergeSnaps{}, func() {}, err
	}

	baseDir, err := os.MkdirTemp(filepath.Dir(target), ".wire-base-*")
	if err != nil {
		return fail(fmt.Errorf("stage base: %w", err))
	}

	newDir, err := os.MkdirTemp(filepath.Dir(target), ".wire-new-*")
	if err != nil {
		_ = os.RemoveAll(baseDir)

		return fail(fmt.Errorf("stage new: %w", err))
	}

	cleanup := func() {
		_ = os.RemoveAll(baseDir)
		_ = os.RemoveAll(newDir)
	}

	for _, job := range []struct {
		res Resolution
		dir string
	}{{baseRes, baseDir}, {res, newDir}} {
		if _, err := fetcher.Materialize(ctx, source, job.res, job.dir); err != nil {
			cleanup()

			return fail(err)
		}
	}

	snapped, err := snapshotAll([]string{baseDir, target, newDir})
	if err != nil {
		cleanup()

		return fail(err)
	}

	return mergeSnaps{base: snapped[0], local: snapped[1], new: snapped[2], newDir: newDir}, cleanup, nil
}

// snapshotAll snapshots every directory concurrently, returning results in
// input order with the first error in that order winning — the same contract
// as snapshotting them one by one. The snapshots are independent
// pure-filesystem reads (walk, read, SHA-256), while the Materialize calls
// above stay sequential: they touch the fetcher and git worktrees, which are
// not safe for concurrent use.
func snapshotAll(dirs []string) ([]map[string]snapFile, error) {
	snapped := make([]map[string]snapFile, len(dirs))
	errs := make([]error, len(dirs))

	var wg sync.WaitGroup

	for i, dir := range dirs {
		wg.Go(func() {
			snap, err := snapshotDir(dir)
			snapped[i] = snap
			errs[i] = err
		})
	}

	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return snapped, nil
}

// keptLocal counts locally-changed paths the merge preserves (changed
// locally, not conflicting).
func keptLocal(base, local map[string]snapFile, conflicts []string) int {
	conflicted := make(map[string]bool, len(conflicts))

	for _, p := range conflicts {
		conflicted[p] = true
	}

	kept := 0

	for _, p := range unionKeys(base, local) {
		if conflicted[p] {
			continue
		}

		if local[p].changedFrom(base[p]) {
			kept++
		}
	}

	return kept
}

// applyMerged stages the merge onto a clone of the target, then swaps it
// in and advances the registry entry.
func applyMerged(
	source SourceRef,
	res Resolution,
	registryPath, key,
	target, newDir string,
	take, del []string,
	kept int,
) (string, error) {
	staging, err := os.MkdirTemp(filepath.Dir(target), ".wire-merge-*")
	if err != nil {
		return "", fmt.Errorf("stage merge: %w", err)
	}

	defer func() { _ = os.RemoveAll(staging) }()

	if err := cloneCheckout(target, staging); err != nil {
		return "", err
	}

	if err := applyMerge(staging, newDir, take, del); err != nil {
		return "", err
	}

	resolved := source
	resolved.Ref = res.Ref
	resolved.Subpath = res.Subpath

	if err := replaceDir(target, staging); err != nil {
		return "", err
	}

	if err := recordCheckout(registryPath, key, target, resolved, res.Commit); err != nil {
		return "", err
	}

	applied := len(take) + len(del)

	slog.Debug("wire merged", "dir", target, "commit", res.Commit, "applied", applied, "kept", kept)

	return fmt.Sprintf("Merged %s into %s (%d upstream files, %d local files kept).",
		ShortSHA(res.Commit), target, applied, kept), nil
}

// swapCheckout stages the new state aside, then swaps it into target with
// a backup so interruption cannot lose the previous checkout. The registry
// entry advances to the new commit once the swap succeeds.
func swapCheckout(
	ctx context.Context,
	fetcher Fetcher,
	source SourceRef,
	res Resolution,
	target, registryPath string,
) (int, error) {
	staging, err := os.MkdirTemp(filepath.Dir(target), ".wire-update-*")
	if err != nil {
		return 0, fmt.Errorf("stage checkout: %w", err)
	}

	defer func() { _ = os.RemoveAll(staging) }()

	files, err := fetcher.Materialize(ctx, source, res, staging)
	if err != nil {
		return 0, err
	}

	resolved := source
	resolved.Ref = res.Ref
	resolved.Subpath = res.Subpath

	key, err := KeyFor(registryPath, target)
	if err != nil {
		return 0, err
	}

	if err := replaceDir(target, staging); err != nil {
		return 0, err
	}

	if err := recordCheckout(registryPath, key, target, resolved, res.Commit); err != nil {
		return 0, err
	}

	return files, nil
}

// replaceDir swaps staging into place at target, keeping a backup until
// the swap succeeds so a failure restores rather than loses the checkout.
func replaceDir(target, staging string) error {
	backup, err := os.MkdirTemp(filepath.Dir(target), ".wire-backup-*")
	if err != nil {
		return fmt.Errorf("stage backup: %w", err)
	}

	defer func() { _ = os.RemoveAll(backup) }()

	// MkdirTemp created backup; rename target into its parent slot.
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("stage backup: %w", err)
	}

	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("back up checkout: %w", err)
	}

	if err := os.Rename(staging, target); err != nil {
		_ = os.Rename(backup, target)

		return fmt.Errorf("install checkout: %w", err)
	}

	return nil
}
