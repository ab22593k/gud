package wire

// List orchestration: read registry entries and derive their states.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Entry is one tracked checkout with its derived sync state. RemoteSHA is
// empty when the source is unreachable or the checkout diverged (diverged
// states derive locally without resolving). A missing target directory
// reports diverged: local state differs maximally from the export.
type Entry struct {
	Dir       string
	Entry     RegistryEntry
	State     SyncState
	RemoteSHA string
}

// List reads the registry at root and derives each entry's state. A missing
// registry file reads as empty. Unreachable remotes never fail the run:
// they become unreachable entries. A corrupt registry fails the run
// (fail-closed: never guess which entries survive).
func List(ctx context.Context, fetcher Fetcher, root string) ([]Entry, error) {
	if fetcher == nil {
		return nil, fmt.Errorf("list %s: nil fetcher", root)
	}

	reg, err := LoadRegistry(RegistryPath(root))
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", root, err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", root, err)
	}

	cached := Memoize(fetcher, newMemo())

	keys := reg.Keys()
	dirs := make([]string, len(keys))

	for i, key := range keys {
		dirs[i] = dirForKey(absRoot, key)
	}

	// Phase 1 hashes every checkout concurrently; phase 2 replays the exact
	// sequential decision order below, so error precedence, diverged
	// short-circuiting, and resolve order match the serial flow bit for bit.
	live := hashAll(dirs)

	entries := make([]Entry, 0, len(keys))

	for i, key := range keys {
		entry, err := describeHashed(ctx, cached, dirs[i], reg.Entries[key], live[i])
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Dir < entries[j].Dir })

	return entries, nil
}

// dirForKey resolves a registry key against the absolute registry directory.
func dirForKey(absRoot, key string) string {
	if key == "." {
		return absRoot
	}

	return filepath.Join(absRoot, filepath.FromSlash(strings.TrimPrefix(key, "./")))
}

// hashResult is one checkout's phase-1 outcome. missing reports an absent
// target directory (diverged, never an error); err aborts the run exactly as
// the serial describe did.
type hashResult struct {
	live    string
	missing bool
	err     error
}

// hashOne stats and hashes a single checkout. Read-only: safe to run
// concurrently across entries.
func hashOne(dir string) hashResult {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return hashResult{missing: true}
		}

		return hashResult{err: fmt.Errorf("describe %s: %w", dir, err)}
	}

	live, err := HashDir(dir)
	if err != nil {
		return hashResult{err: fmt.Errorf("describe %s: %w", dir, err)}
	}

	return hashResult{live: live}
}

// hashAll stats and hashes every directory, bounded by NumCPU. Hashing is a
// walk plus reads plus SHA-256 per checkout — profiled at ~13 syscalls per
// file plus 22% SHA time — so it scales with workers on SSD/NVMe and parallel
// filesystems. Results land per index; callers replay decisions in order.
func hashAll(dirs []string) []hashResult {
	out := make([]hashResult, len(dirs))
	if len(dirs) == 0 {
		return out
	}

	workers := max(runtime.NumCPU(), 1)
	workers = min(workers, len(dirs))

	var wg sync.WaitGroup

	sem := make(chan struct{}, workers)

	for i, dir := range dirs {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			out[i] = hashOne(dir)
		})
	}

	wg.Wait()

	return out
}

// describeHashed derives one entry's state from its precomputed hash,
// skipping the remote resolution when local divergence already decides it. A
// missing target directory reports diverged without resolving.
func describeHashed(
	ctx context.Context, fetcher Fetcher, dir string, entry RegistryEntry, h hashResult,
) (Entry, error) {
	if h.err != nil {
		return Entry{}, h.err
	}

	if h.missing {
		return Entry{Dir: dir, Entry: entry, State: StateDiverged}, nil
	}

	if h.live != entry.ExportHash {
		return Entry{Dir: dir, Entry: entry, State: StateDiverged}, nil
	}

	remoteSHA, resolveErr := resolveSHA(ctx, fetcher, entry)

	return Entry{
		Dir:       dir,
		Entry:     entry,
		State:     deriveState(entry, h.live, remoteSHA, resolveErr),
		RemoteSHA: remoteSHA,
	}, nil
}

// resolveSHA resolves the tracked source, returning "" on any failure; the
// caller reports the entry unreachable instead of failing the run.
func resolveSHA(ctx context.Context, fetcher Fetcher, entry RegistryEntry) (string, error) {
	res, err := fetcher.Resolve(ctx, entry.Source())
	if err != nil {
		slog.Debug("wire resolve failed", "source", entry.Source().Display(), "err", err)

		return "", err
	}

	return res.Commit, nil
}
