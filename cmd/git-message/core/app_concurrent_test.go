package core

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gud/internal/config"
	"gud/internal/git"
)

// TestAppContextConcurrentBranchSafe is the regression lock for the
// buildPromptContextParallel data race: Branch memoised branch/branchOK
// without synchronisation, so concurrent callers raced on a cold context.
// Run with -race; it fails before the RWMutex fix.
func TestAppContextConcurrentBranchSafe(t *testing.T) {
	t.Parallel()

	app := &AppContext{}
	app.branchFn = func(context.Context) string {
		time.Sleep(5 * time.Millisecond)

		return "main"
	}

	const callers = 8

	var wg sync.WaitGroup

	got := make([]string, callers)

	for i := range callers {
		wg.Go(func() {
			got[i] = app.Branch(context.Background())
		})
	}

	wg.Wait()

	for i, b := range got {
		if b != "main" {
			t.Fatalf("Branch()[%d] = %q, want main (inconsistent memo under concurrency)", i, b)
		}
	}
}

// TestAppContextConcurrentRepoRootSafe covers the exact parallel-builder
// race: buildRepoContext and buildSubmoduleContext both call RepoRoot on one
// cold AppContext. Hammering RepoRoot directly is the minimal reproducer.
func TestAppContextConcurrentRepoRootSafe(t *testing.T) {
	t.Parallel()

	app := &AppContext{}

	const callers = 8

	start := make(chan struct{})

	var wg sync.WaitGroup

	roots := make([]string, callers)
	errs := make([]error, callers)

	for i := range callers {
		wg.Go(func() {
			<-start

			roots[i], errs[i] = app.RepoRoot(context.Background())
		})
	}

	close(start)
	wg.Wait()

	for i := 1; i < callers; i++ {
		if roots[i] != roots[0] {
			t.Fatalf("RepoRoot()[%d] = %q, want %q (inconsistent memo)", i, roots[i], roots[0])
		}

		if (errs[i] == nil) != (errs[0] == nil) {
			t.Fatalf("RepoRoot() error mismatch: %v vs %v", errs[i], errs[0])
		}
	}
}

// TestAppContextConcurrentOperationSafe guards the same memo pattern for
// Operation: state-file probe must be safe when probed concurrently.
func TestAppContextConcurrentOperationSafe(t *testing.T) {
	t.Parallel()

	app := &AppContext{}
	app.operationFn = func(context.Context) git.Operation {
		time.Sleep(5 * time.Millisecond)

		return git.OperationMerge
	}

	const callers = 8

	var wg sync.WaitGroup

	got := make([]git.Operation, callers)

	for i := range callers {
		wg.Go(func() {
			got[i] = app.Operation(context.Background())
		})
	}

	wg.Wait()

	for i, op := range got {
		if op != git.OperationMerge {
			t.Fatalf("Operation()[%d] = %q, want merge", i, op)
		}
	}
}

// TestBuildPromptContextParallelColdSubmoduleSafe reproduces the production
// trigger in one call: a gitlink diff makes buildSubmoduleContext call
// RepoRoot concurrently with buildRepoContext on a cold context. A plain file
// diff early-returns before the second RepoRoot and hides the race, so the
// 160000 mode is required. History must be enabled or buildHistoryContext
// returns before Branch.
func TestBuildPromptContextParallelColdSubmoduleSafe(t *testing.T) {
	t.Parallel()

	submoduleDiff := "diff --git a/sub b/sub\n" +
		"index 63058a7..84a348a 160000\n" +
		"--- a/sub\n+++ b/sub\n" +
		"@@ -1 +1 @@\n" +
		"-Subproject commit 63058a7ee2dbeaa7d173fc0ce7d16696d2ca9a3d\n" +
		"+Subproject commit 84a348a87712d8fbddb88172c5dd34174d04f312\n"

	for range 20 {
		app := &AppContext{cfg: config.Config{History: config.Ptr(5)}}
		app.branchFn = func(context.Context) string {
			time.Sleep(5 * time.Millisecond)

			return "main"
		}

		_ = buildPromptContextParallel(context.Background(), app, submoduleDiff, git.OperationNone)
	}
}

// TestBuildPromptContextParallelSharedAppSafe guards shared-AppContext use:
// concurrent builds must observe one memoised value, not race on the flags.
func TestBuildPromptContextParallelSharedAppSafe(t *testing.T) {
	t.Parallel()

	app := &AppContext{cfg: config.Config{History: config.Ptr(5)}}
	app.branchFn = func(context.Context) string {
		time.Sleep(5 * time.Millisecond)

		return "main"
	}

	const callers = 8

	var wg sync.WaitGroup

	for range callers {
		wg.Go(func() {
			_ = buildPromptContextParallel(context.Background(), app, "diff --git a/x b/x", git.OperationNone)
		})
	}

	wg.Wait()
}

// TestAppContextConcurrentAgentFilesNoDeadlock guards the nested-lock path:
// AgentFiles calls RepoRoot, so holding the memo lock across the filesystem
// walk would self-deadlock. Concurrent cold discoveries must complete.
func TestAppContextConcurrentAgentFilesNoDeadlock(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, agentsFileName), []byte("concurrent\n"), 0600); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	t.Chdir(dir)

	app := &AppContext{}

	const callers = 4

	var wg sync.WaitGroup

	for range callers {
		wg.Go(func() {
			_ = app.AgentFiles(context.Background())
			_, _ = app.RepoRoot(context.Background())
		})
	}

	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("concurrent AgentFiles/RepoRoot deadlocked")
	}
}
