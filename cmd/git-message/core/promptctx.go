package core

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"gud/internal/git"
	"gud/internal/obs"
)

// contextBuilderTimeout bounds each independent prompt-context builder so one
// slow git/filesystem probe cannot stall generation.
const contextBuilderTimeout = 5 * time.Second

// historyState names the repository fact that decides whether the history
// family is meaningful. It is resolved once per invocation (see
// resolveHistoryState) and threaded into the history and submodule builders
// so no builder issues its own state probe.
type historyState int

const (
	// historyAbsent means the branch is unborn: no commits exist, so no
	// history probe may run and the history slot carries the first-commit
	// statement instead.
	historyAbsent historyState = iota
	// historyPresent means at least one commit exists: history behaves as
	// before.
	historyPresent
	// historyUnknown means the commit-state probe failed: history behaves
	// as before and never claims a first commit.
	historyUnknown
)

// resolveHistoryState asks git once whether the repository has any commits
// and maps the probe answer to a historyState. A probe error degrades to
// historyUnknown rather than asserting an absence: an unreadable repository
// must never be described to the model as a repository with no commits.
func resolveHistoryState(ctx context.Context) historyState {
	has, err := git.HasCommits(ctx)
	if err != nil {
		slog.Debug("commit-state probe failed; history behaviour unchanged", "error", err)

		return historyUnknown
	}

	if !has {
		slog.Debug("commit-history context skipped: repository has no commits", "reason", "unborn branch")

		return historyAbsent
	}

	return historyPresent
}

// buildPromptContextParallel assembles repo, history, submodule, and operation
// fragments concurrently. The commit state is resolved once before the
// builders start; history and submodule receive it and never probe. Each I/O
// builder gets its own timeout; failures degrade to "" (same contract as
// the sequential version). Join order is fixed: repo, history, submodule,
// operation.
func buildPromptContextParallel(ctx context.Context, app *AppContext, diff string, op git.Operation) string {
	tm := obs.Start("ctx.build")
	defer tm.Done()

	state := resolveHistoryState(ctx)

	type slot struct {
		idx int
		val string
	}

	out := make([]string, 3)

	var wg sync.WaitGroup

	ch := make(chan slot, 3)

	builders := []func(context.Context) string{
		func(c context.Context) string { return buildRepoContext(c, app) },
		func(c context.Context) string { return buildHistoryContext(c, app, diff, state) },
		func(c context.Context) string { return buildSubmoduleContext(c, app, diff, state) },
	}

	for i, b := range builders {
		wg.Add(1)

		go func(idx int, fn func(context.Context) string) {
			defer wg.Done()

			bctx, cancel := context.WithTimeout(ctx, contextBuilderTimeout)
			defer cancel()

			ch <- slot{idx: idx, val: fn(bctx)}
		}(i, b)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for s := range ch {
		out[s.idx] = s.val
	}

	merged := joinContexts(joinContexts(out[0], out[1]), out[2])

	return joinContexts(merged, buildOperationContext(op))
}
