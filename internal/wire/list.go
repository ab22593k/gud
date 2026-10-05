package wire

// List orchestration: discover checkouts and derive their states.

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxListDepth bounds the checkout discovery walk under the list root.
const maxListDepth = 8

// Entry is one tracked checkout with its derived sync state. RemoteSHA is
// empty when the source is unreachable or the checkout diverged (diverged
// states derive locally without resolving).
type Entry struct {
	Dir       string
	Record    TrackingRecord
	State     SyncState
	RemoteSHA string
}

// List discovers checkouts under root and derives each entry's state.
// Unreachable remotes and invalid records never fail the run: the former
// become unreachable entries, the latter are skipped with a debug record.
func List(ctx context.Context, fetcher Fetcher, root string) ([]Entry, error) {
	if fetcher == nil {
		return nil, fmt.Errorf("list %s: nil fetcher", root)
	}

	dirs, err := findCheckouts(root)
	if err != nil {
		return nil, err
	}

	cached := Memoize(fetcher, newMemo())

	entries := make([]Entry, 0, len(dirs))

	for _, dir := range dirs {
		entry, skip, err := describeCheckout(ctx, cached, dir)
		if err != nil {
			return nil, err
		}

		if skip {
			continue
		}

		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Dir < entries[j].Dir })

	return entries, nil
}

// describeCheckout loads one checkout's record and derives its state,
// skipping the remote resolution when local divergence already decides it.
// Records that fail to load are skipped with a debug record: one corrupt
// checkout must not hide the healthy ones.
func describeCheckout(ctx context.Context, fetcher Fetcher, dir string) (Entry, bool, error) {
	rec, err := LoadRecord(dir)
	if err != nil {
		slog.Debug("wire skipping unreadable checkout", "dir", dir, "err", err)

		return Entry{}, true, nil
	}

	live, err := HashDir(dir)
	if err != nil {
		return Entry{}, false, fmt.Errorf("describe %s: %w", dir, err)
	}

	if live != rec.ExportHash {
		return Entry{Dir: dir, Record: rec, State: StateDiverged}, false, nil
	}

	remoteSHA, resolveErr := resolveSHA(ctx, fetcher, rec)

	return Entry{
		Dir:       dir,
		Record:    rec,
		State:     deriveState(rec, live, remoteSHA, resolveErr),
		RemoteSHA: remoteSHA,
	}, false, nil
}

// resolveSHA resolves the tracked source, returning "" on any failure; the
// caller reports the entry unreachable instead of failing the run.
func resolveSHA(ctx context.Context, fetcher Fetcher, rec TrackingRecord) (string, error) {
	res, err := fetcher.Resolve(ctx, rec.Source())
	if err != nil {
		slog.Debug("wire resolve failed", "source", rec.Source().Display(), "err", err)

		return "", err
	}

	return res.Commit, nil
}

// findCheckouts walks root for tracking records, pruning .git internals
// and stopping past maxListDepth. Symlinked directories are never followed.
func findCheckouts(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", root, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("list %s: not a directory", root)
	}

	var dirs []string

	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		if rel != "." {
			if depth(rel) > maxListDepth {
				return filepath.SkipDir
			}

			if d.Name() == ".git" {
				return filepath.SkipDir
			}
		}

		if hasRecord(path) {
			dirs = append(dirs, path)
		}

		return nil
	}

	if err := filepath.WalkDir(root, walk); err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}

	return dirs, nil
}

// hasRecord reports whether dir holds a tracking record file. Candidates
// are validated later; unreadable records are skipped by describeCheckout.
func hasRecord(dir string) bool {
	info, err := os.Stat(RecordPath(dir))

	return err == nil && info.Mode().IsRegular()
}

// depth counts separators in a slash-separated relative path.
func depth(rel string) int {
	return strings.Count(filepath.ToSlash(rel), "/")
}
