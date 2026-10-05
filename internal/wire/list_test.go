package wire

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// listFixture fetches two checkouts of the fake source under root.
func listFixture(t *testing.T, f *fakeFetcher, root string) (first, second string) {
	t.Helper()

	first = filepath.Join(root, "one")
	second = filepath.Join(root, "two")

	for _, target := range []string{first, second} {
		if _, err := Fetch(context.Background(), FetchOptions{Fetcher: f}, testSource(), target); err != nil {
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

		if e.RemoteSHA != testCommitA || e.Record.ResolvedCommit != testCommitA {
			t.Fatalf("%s SHAs = %q/%q", dir, e.RemoteSHA, e.Record.ResolvedCommit)
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

func TestListSkipsInvalidRecords(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	root := t.TempDir()
	listFixture(t, f, root)

	bad := filepath.Join(root, "bad")
	if err := os.MkdirAll(bad, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(RecordPath(bad), []byte("{oops"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries, err := List(context.Background(), f, root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (bad record skipped)", len(entries))
	}
}

func TestListDepthCap(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	root := t.TempDir()

	deep := root
	for range maxListDepth + 2 {
		deep = filepath.Join(deep, "d")
	}

	if _, err := Fetch(context.Background(), FetchOptions{Fetcher: f}, testSource(), deep); err != nil {
		t.Fatalf("fixture fetch: %v", err)
	}

	entries, err := List(context.Background(), f, root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf("deep checkout listed despite cap: %v", entries)
	}
}

func TestListMissingRoot(t *testing.T) {
	t.Parallel()

	if _, err := List(context.Background(), successFetcher(), filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected error for missing root")
	}
}
