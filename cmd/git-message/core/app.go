package core

import (
	"context"
	"fmt"
	"sync"

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
//
// It is safe for concurrent use: buildPromptContextParallel calls RepoRoot
// and Branch from multiple goroutines on a cold (unmemoised) context, so all
// memoised fields are guarded by mu. The lock is never held across I/O
// (subprocess, filesystem walk); concurrent cold callers may duplicate work
// but the first completed result wins and all callers observe one value.
// resolveAgentFiles calls RepoRoot, so AgentFiles must not hold mu while
// resolving (it releases before the nested call).
type AppContext struct {
	mu     sync.RWMutex
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
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.cfg
}

// Client returns the request client, or nil if InitClient has not been called.
func (a *AppContext) Client() *request.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.client
}

// InitClient creates the request client from the resolved configuration.
// Must be called at most once with a context that supports cancellation.
func (a *AppContext) InitClient(ctx context.Context) error {
	a.mu.RLock()
	apiKey, model := a.cfg.APIKey, a.cfg.Model
	a.mu.RUnlock()

	client, err := request.NewClient(ctx, request.ClientConfig{
		APIKey: apiKey,
		Model:  model,
	})
	if err != nil {
		return fmt.Errorf("failed to create request client: %w", err)
	}

	a.mu.Lock()
	a.client = client
	a.mu.Unlock()

	return nil
}

// AgentFiles returns the AGENTS.md files to mount, memoised per invocation.
//
// Discovery walks the tree, so it is cached the way RepoRoot, Branch, and
// Operation are: the review loop regenerates on demand and must not rescan for
// a result that cannot change mid-run.
//
// Safe for concurrent use. The lock is released before the filesystem walk;
// a nested RepoRoot call re-acquires it, so holding it across resolve would
// deadlock.
func (a *AppContext) AgentFiles(ctx context.Context) []request.AgentFile {
	a.mu.RLock()

	if a.agentsFilesOK {
		files := a.agentsFiles
		a.mu.RUnlock()

		return files
	}

	a.mu.RUnlock()

	files := resolveAgentFiles(ctx, a)

	a.mu.Lock()
	if !a.agentsFilesOK {
		a.agentsFiles = files
		a.agentsFilesOK = true
	} else {
		files = a.agentsFiles
	}
	a.mu.Unlock()

	return files
}

// RepoRoot returns the absolute path to the git repository root, caching the
// result so that repeated calls within the same invocation use the cached
// value and avoid a redundant subprocess spawn.
//
// Safe for concurrent use: buildRepoContext and buildSubmoduleContext call it
// from parallel builders on a cold context.
func (a *AppContext) RepoRoot(ctx context.Context) (string, error) {
	a.mu.RLock()

	if a.repoRootOK {
		root, err := a.repoRoot, a.repoRootErr
		a.mu.RUnlock()

		return root, err
	}

	a.mu.RUnlock()

	root, err := git.GetRepoRoot(ctx)

	a.mu.Lock()
	if !a.repoRootOK {
		a.repoRoot, a.repoRootErr = root, err
		a.repoRootOK = true
	} else {
		root, err = a.repoRoot, a.repoRootErr
	}
	a.mu.Unlock()

	return root, err
}

// Branch returns the current git branch, memoised per invocation. The branch
// cannot change within a single run, so callers pay at most one subprocess
// spawn in the sequential case.
//
// Safe for concurrent use.
func (a *AppContext) Branch(ctx context.Context) string {
	a.mu.RLock()

	if a.branchOK {
		branch := a.branch
		a.mu.RUnlock()

		return branch
	}

	fn := a.branchFn
	a.mu.RUnlock()

	var branch string
	if fn != nil {
		branch = fn(ctx)
	} else {
		branch = git.GetBranch(ctx)
	}

	a.mu.Lock()
	if !a.branchOK {
		a.branch = branch
		a.branchOK = true
	} else {
		branch = a.branch
	}
	a.mu.Unlock()

	return branch
}

// Operation returns the git operation the current commit completes (merge,
// cherry-pick, revert, rebase, squash, fixup), memoised per invocation so the
// state-file probe runs at most once in the sequential case. Returns git.OperationNone for ordinary
// commits or when git state cannot be read.
//
// Safe for concurrent use.
func (a *AppContext) Operation(ctx context.Context) git.Operation {
	a.mu.RLock()

	if a.operationOK {
		op := a.operation
		a.mu.RUnlock()

		return op
	}

	fn := a.operationFn
	a.mu.RUnlock()

	var op git.Operation
	if fn != nil {
		op = fn(ctx)
	} else {
		op = git.DetectOperation(ctx)
	}

	a.mu.Lock()
	if !a.operationOK {
		a.operation = op
		a.operationOK = true
	} else {
		op = a.operation
	}
	a.mu.Unlock()

	return op
}
