package wire

// Update orchestration: refresh a checkout from its tracking record.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UpdateOptions tunes Update. Fetcher must be non-nil; Force discards local
// modifications instead of refusing with ErrDiverged.
type UpdateOptions struct {
	Fetcher Fetcher
	Force   bool
}

// Update brings target up to date with its tracked source, working from the
// colocated record alone. Unchanged checkouts report "Already up to date"
// without rewriting files. The new state is staged aside and swapped in,
// so a failed update leaves local files and the record untouched.
func Update(ctx context.Context, opts UpdateOptions, target string) (string, error) {
	if opts.Fetcher == nil {
		return "", fmt.Errorf("update %s: nil fetcher", target)
	}

	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", target, err)
	}

	target = abs

	rec, err := LoadRecord(target)
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	live, err := HashDir(target)
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	res, err := opts.Fetcher.Resolve(ctx, rec.Source())
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	if res.Commit == rec.ResolvedCommit {
		return updateSameCommit(ctx, opts, rec, res, live, target)
	}

	if live == rec.ExportHash {
		return updateClean(ctx, opts, rec, res, target)
	}

	if opts.Force {
		return updateClean(ctx, opts, rec, res, target)
	}

	return mergeAttempt(ctx, opts.Fetcher, rec, res, target)
}

// updateSameCommit handles an update when upstream has not moved: a clean
// checkout is already current, a diverged one has nothing to merge with.
func updateSameCommit(
	ctx context.Context,
	opts UpdateOptions,
	rec TrackingRecord,
	res Resolution,
	live, target string,
) (string, error) {
	if live == rec.ExportHash {
		slog.Debug("wire up to date", "dir", target, "commit", res.Commit)

		return fmt.Sprintf("Already up to date (%s).", ShortSHA(res.Commit)), nil
	}

	if opts.Force {
		return updateClean(ctx, opts, rec, res, target)
	}

	return "", fmt.Errorf("update %s: %w", target, ErrDiverged)
}

// updateClean replaces the checkout with the resolved upstream state,
// discarding local edits only under explicit force.
func updateClean(
	ctx context.Context,
	opts UpdateOptions,
	rec TrackingRecord,
	res Resolution,
	target string,
) (string, error) {
	files, err := swapCheckout(ctx, opts.Fetcher, rec.Source(), res, target)
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	slog.Debug("wire updated", "dir", target, "commit", res.Commit, "files", files)

	return fmt.Sprintf("Updated %s to %s (%d files).", target, ShortSHA(res.Commit), files), nil
}

// mergeAttempt merges a moved upstream into a diverged checkout. Base (the
// recorded commit) and new (the resolved commit) materialize to temp dirs;
// every path classifies before anything writes, so a conflict leaves the
// target and its record untouched.
func mergeAttempt(
	ctx context.Context,
	fetcher Fetcher,
	rec TrackingRecord,
	res Resolution,
	target string,
) (string, error) {
	source := rec.Source()

	baseDir, err := os.MkdirTemp(filepath.Dir(target), ".wire-base-*")
	if err != nil {
		return "", fmt.Errorf("update %s: stage base: %w", target, err)
	}

	defer func() { _ = os.RemoveAll(baseDir) }()

	newDir, err := os.MkdirTemp(filepath.Dir(target), ".wire-new-*")
	if err != nil {
		return "", fmt.Errorf("update %s: stage new: %w", target, err)
	}

	defer func() { _ = os.RemoveAll(newDir) }()

	baseRes := Resolution{Commit: rec.ResolvedCommit, Ref: rec.Ref, Subpath: rec.Subpath}

	if _, err := fetcher.Materialize(ctx, source, baseRes, baseDir); err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	if _, err := fetcher.Materialize(ctx, source, res, newDir); err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	baseSnap, err := snapshotDir(baseDir)
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	localSnap, err := snapshotDir(target)
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	newSnap, err := snapshotDir(newDir)
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	take, del, conflicts := classify(baseSnap, localSnap, newSnap)

	if len(conflicts) > 0 {
		return "", fmt.Errorf("update %s: conflicting files:\n  %s: %w", target, strings.Join(conflicts, "\n  "), ErrDiverged)
	}

	summary, err := applyMerged(source, res, target, newDir, take, del, keptLocal(baseSnap, localSnap, conflicts))
	if err != nil {
		return "", fmt.Errorf("update %s: %w", target, err)
	}

	return summary, nil
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
// in and advances the tracking record.
func applyMerged(
	source SourceRef,
	res Resolution,
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

	if err := recordCheckout(target, resolved, res.Commit); err != nil {
		return "", err
	}

	applied := len(take) + len(del)

	slog.Debug("wire merged", "dir", target, "commit", res.Commit, "applied", applied, "kept", kept)

	return fmt.Sprintf("Merged %s into %s (%d upstream files, %d local files kept).",
		ShortSHA(res.Commit), target, applied, kept), nil
}

// swapCheckout stages the new state aside, then swaps it into target with
// a backup so interruption cannot lose the previous checkout.
func swapCheckout(ctx context.Context, fetcher Fetcher, source SourceRef, res Resolution, target string) (int, error) {
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

	hash, err := HashDir(staging)
	if err != nil {
		return 0, fmt.Errorf("hash checkout: %w", err)
	}

	if err := replaceDir(target, staging); err != nil {
		return 0, err
	}

	if err := SaveRecord(target, RecordFor(resolved, res.Commit, hash, time.Now())); err != nil {
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
