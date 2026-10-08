package wire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// FetchOptions tunes Fetch. Fetcher must be non-nil; Force permits
// replacing a non-empty target. RegistryPath is the explicit run-level
// registry file fetch upserts; only the Cobra layer resolves it from cwd.
type FetchOptions struct {
	Fetcher      Fetcher
	Force        bool
	RegistryPath string
}

// Fetch downloads only source's subfolder into target and upserts its
// registry entry, returning the user-facing summary. A re-fetch of a
// tracked checkout that already matches its upstream reports "Already up
// to date" without re-downloading or rewriting anything. The subset is
// acquired through a sparse checkout (see Fetcher.Materialize) and staged
// aside first: a failed fetch never reports success and never leaves a
// half-populated target behind. Targets outside the registry tree are
// rejected: this registry does not track them.
func Fetch(ctx context.Context, opts FetchOptions, source SourceRef, target string) (string, error) {
	if opts.Fetcher == nil {
		return "", fmt.Errorf("fetch %s: nil fetcher", source.Display())
	}

	if opts.RegistryPath == "" {
		return "", fmt.Errorf("fetch %s: no registry path", source.Display())
	}

	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", target, err)
	}

	target = abs

	absReg, err := filepath.Abs(opts.RegistryPath)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", opts.RegistryPath, err)
	}

	key, err := KeyFor(absReg, target)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", source.Display(), err)
	}

	entry, tracked, err := trackedEntry(absReg, key)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", source.Display(), err)
	}

	resolved, res := source, Resolution{}

	if tracked {
		var summary string

		res, resolved, summary, err = fetchTracked(ctx, opts, source, target, entry)
		if err != nil {
			return "", fmt.Errorf("fetch %s: %w", source.Display(), err)
		}

		if summary != "" {
			return summary, nil
		}
	}

	if err := checkTarget(target, opts.Force); err != nil {
		return "", fmt.Errorf("fetch %s: %w", source.Display(), err)
	}

	if !tracked {
		res, resolved, err = fetchFresh(ctx, opts, source)
		if err != nil {
			return "", fmt.Errorf("fetch %s: %w", source.Display(), err)
		}
	}

	summary, err := fetchInstall(ctx, opts, resolved, res, absReg, key, target)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", source.Display(), err)
	}

	return summary, nil
}

// fetchInstall materializes res into target, upserts its registry entry,
// and renders the fetch success summary.
func fetchInstall(
	ctx context.Context,
	opts FetchOptions,
	resolved SourceRef,
	res Resolution,
	absReg, key, target string,
) (string, error) {
	files, err := installCheckout(ctx, opts.Fetcher, resolved, res, target)
	if err != nil {
		return "", err
	}

	if err := recordCheckout(absReg, key, target, resolved, res.Commit); err != nil {
		return "", err
	}

	slog.Debug("wire fetched", "source", resolved.Display(), "commit", res.Commit, "files", files)

	return fmt.Sprintf("Fetched %s at %s into %s (%d files).\nTracked for future updates (%s).",
		resolved.Display(), ShortSHA(res.Commit), target, files, absReg), nil
}

// installCheckout materializes the resolved subfolder into a staging
// sibling, then moves it into place: fresh renames for missing targets,
// backup-swaps for force-replaced ones.
func installCheckout(
	ctx context.Context,
	fetcher Fetcher,
	resolved SourceRef,
	res Resolution,
	target string,
) (int, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return 0, fmt.Errorf("create parent: %w", err)
	}

	staging, err := os.MkdirTemp(filepath.Dir(target), ".wire-fetch-*")
	if err != nil {
		return 0, fmt.Errorf("stage checkout: %w", err)
	}

	defer func() { _ = os.RemoveAll(staging) }()

	files, err := fetcher.Materialize(ctx, resolved, res, staging)
	if err != nil {
		return 0, err
	}

	if _, err := os.Stat(target); err != nil {
		if !os.IsNotExist(err) {
			return 0, fmt.Errorf("inspect target: %w", err)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return 0, fmt.Errorf("create parent: %w", err)
		}

		if err := os.Rename(staging, target); err != nil {
			return 0, fmt.Errorf("install checkout: %w", err)
		}

		return files, nil
	}

	if err := replaceDir(target, staging); err != nil {
		return 0, err
	}

	return files, nil
}

// fetchFresh resolves a first-time source. The target guard already ran,
// so this stays network-only like its tracked counterpart.
func fetchFresh(ctx context.Context, opts FetchOptions, source SourceRef) (Resolution, SourceRef, error) {
	resolved := source

	res, err := opts.Fetcher.Resolve(ctx, source)
	if err != nil {
		return Resolution{}, SourceRef{}, err
	}

	resolved.Ref = res.Ref
	resolved.Subpath = res.Subpath

	return res, resolved, nil
}

// trackedEntry returns the registry entry for key. A missing registry
// file or an absent key reports tracked=false without resolving; a
// present-but-broken registry fails instead of guessing a source.
func trackedEntry(registryPath, key string) (RegistryEntry, bool, error) {
	reg, err := LoadRegistry(registryPath)
	if err != nil {
		return RegistryEntry{}, false, err
	}

	entry, err := reg.Lookup(key)
	if err != nil {
		if errors.Is(err, ErrNotACheckout) {
			return RegistryEntry{}, false, nil
		}

		return RegistryEntry{}, false, err
	}

	return entry, true, nil
}

// fetchTracked resolves an already-tracked checkout and reports its
// no-op summary when it already matches upstream. An empty summary means
// fall through to the regular fetch flow with res resolved.
func fetchTracked(
	ctx context.Context,
	opts FetchOptions,
	source SourceRef,
	target string,
	entry RegistryEntry,
) (Resolution, SourceRef, string, error) {
	resolved := source

	res, err := opts.Fetcher.Resolve(ctx, source)
	if err != nil {
		return Resolution{}, SourceRef{}, "", err
	}

	resolved.Ref = res.Ref
	resolved.Subpath = res.Subpath

	summary, _ := noOpSummary(target, entry, res)

	return res, resolved, summary, nil
}

// noOpSummary reports "Already up to date" when the checkout at target
// already matches the resolved upstream: same commit and unmodified
// contents. Anything else (moved upstream, local edits, unreadable
// target) reports false so the caller falls through to the regular flow.
func noOpSummary(target string, entry RegistryEntry, res Resolution) (string, bool) {
	if res.Commit != entry.ResolvedCommit {
		return "", false
	}

	live, err := HashDir(target)
	if err != nil || live != entry.ExportHash {
		return "", false
	}

	slog.Debug("wire fetch up to date", "dir", target, "commit", res.Commit)

	return fmt.Sprintf("Already up to date (%s).", ShortSHA(res.Commit)), true
}

// checkTarget verifies target is usable without touching it: missing
// directories are fine, empty ones are fine, and anything else requires
// Force. Runs before any network use so local mistakes fail fast offline.
//
// Only one directory entry is read to decide: the previous shape used
// os.ReadDir, which reads and sorts every entry, so a target holding tens of
// thousands of files paid O(n log n) to answer "is it empty". The full
// listing runs solely on the non-empty, non-force path, where the count is
// needed for the error message.
func checkTarget(target string, force bool) error {
	f, err := os.Open(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("inspect target: %w", err)
	}

	defer func() { _ = f.Close() }()

	names, err := f.Readdirnames(1)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("inspect target: %w", err)
	}

	if len(names) == 0 || force {
		return nil
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		return fmt.Errorf("inspect target: %w", err)
	}

	return fmt.Errorf("target %s holds %d entries: %w", target, len(entries), ErrTargetNotEmpty)
}

// recordCheckout hashes the extracted target and upserts its registry
// entry under the already-validated key, persisting the registry atomically.
func recordCheckout(registryPath, key, target string, resolved SourceRef, commit string) error {
	hash, err := HashDir(target)
	if err != nil {
		return fmt.Errorf("hash checkout: %w", err)
	}

	reg, err := LoadRegistry(registryPath)
	if err != nil {
		return err
	}

	if err := reg.Upsert(key, EntryFor(resolved, commit, hash, time.Now())); err != nil {
		return err
	}

	return SaveRegistry(registryPath, reg)
}
