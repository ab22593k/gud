package core

import (
	"context"
	"fmt"

	"gud/internal/config"
	"gud/internal/config/mediator"
	"gud/internal/git"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

// ConfigGetter allows read-only access to resolved configuration.
type ConfigGetter interface {
	Config() config.Config
}

// AppContext bundles resolved application configuration with the request client.
type AppContext struct {
	cfg    config.Config
	client *request.Client

	// Cached values computed once per invocation.
	repoRoot    string
	repoRootErr error
	repoRootOK  bool // true once repoRoot has been computed

	// branch is memoised from the first successful git branch lookup. The
	// branch cannot change within a single invocation, so callers pay
	// at most one subprocess spawn.
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

	// agentsFiles is memoised from the first AGENTS.md discovery. The review
	// loop regenerates on demand and would otherwise rescan the tree — a
	// filesystem walk — on every pass, for a result that cannot change within
	// one invocation.
	agentsFiles   []request.AgentFile
	agentsFilesOK bool
}

// NewAppContext loads and merges configuration from all sources (CLI flags,
// environment variables, config files) and returns an AppContext with the
// resolved config. The request client is NOT created here — call InitClient
// separately.
func NewAppContext(cmd *cobra.Command) (*AppContext, error) {
	cliCfg := configFromCmd(cmd)

	m, err := mediator.New()
	if err != nil {
		return nil, fmt.Errorf("config mediator: %w", err)
	}

	cfg, err := m.Load(cliCfg)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	return &AppContext{
		cfg: cfg,
	}, nil
}

// Config returns the resolved application configuration.
func (a *AppContext) Config() config.Config {
	return a.cfg
}

// Client returns the request client, or nil if InitClient has not been called.
func (a *AppContext) Client() *request.Client {
	return a.client
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

// AgentFiles returns the AGENTS.md files to mount, memoised per invocation.
//
// Discovery walks the tree, so it is cached the way RepoRoot, Branch, and
// Operation are: the review loop regenerates on demand and must not rescan for
// a result that cannot change mid-run.
func (a *AppContext) AgentFiles(ctx context.Context) []request.AgentFile {
	if !a.agentsFilesOK {
		a.agentsFiles = resolveAgentFiles(ctx, a)
		a.agentsFilesOK = true
	}

	return a.agentsFiles
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
// spawn.
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
