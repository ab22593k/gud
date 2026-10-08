package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
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

// TestListMixedStatesAcrossEntries pins the parallel hash fan-out to the
// sequential decision order: four checkouts in four different states must
// each derive exactly, with results slotted per entry rather than per
// completion order.
func TestListMixedStatesAcrossEntries(t *testing.T) {
	t.Parallel()

	f := &fakeFetcher{
		commits: map[string]string{"19.0": testCommitA},
		files: map[string]map[string]string{
			testCommitA: {"a.txt": "alpha"},
			testCommitB: {"a.txt": "alpha2"},
		},
	}

	root := t.TempDir()
	registry := RegistryPath(root)

	fetchAt := func(name string) string {
		t.Helper()

		target := filepath.Join(root, name)

		if _, err := Fetch(context.Background(), FetchOptions{Fetcher: f, RegistryPath: registry},
			testSource(), target); err != nil {
			t.Fatalf("fixture fetch %s: %v", name, err)
		}

		return target
	}

	behind := fetchAt("behind")
	diverged := fetchAt("diverged")

	// Upstream moves: behind is now stale, current fetches at the new tip.
	f.commits["19.0"] = testCommitB
	current := fetchAt("current")

	writeFile(t, diverged, "a.txt", "local edits")

	// Unreachable: a clean checkout whose ref no fetcher resolves. The target
	// must exist and match its export hash, or a missing directory would
	// short-circuit to diverged before resolution runs.
	ghost := filepath.Join(root, "ghost")
	writeFile(t, ghost, "a.txt", "alpha2")

	live, err := HashDir(ghost)
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	reg, err := LoadRegistry(registry)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	ghostEntry := EntryFor(SourceRef{
		Host:      "github.com",
		Owner:     "OCA",
		Repo:      "server-tools",
		Ref:       "vanished-branch",
		Subpath:   "auto_backup",
		SourceURL: "https://github.com/OCA/server-tools/tree/vanished-branch/auto_backup",
	}, testCommitB, live, time.Now())

	key, err := KeyFor(registry, ghost)
	if err != nil {
		t.Fatalf("KeyFor: %v", err)
	}

	if err := reg.Upsert(key, ghostEntry); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := SaveRegistry(registry, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	entries, err := List(context.Background(), f, root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := map[string]SyncState{
		current:  StateCurrent,
		behind:   StateBehind,
		diverged: StateDiverged,
		ghost:    StateUnreachable,
	}

	if len(entries) != len(want) {
		t.Fatalf("entries = %d, want %d (%v)", len(entries), len(want), entries)
	}

	for dir, state := range want {
		if got := entryByDir(entries, dir).State; got != state {
			t.Errorf("%s = %q, want %q", dir, got, state)
		}
	}
}
