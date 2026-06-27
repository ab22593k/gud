# Application Context — Configuration Injection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Formalize configuration injection by creating an `AppContext` struct that bundles resolved `config.Config` and eliminates duplicated mediator+client setup in entry points.

**Architecture:** A thin `AppContext` struct in the `core` package holds the immutable resolved config. Its constructor (`NewAppContext`) owns the mediator loading pipeline (replaces the duplicated `configFromCmd` → `mediator.New()` → `m.Load()` pattern in `runGenerate` and `runHookMode`). Function signatures throughout change from accepting `config.Config` + separately-created `*request.Client` to accepting a single `*AppContext` — client creation becomes a method on AppContext.

**Tech Stack:** Go 1.26.3, no new dependencies.

## Global Constraints

- All existing tests must pass unchanged (signature changes must be compatible or updated)
- `golangci-lint` must pass with zero issues
- No global variables or package-level state introduced
- `AppContext` is NOT a DI container — it's a simple value object bundling config + client
- `request.Client` creation stays separate from config loading (needs `context.Context` from caller for signal handling)

---

### Task 1: Create `core/app.go` — AppContext struct + constructor

**Files:**
- Create: `cmd/git-message/core/app.go`
- Test: none (implicitly tested by subsequent task changes)

**Interfaces:**
- Produces: `AppContext` struct with `Config() config.Config`, `Client() *request.Client`, `InitClient(ctx) error`

- [ ] **Step 1: Write `core/app.go`**

```go
package core

import (
	"context"
	"fmt"
	"log/slog"

	"gud/internal/config"
	"gud/internal/config/mediator"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

// AppContext bundles resolved application configuration with the request client.
// Create with NewAppContext, then call InitClient with a cancellable context.
type AppContext struct {
	cfg    config.Config
	client *request.Client
}

// NewAppContext loads and merges configuration from all sources (CLI flags,
// environment variables, config files) and returns an AppContext with the
// resolved config. The request client is NOT created here — call InitClient
// separately with a context that carries timeouts/cancellation.
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

	if cfg.APIKey == "" {
		slog.Debug("no API key configured; opencode provider does not require one")
	}

	if err := requireProfile(string(cfg.Profile)); err != nil {
		return nil, err
	}

	return &AppContext{cfg: cfg}, nil
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
		APIKey:      a.cfg.APIKey,
		Model:       a.cfg.Model,
		Temperature: a.cfg.Temperature,
		ACP:         string(a.cfg.ACP),
	})
	if err != nil {
		return fmt.Errorf("failed to create request client: %w", err)
	}
	a.client = client
	return nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/abdelwahab/organicode/go/gud && go build ./cmd/git-message/...`
Expected: success (no test for this step; implicit coverage from later tasks)

---

### Task 2: Simplify `generate.go` — use AppContext

**Files:**
- Modify: `cmd/git-message/core/generate.go`
- Test: `cmd/git-message/core/core_test.go` (build history tests use `buildHistoryContext`)

**Interfaces:**
- Consumes: `AppContext` from Task 1
- Changes: `runGenerate` uses `NewAppContext` + `InitClient`, `buildHistoryContext` accepts `*AppContext`

- [ ] **Step 1: Update `runGenerate` to use AppContext**

Replace lines 26-67 with:

```go
func runGenerate(cmd *cobra.Command, _ []string) error {
	app, err := NewAppContext(cmd)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.InitClient(ctx); err != nil {
		return err
	}

	diff, err := getStagedDiffOrError(ctx)
	if err != nil {
		return err
	}

	promptContext := buildHistoryContext(ctx, app)

	return interactiveCommit(ctx, cmd, app, diff, promptContext)
}
```

- [ ] **Step 2: Update `buildHistoryContext` signature**

Change from `buildHistoryContext(ctx context.Context, cfg config.Config) string` to `buildHistoryContext(ctx context.Context, app *AppContext) string`

Inside the function, change `cfg.History` to `app.Config().History`.

Full function:

```go
func buildHistoryContext(ctx context.Context, app *AppContext) string {
	var history string
	if app.Config().History > 0 {
		var err error
		history, err = git.GetRecentCommits(ctx, app.Config().History)
		if err != nil {
			slog.Debug("failed to get recent commits, proceeding without history", "error", err)
			history = ""
		}
	}
	if history != "" {
		history = "Recent commits:\n" + history
	}

	return history
}
```

---

### Task 3: Simplify `interact.go` — use AppContext

**Files:**
- Modify: `cmd/git-message/core/interact.go`

**Interfaces:**
- Consumes: `AppContext` from Task 1
- Changes: `interactiveCommit` accepts `*AppContext` instead of `*request.Client` + `config.Config`

- [ ] **Step 1: Update `interactiveCommit` signature**

Change from:
```go
func interactiveCommit(ctx context.Context, cmd *cobra.Command, client *request.Client,
	diff, promptContext string, cfg config.Config) error {
```

To:
```go
func interactiveCommit(ctx context.Context, cmd *cobra.Command, app *AppContext,
	diff, promptContext string) error {
```

Inside, replace `client` with `app.Client()`, `cfg.DetailLevel` with `app.Config().DetailLevel`, etc.:

```go
func interactiveCommit(ctx context.Context, cmd *cobra.Command, app *AppContext,
	diff, promptContext string) error {
	scanner := bufio.NewScanner(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	cfg := app.Config()

	for {
		profileContent := resolveProfileContent(string(cfg.Profile))
		msg, err := showProgress("Rolling in, obscuring the landscape of the codebase...", func() (string, error) {
			return app.Client().GenerateCommitMessageWithContent(ctx, diff, promptContext, request.DetailLevel(cfg.DetailLevel),
				cfg.Hint, request.ProfileName(cfg.Profile), profileContent, cfg.WrapLine)
		})
		if err != nil {
			return fmt.Errorf("failed to generate commit message: %w", err)
		}

		_, _ = fmt.Fprintln(out, "")
		_, _ = fmt.Fprintln(out, msg)
		_, _ = fmt.Fprintln(out, "")

		action := promptAction(scanner, out)
		switch action {
		case actionCommit:
			msg = appendAssistedBy(msg, app.Client().ModelName())
			if err := git.Commit(ctx, msg); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, "Committed successfully.")

			return nil

		case actionEdit:
			edited, err := editMessage(msg)
			if err != nil {
				return fmt.Errorf("failed to edit message: %w", err)
			}
			edited = appendAssistedBy(edited, app.Client().ModelName())
			if err := git.Commit(ctx, edited); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, "Committed successfully.")

			return nil

		case actionRegenerate:
			continue

		case actionAbort:
			_, _ = fmt.Fprintln(out, "Aborted.")

			return nil
		}
	}
}
```

---

### Task 4: Simplify `hook.go` — use AppContext

**Files:**
- Modify: `cmd/git-message/core/hook.go`

**Interfaces:**
- Consumes: `AppContext` from Task 1
- Changes: `runHookMode`, `runHookModeInternal`, `generateAndWriteMsg` use AppContext

- [ ] **Step 1: Simplify `runHookMode` to use AppContext**

Replace:
```go
func runHookMode(cmd *cobra.Command, msgFile string) error {
	cliCfg := configFromCmd(cmd)

	m, err := mediator.New()
	if err != nil {
		return fmt.Errorf("config mediator: %w", err)
	}

	cfg, err := m.Load(cliCfg)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx := context.Background()

	return runHookModeInternal(ctx, msgFile, cfg)
}
```

With:
```go
func runHookMode(cmd *cobra.Command, msgFile string) error {
	app, err := NewAppContext(cmd)
	if err != nil {
		return err
	}

	ctx := context.Background()

	return runHookModeInternal(ctx, msgFile, app)
}
```

- [ ] **Step 2: Update `runHookModeInternal` signature**

Change from:
```go
func runHookModeInternal(ctx context.Context, msgFile string, cfg config.Config) error {
```

To:
```go
func runHookModeInternal(ctx context.Context, msgFile string, app *AppContext) error {
```

Replace the inline `request.NewClient(...)` block and `cfg` references:

```go
func runHookModeInternal(ctx context.Context, msgFile string, app *AppContext) error {
	diff, err := getStagedDiffOrSkip(ctx)
	if err != nil {
		return fmt.Errorf("failed to get staged diff: %w", err)
	}
	if diff == "" {
		return nil
	}

	deleted, err := git.GetStagedDeletedFiles(ctx)
	if err != nil {
		return fmt.Errorf("failed to get deleted files: %w", err)
	}
	diff = appendDeletedContext(diff, deleted)

	if err := app.InitClient(ctx); err != nil {
		return err
	}

	return generateAndWriteMsg(ctx, app, diff, msgFile)
}
```

- [ ] **Step 3: Update `generateAndWriteMsg` signature**

Change from:
```go
func generateAndWriteMsg(ctx context.Context, client *request.Client, diff, msgFile string, cfg config.Config) error {
```

To:
```go
func generateAndWriteMsg(ctx context.Context, app *AppContext, diff, msgFile string) error {
```

Inside, replace `client` with `app.Client()`, `cfg.Profile` with `app.Config().Profile`, etc.:

```go
func generateAndWriteMsg(ctx context.Context, app *AppContext, diff, msgFile string) error {
	cfg := app.Config()
	profileContent := resolveProfileContent(string(cfg.Profile))
	msg, err := app.Client().GenerateCommitMessageWithContent(ctx, diff, "", request.DetailLevel(cfg.DetailLevel), cfg.Hint,
		request.ProfileName(cfg.Profile), profileContent, cfg.WrapLine)
	if err != nil {
		return fmt.Errorf("failed to generate commit message: %w", err)
	}

	msg = appendAssistedBy(msg, app.Client().ModelName())

	if err := os.WriteFile(msgFile, []byte(msg), 0600); err != nil {
		return fmt.Errorf("failed to write message file: %w", err)
	}

	return nil
}
```

---

### Task 5: Clean up unused imports in hook.go

After removing `"gud/internal/config/mediator"` from hook.go imports (no longer needed since mediator is handled by `NewAppContext`):

- [ ] **Step 1: Remove unused imports from hook.go**

Remove `"gud/internal/config/mediator"` from the import block. Keep `"gud/internal/config"` — it may still be needed... wait, let me check.

`hook.go` currently imports `config` for `config.Config` type annotation. After the refactor, `config.Config` is no longer in any function signature in `hook.go` (it's accessed through `app.Config()`). So we can remove `"gud/internal/config"` too.

Remove both: `"gud/internal/config"` and `"gud/internal/config/mediator"` from hook.go's import block. Also remove `"context"` since it's still needed.

Run: `cd /home/abdelwahab/organicode/go/gud && go build ./cmd/git-message/...` to verify.

Wait, `context` IS still used in `runHookModeInternal`. And `request` is still used... actually no, `request.NewClient` is now called inside `app.InitClient`. But `request.DetailLevel`, `request.ProfileName` are used in `generateAndWriteMsg`. So `request` import stays.

Let me verify: hook.go currently imports:
```go
"context"
"fmt"
"os"
"path/filepath"
"strings"

"gud/internal/config"
"gud/internal/config/mediator"
"gud/internal/git"
"gud/internal/request"

"github.com/spf13/cobra"
```

After refactor:
- `context` — still used in `runHookModeInternal` (ctx param)
- `fmt` — still used
- `os` — still used
- `path/filepath` — still used
- `strings` — still used
- `gud/internal/config` — remove (no more `config.Config` type in signatures)
- `gud/internal/config/mediator` — remove (mediated via `NewAppContext`)
- `gud/internal/git` — still used
- `gud/internal/request` — still used in `generateAndWriteMsg` for type conversions
- `cobra` — still used

So remove `"gud/internal/config"` and `"gud/internal/config/mediator"`.

---

### Task 6: Clean up unused imports in generate.go

After moving mediator + API key check + requireProfile into `NewAppContext`:

- [ ] **Step 1: Remove unused imports from generate.go**

Current imports:
```go
"gud/internal/config"
"gud/internal/config/mediator"
"gud/internal/git"
"gud/internal/request"
```

- `config` — still used in `core_test.go` but NOT in `generate.go` anymore (function signatures changed to `*AppContext`)
- `mediator` — remove (handled by `NewAppContext`)
- Actually, `config` is not imported in `generate.go` at all after the refactor since `buildHistoryContext` now takes `*AppContext`.

Run: `cd /home/abdelwahab/organicode/go/gud && go build ./cmd/git-message/...` to verify.

---

### Task 7: Update tests

**Files:**
- Modify: `cmd/git-message/core/core_test.go`

The `TestBuildHistoryContext_Disabled` test calls `buildHistoryContext(ctx, tt.cfg)`. After the signature change, it needs an `*AppContext`.

- [ ] **Step 1: Update `TestBuildHistoryContext_Disabled`**

Change from:
```go
got := buildHistoryContext(ctx, tt.cfg)
```

To:
```go
app := &AppContext{cfg: tt.cfg}
got := buildHistoryContext(ctx, app)
```

Also need to adjust the `AppContext` struct — it has an unexported `cfg` field, so tests in the same package (`package core`) can access it directly.

But wait — the test also doesn't need `client` (the client is nil, which is fine because `buildHistoryContext` only accesses config). So this should work.

---

### Task 8: Build, vet, test, lint — green across the board

- [ ] **Step 1: Build**

Run: `cd /home/abdelwahab/organicode/go/gud && go build ./...`

- [ ] **Step 2: Vet**

Run: `cd /home/abdelwahab/organicode/go/gud && go vet ./...`

- [ ] **Step 3: Test**

Run: `cd /home/abdelwahab/organicode/go/gud && go test ./cmd/git-message/... -count=1 -v`

- [ ] **Step 4: Lint**

Run: `cd /home/abdelwahab/organicode/go/gud && golangci-lint run ./internal/config/... ./cmd/git-message/...`
