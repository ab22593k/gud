package wire

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// FetchOptions tunes Fetch. Fetcher must be non-nil; Force permits
// replacing a non-empty target.
type FetchOptions struct {
	Fetcher Fetcher
	Force   bool
}

// Fetch downloads only source's subfolder into target and writes its
// tracking record, returning the user-facing summary. The subset is
// acquired through a sparse checkout (see Fetcher.Materialize) and staged
// aside first: a failed fetch never reports success and never leaves a
// half-populated target behind.
func Fetch(ctx context.Context, opts FetchOptions, source SourceRef, target string) (string, error) {
	if opts.Fetcher == nil {
		return "", fmt.Errorf("fetch %s: nil fetcher", source.Display())
	}

	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", target, err)
	}

	target = abs

	if err := checkTarget(target, opts.Force); err != nil {
		return "", fmt.Errorf("fetch %s: %w", source.Display(), err)
	}

	res, err := opts.Fetcher.Resolve(ctx, source)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", source.Display(), err)
	}

	resolved := source
	resolved.Ref = res.Ref
	resolved.Subpath = res.Subpath

	files, err := installCheckout(ctx, opts.Fetcher, resolved, res, target)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", source.Display(), err)
	}

	if err := recordCheckout(target, resolved, res.Commit); err != nil {
		return "", fmt.Errorf("fetch %s: %w", source.Display(), err)
	}

	slog.Debug("wire fetched", "source", resolved.Display(), "commit", res.Commit, "files", files)

	return fmt.Sprintf("Fetched %s at %s into %s (%d files).\nTracked for future updates (%s).",
		resolved.Display(), ShortSHA(res.Commit), target, files, RecordPath(target)), nil
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

// checkTarget verifies target is usable without touching it: missing
// directories are fine, empty ones are fine, and anything else requires
// Force. Runs before any network use so local mistakes fail fast offline.
func checkTarget(target string, force bool) error {
	entries, err := os.ReadDir(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("inspect target: %w", err)
	}

	if len(entries) == 0 || force {
		return nil
	}

	return fmt.Errorf("target %s holds %d entries: %w", target, len(entries), ErrTargetNotEmpty)
}

// recordCheckout hashes the extracted target and saves its tracking record.
func recordCheckout(target string, resolved SourceRef, commit string) error {
	hash, err := HashDir(target)
	if err != nil {
		return fmt.Errorf("hash checkout: %w", err)
	}

	return SaveRecord(target, RecordFor(resolved, commit, hash, time.Now()))
}
