package mem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	helix "github.com/helixdb/helix-db/sdks/go"
)

// currentSchemaVersion is the expected HelixDB schema version. Bump it when
// the index set changes so existing databases re-migrate exactly once.
const currentSchemaVersion = 1

// schemaQueryName is the stored-query name used for the batched migration.
const schemaQueryName = "schema_migration"

// EnsureSchema creates indexes and ensures the graph schema exists.
//
// The migration runs as a single batched write transaction and records a
// filesystem version marker beside the embedded database. Steady-state
// invocations hit the marker and pay zero HelixDB transactions.
func (db *DB) EnsureSchema(ctx context.Context) error {
	if !db.enabled || db.client == nil {
		return ErrHelixUnavailable
	}

	if db.isSchemaCurrent() {
		return nil
	}

	sctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := db.client.Exec(sctx, buildSchemaMigrationQuery(), nil); err != nil {
		return fmt.Errorf("schema migration: %w", err)
	}

	// Best-effort: migration already succeeded, so a marker write failure
	// must not disable memory. The next invocation simply retries.
	_ = db.markSchemaCurrent()

	return nil
}

// schemaMarkerPath returns the version-marker file for this database.
// Empty when the DB has no usable data dir or database name.
func (db *DB) schemaMarkerPath() string {
	if db == nil || db.dataDir == "" || db.database == "" {
		return ""
	}

	return filepath.Join(db.dataDir, db.database+".schema_version")
}

// isSchemaCurrent reports whether the marker matches currentSchemaVersion.
func (db *DB) isSchemaCurrent() bool {
	path := db.schemaMarkerPath()
	if path == "" {
		return false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(data)) == strconv.Itoa(currentSchemaVersion)
}

// markSchemaCurrent records currentSchemaVersion atomically via tmp+rename.
func (db *DB) markSchemaCurrent() error {
	path := db.schemaMarkerPath()
	if path == "" {
		return errors.New("schema marker: empty path")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("schema marker mkdir: %w", err)
	}

	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, []byte(strconv.Itoa(currentSchemaVersion)), 0o600); err != nil {
		return fmt.Errorf("schema marker write: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("schema marker rename: %w", err)
	}

	return nil
}

// schemaIndexTraversals returns every index in the current schema version.
// Uses tenant-partitioned indexes where applicable for multi-repo isolation.
func schemaIndexTraversals() []*helix.Traversal {
	return []*helix.Traversal{
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
}

// buildSchemaMigrationQuery batches every schema index into one write
// transaction instead of one Exec per index on the hot path.
func buildSchemaMigrationQuery() helix.Request {
	query := helix.WriteQuery(schemaQueryName)

	for i, idx := range schemaIndexTraversals() {
		query.VarAs(fmt.Sprintf("idx_%02d", i), idx)
	}

	return query.Returning()
}
