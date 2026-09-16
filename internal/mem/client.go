package mem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	helix "github.com/helixdb/helix-db/sdks/go"
)

// ErrHelixUnavailable is returned when HelixDB is not reachable.
var ErrHelixUnavailable = errors.New("helixdb: unavailable")

// NewHelixUnavailableError wraps a cause into an ErrHelixUnavailable sentinel.
func NewHelixUnavailableError(cause error) error {
	if cause == nil {
		return ErrHelixUnavailable
	}

	return fmt.Errorf("%w: %w", ErrHelixUnavailable, cause)
}

// DefaultDatabase is the embedded logical database name.
const DefaultDatabase = "gud"

// DefaultDataDir returns the default embedded storage root:
// $XDG_CACHE_HOME/gud/helixdb (os.UserCacheDir), falling back to
// ~/.cache/gud/helixdb when the cache dir is unavailable.
func DefaultDataDir() string {
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		return filepath.Join(dir, "gud", "helixdb")
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".cache", "gud", "helixdb")
	}

	return filepath.Join(".", ".helixdb")
}

// Options configures an embedded DB connection.
type Options struct {
	DataDir  string
	Database string
	Enabled  bool
}

// DB wraps a helix.Client opened against an embedded database with
// lifecycle management and degraded-mode support. DB must always be
// used as a pointer.
//
// The handle is per-process against a shared on-disk store. A second
// concurrent process opening the same DataDir/Database may fail to acquire
// the store and degrades to disabled via NewDB; Close flushes pending writes
// and releases the lock promptly, so owners must Close exactly once per open.
type DB struct {
	client   *helix.Client
	dataDir  string
	database string
	enabled  bool
}

// NewDB opens an embedded HelixDB at DataDir/Database. If opts.Enabled
// is false the client is nil and all operations return
// ErrHelixUnavailable. When the embedded runtime is unavailable (standard
// Go module without native bindings) or a concurrent process holds the
// store, the DB degrades to disabled so callers proceed without memory
// instead of failing the commit.
func NewDB(opts Options) *DB {
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}

	database := opts.Database
	if database == "" {
		database = DefaultDatabase
	}

	db := &DB{dataDir: dataDir, database: database, enabled: opts.Enabled}

	if !opts.Enabled {
		return db
	}

	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		db.enabled = false

		return db
	}

	client, err := helix.NewEmbeddedClient(helix.DiskSource{Root: dataDir, Database: database})
	if err != nil {
		db.enabled = false

		return db
	}

	db.client = client

	return db
}

// DataDir returns the embedded storage root.
func (db *DB) DataDir() string { return db.dataDir }

// Database returns the embedded logical database name.
func (db *DB) Database() string { return db.database }

// Enabled returns whether HelixDB integration is enabled.
func (db *DB) Enabled() bool { return db.enabled && db.client != nil }

// IsAvailable reports whether the embedded database is open. There is
// no network probe: availability is purely process-local.
func (db *DB) IsAvailable(_ context.Context) bool {
	return db.enabled && db.client != nil
}

// Close releases the embedded handle, flushing pending writes and freeing
// the on-disk lock for the next invocation. Nil-safe; degraded DBs are a no-op.
func (db *DB) Close() error {
	if db == nil || db.client == nil {
		return nil
	}

	return db.client.Close()
}

// Exec runs a HelixDB query against the embedded engine. Server routing
// options (WriterOnly, WarmOnly, AwaitDurability) are rejected by the
// SDK in embedded mode, so callers must not pass them.
func (db *DB) Exec(ctx context.Context, req helix.Request, out any, opts ...helix.ExecOption) error {
	if !db.enabled || db.client == nil {
		return ErrHelixUnavailable
	}

	return db.client.Exec(ctx, req, out, opts...)
}
