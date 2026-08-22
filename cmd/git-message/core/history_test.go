package core

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"gud/internal/config"
)

// newCoreHistoryTestRepo builds a repo with a diverged topic branch, chdirs
// into it, and returns the dir:
//
//	main:      "feat: base" → "feat: main work"
//	origin/main → "feat: base" (topic upstream)
//	topic:     "feat: base" → "feat: change file1" → "feat: change file2"
func newCoreHistoryTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		//nolint:gosec // test-only git invocation with fixed repo-local args
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(dir+"/"+name, []byte(content), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")
	write("file1.txt", "one\n")
	write("file2.txt", "two\n")
	run("add", ".")
	run("commit", "-q", "-m", "feat: base")
	// Publish an upstream ref and configure the topic to track it. The remote
	// URL is a throwaway path: upstream resolution only needs the remote to be
	// registered, not reachable.
	run("remote", "add", "origin", t.TempDir())
	run("update-ref", "refs/remotes/origin/main", "HEAD")
	run("config", "branch.topic.remote", "origin")
	run("config", "branch.topic.merge", "refs/heads/main")

	run("checkout", "-q", "-b", "topic")
	write("file1.txt", "one changed\n")
	run("add", ".")
	run("commit", "-q", "-m", "feat: change file1")
	write("file2.txt", "two changed\n")
	run("add", ".")
	run("commit", "-q", "-m", "feat: change file2")
	run("checkout", "-q", "main")
	write("file3.txt", "three\n")
	run("add", ".")
	run("commit", "-q", "-m", "feat: main work")
	run("checkout", "-q", "topic")

	t.Chdir(dir)

	return dir
}

func TestBuildHistoryContext_GraphRelative(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	newCoreHistoryTestRepo(t)
	app := &AppContext{cfg: config.Config{History: config.Ptr(5)}}

	got := buildHistoryContext(context.Background(), app, "")
	if !strings.Contains(got, "Commits on topic since diverging from origin/main:") {
		t.Errorf("history should be labelled by branch and upstream, got: %q", got)
	}
	if !strings.Contains(got, "feat: change file2") || !strings.Contains(got, "feat: change file1") {
		t.Errorf("history should contain the topic commits since divergence, got: %q", got)
	}
	if strings.Contains(got, "feat: main work") {
		t.Errorf("history should exclude commits outside the topic (merge-base .. HEAD), got: %q", got)
	}
	if strings.Contains(got, "Recent commits:") {
		t.Errorf("graph-relative history should not use the recent-commits label, got: %q", got)
	}
}

func TestBuildHistoryContext_GraphRelative_StagedPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	newCoreHistoryTestRepo(t)
	app := &AppContext{cfg: config.Config{History: config.Ptr(5)}}

	// A staged diff touching only file1.txt must limit the topic history.
	diff := "diff --git a/file1.txt b/file1.txt\n--- a/file1.txt\n+++ b/file1.txt\n"
	got := buildHistoryContext(context.Background(), app, diff)
	if !strings.Contains(got, "(staged files)") {
		t.Errorf("history limited to staged files should say so, got: %q", got)
	}
	if !strings.Contains(got, "feat: change file1") {
		t.Errorf("history should include commits touching the staged file, got: %q", got)
	}
	if strings.Contains(got, "feat: change file2") {
		t.Errorf("history should exclude commits not touching the staged file, got: %q", got)
	}
}

func TestBuildHistoryContext_FallsBackToRecent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		//nolint:gosec // test-only git invocation with fixed repo-local args
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")
	if err := os.WriteFile(dir+"/f.txt", []byte("x\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "feat: first")
	if err := os.WriteFile(dir+"/f.txt", []byte("x\nmore\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "feat: second")

	t.Chdir(dir)

	app := &AppContext{cfg: config.Config{History: config.Ptr(5)}}
	got := buildHistoryContext(context.Background(), app, "")
	if !strings.Contains(got, "Recent commits:") {
		t.Errorf("without an upstream the history should fall back to recent commits, got: %q", got)
	}
	if !strings.Contains(got, "feat: second") {
		t.Errorf("recent-commits fallback should contain the latest commit, got: %q", got)
	}
}
