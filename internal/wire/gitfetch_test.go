package wire

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestShortRefs(t *testing.T) {
	t.Parallel()

	refs := map[string]string{
		"HEAD":                 "h0",
		"refs/heads/main":      "c1",
		"refs/heads/feature/x": "c2",
		"refs/tags/v1":         "t1",
		"refs/tags/v1^{}":      "c3",
		"refs/notes/n":         "n1",
	}

	known, commits := shortRefs(refs)

	slices.Sort(known)

	if !slices.Equal(known, []string{"feature/x", "main", "v1"}) {
		t.Fatalf("known = %v", known)
	}

	if commits["v1"] != "c3" {
		t.Fatalf("peeled tag SHA = %q, want commit c3", commits["v1"])
	}

	if commits["main"] != "c1" || commits["feature/x"] != "c2" {
		t.Fatalf("commits = %v", commits)
	}
}

func TestGitFetcherMaterializeFromFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	ctx := context.Background()

	// Build a local "remote" and pre-seed the mirror location by hand, so
	// no network is needed: ensureMirror finds the mirror and only fetches
	// its (local) origin.
	remote := t.TempDir() + "/remote"
	mustInitFixture(t, remote)

	store := NewStoreWithDir(t.TempDir())
	mirror := store.MirrorDir("example.com", "o", "r")

	if err := localClone(t, remote, mirror); err != nil {
		t.Fatalf("seed mirror: %v", err)
	}

	head := localHead(t, remote)
	fetcher := NewFetcher(store)

	source := SourceRef{Host: "example.com", Owner: "o", Repo: "r", Ref: "main", Subpath: "auto_backup"}
	target := t.TempDir() + "/out"

	n, err := fetcher.Materialize(ctx, source, Resolution{Commit: head, Ref: "main", Subpath: "auto_backup"}, target)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}

	if n != 1 {
		t.Fatalf("files = %d, want 1", n)
	}

	data, err := os.ReadFile(filepath.Join(target, "a.txt"))
	if err != nil || string(data) != "alpha" {
		t.Fatalf("content = %q, err = %v", data, err)
	}

	entries, err := os.ReadDir(filepath.Join(store.Root(), "worktrees"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read worktrees: %v", err)
	}

	for _, e := range entries {
		t.Fatalf("worktree residue: %q", e.Name())
	}
}

// mustInitFixture creates a committed repo with auto_backup/a.txt and
// other/b.txt. Local git only: deterministic, no network.
func mustInitFixture(t *testing.T, dir string) {
	t.Helper()

	sub := filepath.Join(dir, "auto_backup")

	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(sub, "a.txt"), []byte("alpha"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	gitOut(t, "", "init", "-b", "main", dir)
	gitOut(t, dir, "config", "user.email", "wire@test")
	gitOut(t, dir, "config", "user.name", "wire")
	gitOut(t, dir, "add", ".")
	gitOut(t, dir, "commit", "-m", "first")
}

// gitOut runs git and fails the test on error.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", args...)

	if dir != "" {
		cmd.Dir = dir
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}

	return strings.TrimSpace(string(out))
}

// localClone seeds a mirror without network (partial-clone filters do not
// apply to local transports, which is fine: blob filtering is a server
// interaction, asserted live, not here).
func localClone(t *testing.T, remote, mirror string) error {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", "clone", "--no-checkout", remote, mirror)

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("clone: %w\n%s", err, out)
	}

	return nil
}

// localHead resolves the fixture HEAD.
func localHead(t *testing.T, remote string) string {
	t.Helper()

	return gitOut(t, remote, "rev-parse", "HEAD")
}
