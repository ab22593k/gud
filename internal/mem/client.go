package mem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
type DB struct {
	client   *helix.Client
	dataDir  string
	database string
	enabled  bool
}

// NewDB opens an embedded HelixDB at DataDir/Database. If opts.Enabled
// is false the client is nil and all operations return
// ErrHelixUnavailable. When the embedded runtime is unavailable (standard
// Go module without native bindings) the DB is disabled so callers
// degrade gracefully.
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

// Close releases the embedded handle. Nil-safe; degraded DBs are a no-op.
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

// EnsureSchema creates indexes and ensures the graph schema exists.
// This is idempotent and safe to call on every startup.
// Uses tenant-partitioned indexes where applicable for multi-repo isolation.
func (db *DB) EnsureSchema(ctx context.Context) error {
	if !db.enabled || db.client == nil {
		return ErrHelixUnavailable
	}

	sctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	ctx = sctx

	indexes := []*helix.Traversal{
		// Tenant-partitioned text indexes for the Commit label.
		helix.G().CreateTextIndexNodes("Commit", "message", DefaultTenantProperty),
		helix.G().CreateTextIndexNodes("Commit", "diff_text", DefaultTenantProperty),
		helix.G().CreateTextIndexNodes("File", "path", DefaultTenantProperty),
		helix.G().CreateTextIndexNodes("CodeElement", "signature", DefaultTenantProperty),
		helix.G().CreateTextIndexNodes("CodeElement", "name", DefaultTenantProperty),
		helix.G().CreateTextIndexNodes("Memory", "content", DefaultTenantProperty),

		// Tenant-partitioned vector index for Commit embeddings.
		helix.G().CreateVectorIndexNodes(
			"Commit", "embedding", DefaultEmbeddingDimension, helix.VectorDistanceCosine, DefaultTenantProperty,
		),
		helix.G().CreateVectorIndexNodes(
			"Memory", "embedding", DefaultEmbeddingDimension, helix.VectorDistanceCosine, DefaultTenantProperty,
		),

		// Commit equality indexes.
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("Commit", "id")),
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("Commit", "repo_path")),
		helix.G().CreateIndexIfNotExists(helix.NodeRangeIndex("Commit", "timestamp")),

		// Tenant-scoped equality indexes.
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("Author", "email")),
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("Repo", "path")),
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("File", "path")),

		// CodeElement indexes for entity-aware queries.
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("CodeElement", "elementKey")),
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("CodeElement", "name")),

		// Memory indexes for the general memory model.
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("Memory", "memoryId")),
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("Memory", "userId")),
		helix.G().CreateIndexIfNotExists(helix.NodeRangeIndex("Memory", "createdAt")),
		helix.G().CreateIndexIfNotExists(helix.NodeRangeIndex("Memory", "salience")),

		// Category and Entity indexes.
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("Category", "categoryKey")),
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("Entity", "entityKey")),
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("Entity", "name")),
	}

	for _, idx := range indexes {
		req := helix.WriteQuery("schema_migration").VarAs("_", idx).Returning()
		if err := db.client.Exec(ctx, req, nil); err != nil {
			return fmt.Errorf("schema migration: %w", err)
		}
	}

	return nil
}
