package core

import (
	"context"
	"fmt"
	"log/slog"

	"gud/internal/config"
	"gud/internal/config/mediator"
	"gud/internal/git"
	"gud/internal/mem"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

// ConfigGetter allows read-only access to resolved configuration.
type ConfigGetter interface {
	Config() config.Config
}

// AppContext bundles resolved application configuration with the request client
// and optional HelixDB connection. gud never manages a HelixDB server itself:
// it connects to a shared, externally-run instance at the default URL
// , so one database is reused across projects and repos.
type AppContext struct {
	cfg     config.Config
	client  *request.Client
	helixDB *mem.DB

	// Cached values computed once per invocation.
	repoRoot    string
	repoRootErr error
	repoRootOK  bool // true once repoRoot has been computed

	// branch is memoised from the first successful git branch lookup. The
	// branch cannot change within a single invocation, so the MEM persist
	// path must not pay a subprocess spawn for each write.
	branch   string
	branchOK bool
	// branchFn is the branch lookup used by Branch. It is swappable in tests
	// to avoid subprocess spawns; nil means git.GetBranch.
	branchFn func(context.Context) string

	// operation is memoised from the first git operation detection. The
	// in-progress operation (merge, cherry-pick, revert, rebase, squash,
	// fixup) cannot change within a single invocation, so detection runs at
	// most one subprocess probe.
	operation   git.Operation
	operationOK bool
	// operationFn is the operation lookup used by Operation. It is swappable
	// in tests to avoid subprocess spawns; nil means git.DetectOperation.
	operationFn func(context.Context) git.Operation
}

// NewAppContext loads and merges configuration from all sources (CLI flags,
// environment variables, config files) and returns an AppContext with the
// resolved config. The request client is NOT created here — call InitClient
// separately. HelixDB is NOT initialized here — call InitHelixDB separately.
//
// A configured profile must be cached locally; if it is not, an error is
// returned telling the user to download it first (see requireProfile).
func NewAppContext(cmd *cobra.Command) (*AppContext, error) {
	return newAppContext(cmd, true)
}

// NewAppContextTolerant is like NewAppContext but tolerates a configured
// profile that is not cached: the profile content simply degrades to "" —
// the same behaviour resolveProfileContent already has — instead of failing.
// It is used by hook mode, where a hard error would abort the user's git
// commit.
func NewAppContextTolerant(cmd *cobra.Command) (*AppContext, error) {
	return newAppContext(cmd, false)
}

// newAppContext loads and merges configuration from all sources and returns
// an AppContext with the resolved config. When requireCachedProfile is true,
// a configured but uncached profile is a hard error; when false, it degrades
// gracefully to an empty profile.
func newAppContext(cmd *cobra.Command, requireCachedProfile bool) (*AppContext, error) {
	cliCfg := configFromCmd(cmd)

	m, err := mediator.New()
	if err != nil {
		return nil, fmt.Errorf("config mediator: %w", err)
	}

	cfg, err := m.Load(cliCfg)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	if requireCachedProfile {
		if err := requireProfile(string(cfg.Profile)); err != nil {
			return nil, err
		}
	}

	return &AppContext{
		cfg: cfg,
	}, nil
}

// Config returns the resolved application configuration.
func (a *AppContext) Config() config.Config {
	return a.cfg
}

// setProfile updates the profile in the resolved config.
// This is used by the suggestion flow to apply a newly chosen profile
// immediately without requiring a config reload.
func (a *AppContext) setProfile(name config.ProfileName) {
	a.cfg.Profile = name
}

// Client returns the request client, or nil if InitClient has not been called.
func (a *AppContext) Client() *request.Client {
	return a.client
}

// HelixDB returns the HelixDB connection, or nil if not initialized.
func (a *AppContext) HelixDB() *mem.DB {
	return a.helixDB
}

// InitClient creates the request client from the resolved configuration.
// Must be called at most once with a context that supports cancellation.
func (a *AppContext) InitClient(ctx context.Context) error {
	client, err := request.NewClient(ctx, request.ClientConfig{
		APIKey: a.cfg.APIKey,
		Model:  a.cfg.Model,
	})
	if err != nil {
		return fmt.Errorf("failed to create request client: %w", err)
	}
	a.client = client

	return nil
}

// InitHelixDB creates the HelixDB connection. Memory is always enabled: gud
// connects to a shared, externally-run HelixDB server at the default URL
//
// Schema migration (EnsureSchema) runs whenever the DB is reachable. It is
// idempotent and fast (verified ~15ms on a warm server), and guarantees a
// pre-existing server never misses the indexes.
func (a *AppContext) InitHelixDB(ctx context.Context) error {
	db := mem.NewDB(mem.Options{Enabled: true})

	if !db.Enabled() {
		slog.Debug("helixdb: client creation failed, degraded mode")

		return nil
	}

	if !db.IsAvailable(ctx) {
		slog.Debug("helixdb: server not reachable, degraded mode", "url", db.BaseURL())

		return nil
	}

	// EnsureSchema is idempotent and cheap on a warm server, so always run it
	// rather than assuming a pre-existing server already has the schema.
	if err := db.EnsureSchema(ctx); err != nil {
		return fmt.Errorf("helixdb schema: %w", err)
	}

	a.helixDB = db

	return nil
}

// RepoRoot returns the absolute path to the git repository root, caching the
// result so that repeated calls within the same invocation use the cached
// value and avoid a redundant subprocess spawn.
func (a *AppContext) RepoRoot(ctx context.Context) (string, error) {
	if !a.repoRootOK {
		a.repoRoot, a.repoRootErr = git.GetRepoRoot(ctx)
		a.repoRootOK = true
	}

	return a.repoRoot, a.repoRootErr
}

// Branch returns the current git branch, memoised per invocation. The branch
// cannot change within a single run, so callers pay at most one subprocess
// spawn even when persistence needs it.
func (a *AppContext) Branch(ctx context.Context) string {
	if !a.branchOK {
		if a.branchFn != nil {
			a.branch = a.branchFn(ctx)
		} else {
			a.branch = git.GetBranch(ctx)
		}
		a.branchOK = true
	}

	return a.branch
}

// Operation returns the git operation the current commit completes (merge,
// cherry-pick, revert, rebase, squash, fixup), memoised per invocation so the
// state-file probe runs at most once. Returns git.OperationNone for ordinary
// commits or when git state cannot be read.
func (a *AppContext) Operation(ctx context.Context) git.Operation {
	if !a.operationOK {
		if a.operationFn != nil {
			a.operation = a.operationFn(ctx)
		} else {
			a.operation = git.DetectOperation(ctx)
		}
		a.operationOK = true
	}

	return a.operation
}
