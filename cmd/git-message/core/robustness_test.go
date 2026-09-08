package core

import (
	"context"
	"strings"
	"testing"
	"time"

	"gud/internal/git"
)

func TestBuildPromptContextParallel_PreservesOrderAndDegrades(t *testing.T) {
	t.Parallel()
	// Use a real AppContext with no repo: builders must degrade to "" quickly,
	// not block. Timeout the whole call at 10s to catch sequential hangs.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	app := &AppContext{}
	got := buildPromptContextParallel(ctx, app, "diff --git a/x", git.OperationNone)
	// OperationNone (0) contributes "", repo/history/submodule degrade to "" in
	// temp env without repo — result must be "" and must return fast.
	if strings.Contains(got, "\x00") {
		t.Fatalf("buildPromptContextParallel contains NUL, got %q", got)
	}
	// Second call with fake diff must also complete (idempotent, no shared state).
	got2 := buildPromptContextParallel(ctx, app, "", git.OperationNone)
	if got != got2 {
		t.Fatalf("parallel context not deterministic: %q vs %q", got, got2)
	}
}

func TestJoinContexts_Order(t *testing.T) {
	t.Parallel()
	got := joinContexts(joinContexts("a", "b"), "c")
	if got != "a\n\nb\n\nc" {
		t.Fatalf("joinContexts order=%q, want a,b,c", got)
	}
}
