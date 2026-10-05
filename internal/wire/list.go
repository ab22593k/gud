package wire

// List orchestration: read registry entries and derive their states.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

	entries := make([]Entry, 0, len(reg.Entries))

	for _, key := range reg.Keys() {
		entry, err := describeEntry(ctx, cached, dirForKey(absRoot, key), reg.Entries[key])
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

// describeEntry derives one entry's state, skipping the remote resolution
// when local divergence already decides it. A missing target directory
// reports diverged without resolving.
func describeEntry(ctx context.Context, fetcher Fetcher, dir string, entry RegistryEntry) (Entry, error) {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return Entry{Dir: dir, Entry: entry, State: StateDiverged}, nil
		}

		return Entry{}, fmt.Errorf("describe %s: %w", dir, err)
	}

	live, err := HashDir(dir)
	if err != nil {
		return Entry{}, fmt.Errorf("describe %s: %w", dir, err)
	}

	if live != entry.ExportHash {
		return Entry{Dir: dir, Entry: entry, State: StateDiverged}, nil
	}

	remoteSHA, resolveErr := resolveSHA(ctx, fetcher, entry)

	return Entry{
		Dir:       dir,
		Entry:     entry,
		State:     deriveState(entry, live, remoteSHA, resolveErr),
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
