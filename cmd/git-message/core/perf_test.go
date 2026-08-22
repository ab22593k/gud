package core

import (
	"context"
	"testing"
)

// TestBranchMemoisesPerInvocation guards the branch subprocess dedup:
// persistence previously spawned `git rev-parse --abbrev-ref HEAD` more than
// once per invocation; the memo ensures a single spawn.
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
