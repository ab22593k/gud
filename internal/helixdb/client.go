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
	Enabled bool // false = degraded mode, skip all HelixDB operations
}

// DB wraps a helix.Client with lifecycle management and degraded-mode support.
type DB struct {
	client  *helix.Client
	baseURL string
	apiKey  string
	enabled bool
}

// NewDB creates a new DB wrapper. If opts.Enabled is false, the client is nil
// and all operations return ErrHelixUnavailable. If opts.BaseURL is empty,
// defaults to "http://localhost:6969".
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

// Exec runs a HelixDB query. Returns ErrHelixUnavailable if the client is nil
// or disabled.
func (db *DB) Exec(ctx context.Context, req helix.Request, out any, opts ...helix.ExecOption) error {
	if !db.enabled || db.client == nil {
		return ErrHelixUnavailable
	}
	return db.client.Exec(ctx, req, out, opts...)
}

// EnsureSchema creates indexes and ensures the graph schema exists.
// This is idempotent and safe to call on every startup.
func (db *DB) EnsureSchema(ctx context.Context) error {
	if !db.enabled || db.client == nil {
		return ErrHelixUnavailable
	}

	indexes := []*helix.Traversal{
		helix.G().CreateTextIndexNodes("Commit", "message"),
		helix.G().CreateTextIndexNodes("Commit", "diff_text"),
		helix.G().CreateTextIndexNodes("File", "path"),
		helix.G().CreateTextIndexNodes("CodeElement", "signature"),
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("Commit", "id")),
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("Commit", "repo_path")),
		helix.G().CreateIndexIfNotExists(helix.NodeRangeIndex("Commit", "timestamp")),
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("Author", "email")),
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("Repo", "path")),
		helix.G().CreateIndexIfNotExists(helix.NodeEqualityIndex("File", "path")),
	}

	for _, idx := range indexes {
		req := helix.WriteQuery("schema_migration").VarAs("_", idx).Returning()
		if err := db.client.Exec(ctx, req, nil); err != nil {
			return fmt.Errorf("schema migration: %w", err)
		}
	}

	return nil
}
