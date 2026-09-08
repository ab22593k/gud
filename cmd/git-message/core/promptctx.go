package core

import (
	"context"
	"sync"
	"time"

	"gud/internal/git"
)

// contextBuilderTimeout bounds each independent prompt-context builder so one
// slow git/filesystem probe cannot stall generation.
const contextBuilderTimeout = 5 * time.Second

// buildPromptContextParallel assembles repo, history, submodule, and operation
// fragments concurrently. Each I/O builder gets its own timeout; failures
// degrade to "" (same contract as the sequential version). Join order is
// fixed: repo, history, submodule, operation.
func buildPromptContextParallel(ctx context.Context, app *AppContext, diff string, op git.Operation) string {
	type slot struct {
		idx int
		val string
	}

	out := make([]string, 3)

	var wg sync.WaitGroup

	ch := make(chan slot, 3)

	builders := []func(context.Context) string{
		func(c context.Context) string { return buildRepoContext(c, app) },
		func(c context.Context) string { return buildHistoryContext(c, app, diff) },
		func(c context.Context) string { return buildSubmoduleContext(c, app, diff) },
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
