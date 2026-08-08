package core

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"gud/internal/request"
)

// TestEmbedDiffMemoisesPerInvocation guards the recall/persist sharing: the
// same diff must be embedded at most once per invocation. Before this test,
// maybeAppendMEMContext and persistToHelixDB each called EmbedText(diff),
// doubling embedding network round-trips on every committed message.
func TestEmbedDiffMemoisesPerInvocation(t *testing.T) {
	app := &AppContext{}
	app.client = request.NewClientWithGenerator(nil, "mock")
	calls := atomic.Int32{}
	app.client.SetEmbedder(func(_ context.Context, _ string) ([]float32, error) {
		calls.Add(1)

		return []float32{1, 2, 3}, nil
	})

	diff := "diff with staged changes"

	vec1, err := app.EmbedDiff(context.Background(), diff)
	if err != nil {
		t.Fatalf("first EmbedDiff: %v", err)
	}
	vec2, err := app.EmbedDiff(context.Background(), diff)
	if err != nil {
		t.Fatalf("second EmbedDiff: %v", err)
	}

	if calls.Load() != 1 {
		t.Errorf("embedder called %d times for the same diff, want 1", calls.Load())
	}
	if len(vec1) != 3 || len(vec2) != 3 {
		t.Fatalf("vectors: got len %d and %d, want 3 each", len(vec1), len(vec2))
	}
}

// TestEmbedDiffMemoisesErrorPerInvocation ensures a failed embedding is also
// memoised: persist must not retry a known-bad embed (e.g. invalid API key).
func TestEmbedDiffMemoisesErrorPerInvocation(t *testing.T) {
	app := &AppContext{}
	app.client = request.NewClientWithGenerator(nil, "mock")
	calls := atomic.Int32{}
	app.client.SetEmbedder(func(_ context.Context, _ string) ([]float32, error) {
		calls.Add(1)

		return nil, errors.New("embed failure")
	})

	_, err1 := app.EmbedDiff(context.Background(), "diff")
	_, err2 := app.EmbedDiff(context.Background(), "diff")
	if err1 == nil || err2 == nil {
		t.Fatalf("expected errors, got %v and %v", err1, err2)
	}
	if calls.Load() != 1 {
		t.Errorf("embedder called %d times for the same failed diff, want 1", calls.Load())
	}
}

// TestEmbedDiffDifferentTextRecomputes ensures the memo is keyed by content,
// not a blanket once-per-invocation cache.
func TestEmbedDiffDifferentTextRecomputes(t *testing.T) {
	app := &AppContext{}
	app.client = request.NewClientWithGenerator(nil, "mock")
	calls := atomic.Int32{}
	app.client.SetEmbedder(func(_ context.Context, text string) ([]float32, error) {
		calls.Add(1)

		return []float32{float32(len(text))}, nil
	})

	_, _ = app.EmbedDiff(context.Background(), "one")
	_, _ = app.EmbedDiff(context.Background(), "two")

	if calls.Load() != 2 {
		t.Errorf("embedder called %d times for two distinct diffs, want 2", calls.Load())
	}
}

// TestBranchMemoisesPerInvocation guards the branch subprocess dedup: recall
// and persist previously each spawned `git rev-parse --abbrev-ref HEAD`,
// tripling a spawn that only needs to happen once per invocation.
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
