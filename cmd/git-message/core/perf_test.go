package core

import (
	"context"
	"testing"

	"gud/internal/request"
)

// TestBranchMemoisesPerInvocation guards the branch subprocess dedup:
// the memo ensures a single spawn per invocation.
func TestBranchMemoisesPerInvocation(t *testing.T) {
	app := &AppContext{}
	calls := 0
	app.branchFn = func(context.Context) string {
		calls++

		return "main"
	}

	if got := app.Branch(context.Background()); got != "main" {
		t.Fatalf("Branch() = %q, want main", got)
	}

	if got := app.Branch(context.Background()); got != "main" {
		t.Fatalf("second Branch() = %q, want main", got)
	}

	if calls != 1 {
		t.Errorf("branch lookup ran %d times, want 1 (memoised)", calls)
	}
}

// TestAgentFilesMemoisesPerInvocation guards the discovery memo. The review
// loop regenerates on demand, and each pass would otherwise rescan the tree —
// a full filesystem walk — for a result that cannot change mid-run.
//
// The memo is observed white-box: seeding the cache with a sentinel and reading
// it back proves the second call does not rescan, which is the whole contract.
// A version without the memo would ignore the seed and rescan.
func TestAgentFilesMemoisesPerInvocation(t *testing.T) {
	app := &AppContext{}

	const sentinel = "sentinel instructions"

	app.agentsFiles = []request.AgentFile{{Path: request.RootMountPath, Content: sentinel}}
	app.agentsFilesOK = true

	got := app.AgentFiles(context.Background())
	if len(got) != 1 || got[0].Content != sentinel {
		t.Fatalf("AgentFiles() = %+v, want the memoised sentinel", got)
	}

	// A fresh context must not have its cache pre-seeded by a previous
	// invocation's state.
	fresh := &AppContext{}
	if fresh.agentsFilesOK {
		t.Error("a new AppContext must start with discovery unmemoised")
	}
}
