package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// initWireFixture creates a repo with a subfolder, a tag, and a slashed
// branch, returning its directory and file:// URL.
func initWireFixture(t *testing.T) (dir, url string) {
	t.Helper()

	dir = filepath.Join(t.TempDir(), "src")

	mustGit(t, "", "init", "-b", "main", dir)
	mustGit(t, dir, "config", "user.email", "wire@test")
	mustGit(t, dir, "config", "user.name", "wire")

	writeWireFile(t, dir, "auto_backup", "a.txt", "alpha")
	writeWireFile(t, dir, "other", "b.txt", "beta")

	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-m", "first")
	mustGit(t, dir, "tag", "v1")
	mustGit(t, dir, "branch", "feature/foo")

	return dir, "file://" + dir
}

// mustGit runs git and fails the test on error.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	out, err := runGitDir(context.Background(), dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}

	return out
}

// writeWireFile creates name with content under sub in dir.
func writeWireFile(t *testing.T, dir, sub, name, content string) {
	t.Helper()

	full := filepath.Join(dir, sub)

	if err := os.MkdirAll(full, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(full, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestCloneMirrorHasNoCheckout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	_, url := initWireFixture(t)
	mirror := filepath.Join(t.TempDir(), "mirror")

	if err := CloneMirror(context.Background(), url, mirror); err != nil {
		t.Fatalf("CloneMirror: %v", err)
	}

	entries, err := os.ReadDir(mirror)
	if err != nil {
		t.Fatalf("read mirror: %v", err)
	}

	if len(entries) != 1 || entries[0].Name() != ".git" {
		t.Fatalf("mirror holds a checkout: %v", entries)
	}
}

func TestLsRemoteListsRefs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	_, url := initWireFixture(t)

	refs, err := LsRemote(context.Background(), url)
	if err != nil {
		t.Fatalf("LsRemote: %v", err)
	}

	for _, want := range []string{"refs/heads/main", "refs/heads/feature/foo", "refs/tags/v1"} {
		if refs[want] == "" {
			t.Errorf("missing %s in %v", want, refs)
		}
	}
}

func TestLsRemoteUnreachable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	if _, err := LsRemote(context.Background(), "file:///nonexistent-wire-repo"); err == nil {
		t.Fatal("expected error for missing remote")
	}
}

func TestFetchMirrorPicksUpCommits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	ctx := context.Background()
	src, url := initWireFixture(t)
	mirror := filepath.Join(t.TempDir(), "mirror")

	if err := CloneMirror(ctx, url, mirror); err != nil {
		t.Fatalf("CloneMirror: %v", err)
	}

	writeWireFile(t, src, "auto_backup", "new.txt", "gamma")
	mustGit(t, src, "add", ".")
	mustGit(t, src, "commit", "-m", "second")

	if err := FetchMirror(ctx, mirror); err != nil {
		t.Fatalf("FetchMirror: %v", err)
	}

	head := mustGit(t, src, "rev-parse", "HEAD")
	if !CommitExists(ctx, mirror, strings.TrimSpace(head)) {
		t.Fatal("mirror lacks the new commit after fetch")
	}
}

func TestCommitExists(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	ctx := context.Background()
	src, _ := initWireFixture(t)
	head := strings.TrimSpace(mustGit(t, src, "rev-parse", "HEAD"))

	if !CommitExists(ctx, src, head) {
		t.Fatal("HEAD should exist")
	}

	if CommitExists(ctx, src, strings.Repeat("0", 40)) {
		t.Fatal("zero SHA should not exist")
	}
}

func TestTreeExists(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	ctx := context.Background()
	src, _ := initWireFixture(t)
	head := strings.TrimSpace(mustGit(t, src, "rev-parse", "HEAD"))

	if !TreeExists(ctx, src, head, "auto_backup") {
		t.Fatal("auto_backup should exist")
	}

	if TreeExists(ctx, src, head, "nope") {
		t.Fatal("missing path should not exist")
	}

	if TreeExists(ctx, src, strings.Repeat("0", 40), "auto_backup") {
		t.Fatal("missing commit should not exist")
	}
}

func TestAddSparseWorktreePopulatesSubset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	ctx := context.Background()
	src, url := initWireFixture(t)
	mirror := filepath.Join(t.TempDir(), "mirror")

	if err := CloneMirror(ctx, url, mirror); err != nil {
		t.Fatalf("CloneMirror: %v", err)
	}

	head := strings.TrimSpace(mustGit(t, src, "rev-parse", "HEAD"))
	wt := filepath.Join(t.TempDir(), "wt")

	if err := AddSparseWorktree(ctx, mirror, wt, head, "auto_backup"); err != nil {
		t.Fatalf("AddSparseWorktree: %v", err)
	}

	if _, err := os.Stat(filepath.Join(wt, "auto_backup", "a.txt")); err != nil {
		t.Fatalf("subset file missing: %v", err)
	}

	if _, err := os.Stat(filepath.Join(wt, "other")); !os.IsNotExist(err) {
		t.Fatal("unrequested directory materialized")
	}
}

func TestAddSparseWorktreeMissingPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	ctx := context.Background()
	_, url := initWireFixture(t)
	mirror := filepath.Join(t.TempDir(), "mirror")

	if err := CloneMirror(ctx, url, mirror); err != nil {
		t.Fatalf("CloneMirror: %v", err)
	}

	head := strings.TrimSpace(mustGit(t, mirror, "rev-parse", "HEAD"))

	if err := AddSparseWorktree(ctx, mirror, filepath.Join(t.TempDir(), "wt"), head, "nope"); err == nil {
		t.Fatal("expected error for missing subpath")
	}
}

func TestRemoveAndPruneWorktrees(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	ctx := context.Background()
	_, url := initWireFixture(t)
	mirror := filepath.Join(t.TempDir(), "mirror")

	if err := CloneMirror(ctx, url, mirror); err != nil {
		t.Fatalf("CloneMirror: %v", err)
	}

	head := strings.TrimSpace(mustGit(t, mirror, "rev-parse", "HEAD"))
	wt := filepath.Join(t.TempDir(), "wt")

	if err := AddSparseWorktree(ctx, mirror, wt, head, "auto_backup"); err != nil {
		t.Fatalf("AddSparseWorktree: %v", err)
	}

	if err := RemoveWorktree(ctx, mirror, wt); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}

	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatal("worktree dir remains after remove")
	}

	// Simulate a killed run: drop the directory, prune the metadata.
	wt2 := filepath.Join(t.TempDir(), "wt2")

	if err := AddSparseWorktree(ctx, mirror, wt2, head, "auto_backup"); err != nil {
		t.Fatalf("AddSparseWorktree: %v", err)
	}

	if err := os.RemoveAll(wt2); err != nil {
		t.Fatalf("simulate kill: %v", err)
	}

	if err := PruneWorktrees(ctx, mirror); err != nil {
		t.Fatalf("PruneWorktrees: %v", err)
	}

	out, err := runMirror(ctx, mirror, "worktree", "list", "--porcelain")
	if err != nil {
		t.Fatalf("worktree list: %v", err)
	}

	if strings.Contains(out, wt2) {
		t.Fatalf("stale worktree remains: %q", out)
	}
}
