package wire

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// mustFetcher builds the real Fetcher for tests. NewFetcher only fails on a
// nil store (programming error), so failure here is a test bug.
func mustFetcher(t *testing.T, store *Store) Fetcher {
	t.Helper()

	fetcher, err := NewFetcher(store)
	if err != nil {
		t.Fatalf("NewFetcher: %v", err)
	}

	return fetcher
}

// TestNewFetcherNilStoreIsErrInternal pins the no-panic contract: a nil
// store is a programming error reported as an ErrInternal error, never a
// panic that would crash the CLI with a Go trace.
func TestNewFetcherNilStoreIsErrInternal(t *testing.T) {
	t.Parallel()

	fetcher, err := NewFetcher(nil)
	if fetcher != nil {
		t.Errorf("NewFetcher(nil) = %v, want nil fetcher", fetcher)
	}

	if !errors.Is(err, ErrInternal) {
		t.Fatalf("NewFetcher(nil) err = %v, want ErrInternal", err)
	}
}

// seedWireMirror builds a local remote with auto_backup/a.txt, seeds the
// store mirror from it without network, and returns the store and HEAD.
func seedWireMirror(t *testing.T) (*Store, string) {
	t.Helper()

	remote := t.TempDir() + "/remote"
	mustInitFixture(t, remote)

	store, err := NewStoreWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	mirror := store.MirrorDir("example.com", "o", "r")

	if err := localClone(t, remote, mirror); err != nil {
		t.Fatalf("seed mirror: %v", err)
	}

	return store, localHead(t, remote)
}

// seedSlashedRemote builds a local remote with branches main and
// release/v1.0, where release/v1.0 holds pkg/util/tool.go. Local git only:
// deterministic, no network.
func seedSlashedRemote(t *testing.T) (*Store, map[string]string) {
	t.Helper()

	remote := t.TempDir() + "/remote"
	mustInitFixture(t, remote)

	// Create the slashed branch with its own subfolder, then return to main
	// so the default HEAD is unaffected.
	gitOut(t, remote, "checkout", "-b", "release/v1.0")

	util := filepath.Join(remote, "pkg", "util")

	if err := os.MkdirAll(util, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(util, "tool.go"), []byte("package util"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	gitOut(t, remote, "add", ".")
	gitOut(t, remote, "commit", "-m", "slashed branch")

	slashed := gitOut(t, remote, "rev-parse", "HEAD")

	gitOut(t, remote, "checkout", "main")

	store, err := NewStoreWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	if err := localClone(t, remote, store.MirrorDir("example.com", "o", "r")); err != nil {
		t.Fatalf("seed mirror: %v", err)
	}

	return store, map[string]string{"main": localHead(t, remote), "release/v1.0": slashed}
}

// TestGitFetcherResolveSlashedRef proves FR-003a: a greedy URL split of
// .../tree/release/v1.0/pkg/util resolves against the remote's live ref
// listing, choosing the longest known ref (release/v1.0) and leaving the
// remainder as the subpath.
func TestGitFetcherResolveSlashedRef(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	ctx := context.Background()
	store, commits := seedSlashedRemote(t)
	fetcher := mustFetcher(t, store)

	source := func(ref, subpath string) SourceRef {
		return SourceRef{Host: "example.com", Owner: "o", Repo: "r", Ref: ref, Subpath: subpath}
	}

	for _, tc := range []struct {
		name    string
		source  SourceRef
		wantRef string
		wantSub string
	}{
		{name: "slashed branch", source: source("release", "v1.0/pkg/util"), wantRef: "release/v1.0", wantSub: "pkg/util"},
		{name: "plain branch", source: source("main", "auto_backup"), wantRef: "main", wantSub: "auto_backup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := fetcher.Resolve(ctx, tc.source)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}

			if res.Ref != tc.wantRef || res.Subpath != tc.wantSub {
				t.Fatalf("got (%q,%q), want (%q,%q)", res.Ref, res.Subpath, tc.wantRef, tc.wantSub)
			}

			if res.Commit != commits[tc.wantRef] {
				t.Fatalf("commit = %q, want %q", res.Commit, commits[tc.wantRef])
			}
		})
	}

	if _, err := fetcher.Resolve(ctx, source("nope", "v1.0/pkg/util")); !errors.Is(err, ErrUnknownRef) {
		t.Fatalf("err = %v, want ErrUnknownRef", err)
	}

	// The longest match leaves no path: the URL named the repository root,
	// so FR-003b's bad-URL rule applies (not a missing subfolder).
	if _, err := fetcher.Resolve(ctx, source("release", "v1.0")); !errors.Is(err, ErrBadURL) {
		t.Fatalf("root remainder err = %v, want ErrBadURL", err)
	} else if !strings.Contains(err.Error(), WholeRepositoryGuidance) {
		t.Fatalf("root remainder err = %q, want whole-repository guidance", err)
	}
}

// TestGitFetcherResolvePrefersRefreshedRefs proves a cache's clone-time refs
// cannot shadow what the latest fetch brought in. A normal clone never
// advances refs/heads/* on fetch, so the remote-tracking copy must win for
// both named and default (HEAD) references; otherwise the same URL resolves
// to different commits on different runs.
func TestGitFetcherResolvePrefersRefreshedRefs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	ctx := context.Background()

	remote := t.TempDir() + "/remote"
	mustInitFixture(t, remote)

	store, err := NewStoreWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	mirror := store.MirrorDir("example.com", "o", "r")
	if err := localClone(t, remote, mirror); err != nil {
		t.Fatalf("seed mirror: %v", err)
	}

	stale := localHead(t, remote)

	if err := os.WriteFile(filepath.Join(remote, "auto_backup", "b.txt"), []byte("beta"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	gitOut(t, remote, "add", ".")
	gitOut(t, remote, "commit", "-m", "advance")

	fresh := localHead(t, remote)
	if stale == fresh {
		t.Fatal("fixture did not advance")
	}

	for _, ref := range []string{"main", headRef} {
		for range 10 {
			fetcher := mustFetcher(t, store)
			source := SourceRef{Host: "example.com", Owner: "o", Repo: "r", Ref: ref, Subpath: "auto_backup"}

			res, err := fetcher.Resolve(ctx, source)
			if err != nil {
				t.Fatalf("Resolve(%s): %v", ref, err)
			}

			if res.Commit != fresh {
				t.Fatalf("Resolve(%s) = %s, want refreshed %s (stale %s)",
					ref, ShortSHA(res.Commit), ShortSHA(fresh), ShortSHA(stale))
			}
		}
	}
}

// TestShortRefsPrefersRemoteTrackingBranch pins the resolution rule behind the
// test above at the pure-function level, without any git fixture.
func TestShortRefsPrefersRemoteTrackingBranch(t *testing.T) {
	t.Parallel()

	refs := map[string]string{
		"refs/heads/19.0":          "stale",
		"refs/remotes/origin/19.0": "fresh",
	}

	for range 50 {
		_, commits := shortRefs(refs)
		if commits["19.0"] != "fresh" {
			t.Fatalf("commits[19.0] = %q, want fresh", commits["19.0"])
		}
	}
}

// TestShortRefsNameCollisionIsStable pins determinism when a branch and a tag
// share a name: the tag wins, independent of map iteration order.
func TestShortRefsNameCollisionIsStable(t *testing.T) {
	t.Parallel()

	refs := map[string]string{
		"refs/heads/v1": "branch",
		"refs/tags/v1":  "tag",
	}

	for range 50 {
		_, commits := shortRefs(refs)
		if commits["v1"] != "tag" {
			t.Fatalf("commits[v1] = %q, want tag", commits["v1"])
		}
	}
}

// TestDefaultBranchSHAPrefersRemoteTrackingHead pins the default-branch rule:
// the clone-time local HEAD goes stale, the remote-tracking HEAD does not.
func TestDefaultBranchSHAPrefersRemoteTrackingHead(t *testing.T) {
	t.Parallel()

	refs := map[string]string{
		"HEAD":                     "stale",
		"refs/remotes/origin/HEAD": "fresh",
	}

	if got := defaultBranchSHA(refs); got != "fresh" {
		t.Fatalf("defaultBranchSHA = %q, want fresh", got)
	}

	if got := defaultBranchSHA(map[string]string{"HEAD": "only"}); got != "only" {
		t.Fatalf("defaultBranchSHA fallback = %q, want only", got)
	}
}

func TestGitFetcherResolveFileSubpath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	ctx := context.Background()
	store, head := seedWireMirror(t)
	fetcher := mustFetcher(t, store)

	fileSource := func(ref string) SourceRef {
		return SourceRef{Host: "example.com", Owner: "o", Repo: "r", Ref: ref, Subpath: "auto_backup/a.txt"}
	}

	for _, tc := range []struct {
		name   string
		source SourceRef
	}{
		{name: "named ref", source: fileSource("main")},
		{name: "pinned commit", source: fileSource(head)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fetcher.Resolve(ctx, tc.source)
			if !errors.Is(err, ErrMissingPath) {
				t.Fatalf("err = %v, want ErrMissingPath", err)
			}

			if !strings.Contains(err.Error(), "example.com/o/r") {
				t.Fatalf("err = %v, want parsed identity echoed", err)
			}
		})
	}
}

func TestGitFetcherMaterializeFileSubpath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	ctx := context.Background()
	store, head := seedWireMirror(t)
	fetcher := mustFetcher(t, store)

	source := SourceRef{Host: "example.com", Owner: "o", Repo: "r", Ref: head, Subpath: "auto_backup/a.txt"}
	target := t.TempDir() + "/out"

	_, err := fetcher.Materialize(ctx, source,
		Resolution{Commit: head, Ref: head, Subpath: "auto_backup/a.txt"}, target)
	if !errors.Is(err, ErrMissingPath) {
		t.Fatalf("err = %v, want ErrMissingPath", err)
	}
}

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

	store, err := NewStoreWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	mirror := store.MirrorDir("example.com", "o", "r")

	if err := localClone(t, remote, mirror); err != nil {
		t.Fatalf("seed mirror: %v", err)
	}

	head := localHead(t, remote)
	fetcher := mustFetcher(t, store)

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
