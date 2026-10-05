package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// listFixture fetches two checkouts of the fake source under root, tracked
// by the run-level registry at root.
func listFixture(t *testing.T, f *fakeFetcher, root string) (first, second string) {
	t.Helper()

	registry := RegistryPath(root)
	first = filepath.Join(root, "one")
	second = filepath.Join(root, "two")

	for _, target := range []string{first, second} {
		opts := FetchOptions{Fetcher: f, RegistryPath: registry}

		if _, err := Fetch(context.Background(), opts, testSource(), target); err != nil {
			t.Fatalf("fixture fetch: %v", err)
		}
	}

	return first, second
}

func entryByDir(entries []Entry, dir string) Entry {
	for _, e := range entries {
		if e.Dir == dir {
			return e
		}
	}

	return Entry{}
}

func TestListEmpty(t *testing.T) {
	t.Parallel()

	entries, err := List(context.Background(), successFetcher(), t.TempDir())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf("entries = %v", entries)
	}
}

func TestListMissingRootEmpty(t *testing.T) {
	t.Parallel()

	// A missing root holds no registry file, which reads as empty.
	entries, err := List(context.Background(), successFetcher(), filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf("entries = %v, want empty", entries)
	}
}

func TestListCurrent(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	root := t.TempDir()
	first, second := listFixture(t, f, root)

	entries, err := List(context.Background(), f, root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}

	for _, dir := range []string{first, second} {
		e := entryByDir(entries, dir)
		if e.State != StateCurrent {
			t.Fatalf("%s state = %q, want current", dir, e.State)
		}

		if e.RemoteSHA != testCommitA || e.Entry.ResolvedCommit != testCommitA {
			t.Fatalf("%s SHAs = %q/%q", dir, e.RemoteSHA, e.Entry.ResolvedCommit)
		}
	}
}

func TestListBehindAndDiverged(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	root := t.TempDir()
	first, second := listFixture(t, f, root)

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{"a.txt": "new"}

	writeFile(t, second, "a.txt", "local edits")

	entries, err := List(context.Background(), f, root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if got := entryByDir(entries, first).State; got != StateBehind {
		t.Fatalf("first = %q, want behind", got)
	}

	if got := entryByDir(entries, second).State; got != StateDiverged {
		t.Fatalf("second = %q, want diverged", got)
	}
}

func TestListMissingTargetDiverged(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	root := t.TempDir()
	first, _ := listFixture(t, f, root)

	if err := os.RemoveAll(first); err != nil {
		t.Fatalf("remove target: %v", err)
	}

	entries, err := List(context.Background(), f, root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (missing target kept)", len(entries))
	}

	if got := entryByDir(entries, first).State; got != StateDiverged {
		t.Fatalf("missing target = %q, want diverged", got)
	}
}

func TestListDivergedSkipsResolve(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	root := t.TempDir()
	listFixture(t, f, root)

	// Diverge every checkout, then reset the counter: list must not
	// resolve anything when all states derive locally.
	for _, dir := range []string{filepath.Join(root, "one"), filepath.Join(root, "two")} {
		writeFile(t, dir, "a.txt", "edited")
	}

	f.resolves = 0

	entries, err := List(context.Background(), f, root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if f.resolves != 0 {
		t.Fatalf("resolves = %d, want 0", f.resolves)
	}

	for _, e := range entries {
		if e.State != StateDiverged {
			t.Fatalf("%s = %q, want diverged", e.Dir, e.State)
		}
	}
}

func TestListUnreachable(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	root := t.TempDir()
	listFixture(t, f, root)
	f.resolveErr = os.ErrClosed

	entries, err := List(context.Background(), f, root)
	if err != nil {
		t.Fatalf("List must not fail on unreachable remotes: %v", err)
	}

	for _, e := range entries {
		if e.State != StateUnreachable {
			t.Fatalf("%s = %q, want unreachable", e.Dir, e.State)
		}
	}
}

func TestListInvalidRegistryFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	if err := os.WriteFile(RegistryPath(root), []byte("{oops"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := List(context.Background(), successFetcher(), root); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("err = %v, want fail-closed ErrInvalidRecord", err)
	}
}
