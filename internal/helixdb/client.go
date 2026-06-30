package helixdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/helixdb/helix-db/sdks/go"
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

// Options configures a DB connection.
type Options struct {
	BaseURL string
	APIKey  string
	Enabled bool
}

// DB wraps a helix.Client with lifecycle management and degraded-mode support.
type DB struct {
	client  *helix.Client
	baseURL string
	apiKey  string
	enabled bool
}

// NewDB creates a new DB wrapper. If opts.Enabled is false, the client is nil
// and all operations return ErrHelixUnavailable.
func NewDB(opts Options) *DB {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:6969"
	}

	db := &DB{
		baseURL: baseURL,
		apiKey:  opts.APIKey,
		enabled: opts.Enabled,
	}

	if !opts.Enabled {
		return db
	}

	client, err := helix.NewClient(baseURL, helix.WithAPIKey(opts.APIKey))
	if err != nil {
		db.enabled = false
		return db
	}

	db.client = client
	return db
}

// BaseURL returns the configured base URL.
func (db *DB) BaseURL() string { return db.baseURL }

// APIKey returns the configured API key.
func (db *DB) APIKey() string { return db.apiKey }

// Enabled returns whether HelixDB integration is enabled.
func (db *DB) Enabled() bool { return db.enabled && db.client != nil }

// IsAvailable checks if the HelixDB server is reachable via its health endpoint.
func (db *DB) IsAvailable(ctx context.Context) bool {
	if !db.enabled || db.client == nil {
		return false
	}
	healthCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(healthCtx, http.MethodGet, db.baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Exec runs a HelixDB query.
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

	indexes := []*helix.Traversal{
		// Tenant-partitioned text indexes for the Commit label.
		helix.G().CreateTextIndexNodes("Commit", "message", DefaultTenantProperty),
		helix.G().CreateTextIndexNodes("Commit", "diff_text", DefaultTenantProperty),
		helix.G().CreateTextIndexNodes("File", "path", DefaultTenantProperty),
		helix.G().CreateTextIndexNodes("CodeElement", "signature", DefaultTenantProperty),
		helix.G().CreateTextIndexNodes("CodeElement", "name", DefaultTenantProperty),
		helix.G().CreateTextIndexNodes("Memory", "content", DefaultTenantProperty),

		// Tenant-partitioned vector index for Commit embeddings.
		helix.G().CreateVectorIndexNodes("Commit", "embedding", DefaultTenantProperty),
		helix.G().CreateVectorIndexNodes("Memory", "embedding", DefaultTenantProperty),

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
