package wire

import (
	"context"
	"log/slog"
	"time"

	"gud/internal/cache"
)

// Process-scoped memo of remote resolutions.

// memo caches resolutions for one command run so list/update over many
// checkouts sharing a repository resolves each distinct ref once.
type memo struct {
	cache *cache.Cache[string, Resolution]
}

// newMemo returns an empty memo. Entries live for the process run; a stale
// resolution cannot outlive the command that made it.
func newMemo() *memo {
	return &memo{cache: cache.New[string, Resolution](128, time.Hour)}
}

// get returns the memoized resolution for key.
func (m *memo) get(key string) (Resolution, bool) {
	res, ok := m.cache.Get(key)

	slog.Debug("wire memo lookup", "key", key, "hit", ok)

	return res, ok
}

// set records the resolution for key.
func (m *memo) set(key string, res Resolution) {
	m.cache.Set(key, res)
}

// memoFetcher memoizes successful Resolve calls per source cache key.
// Failures pass through unmemoized so transient outages are retried.
type memoFetcher struct {
	inner Fetcher
	memo  *memo
}

// Memoize wraps f with per-run resolution memoization.
func Memoize(f Fetcher, m *memo) Fetcher {
	if m == nil {
		m = newMemo()
	}

	return memoFetcher{inner: f, memo: m}
}

func (m memoFetcher) Resolve(ctx context.Context, source SourceRef) (Resolution, error) {
	key := source.CacheKey()

	if res, ok := m.memo.get(key); ok {
		return res, nil
	}

	res, err := m.inner.Resolve(ctx, source)
	if err != nil {
		return Resolution{}, err
	}

	m.memo.set(key, res)

	return res, nil
}

func (m memoFetcher) Materialize(ctx context.Context, source SourceRef, res Resolution, dir string) (int, error) {
	return m.inner.Materialize(ctx, source, res, dir)
}
