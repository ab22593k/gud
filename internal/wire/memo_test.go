package wire

import (
	"context"
	"strings"
	"testing"
)

func TestMemoRoundTrip(t *testing.T) {
	t.Parallel()

	m := newMemo()
	key := "github.com\x00o\x00r\x0019.0"

	if _, ok := m.get(key); ok {
		t.Fatal("empty memo hit")
	}

	want := Resolution{Commit: strings.Repeat("a", 40), Ref: "19.0", Subpath: "p"}
	m.set(key, want)

	got, ok := m.get(key)
	if !ok || got != want {
		t.Fatalf("got (%+v,%v), want resolution + hit", got, ok)
	}
}

func TestMemoizeResolvesOnce(t *testing.T) {
	t.Parallel()

	inner := &fakeFetcher{
		commits: map[string]string{"19.0": strings.Repeat("a", 40)},
	}

	f := Memoize(inner, nil)
	src := SourceRef{Host: "h", Owner: "o", Repo: "r", Ref: "19.0", Subpath: "p"}

	if _, err := f.Resolve(context.Background(), src); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if _, err := f.Resolve(context.Background(), src); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if inner.resolves != 1 {
		t.Fatalf("inner resolves = %d, want 1", inner.resolves)
	}
}

func TestMemoizePassesFailures(t *testing.T) {
	t.Parallel()

	inner := &fakeFetcher{}
	f := Memoize(inner, nil)
	src := SourceRef{Ref: "missing"}

	if _, err := f.Resolve(context.Background(), src); err == nil {
		t.Fatal("expected error")
	}

	if _, err := f.Resolve(context.Background(), src); err == nil {
		t.Fatal("expected error")
	}

	if inner.resolves != 2 {
		t.Fatalf("failures must not memoize, resolves = %d", inner.resolves)
	}
}
