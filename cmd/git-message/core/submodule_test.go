package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildSubmoduleContext verifies the prompt-context wiring: a staged
// gitlink update yields a submodule fragment (name, URL, range, subjects),
// while diffs without gitlink changes produce no fragment.
func TestBuildSubmoduleContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	runIn := func(workDir string, args ...string) string {
		t.Helper()
		//nolint:gosec // test-only git invocation with fixed repo-local args
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = workDir

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}

		return strings.TrimSpace(string(out))
	}

	// Inner repo: the submodule.
	inner := filepath.Join(dir, "sub")
	if err := os.MkdirAll(inner, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	runIn(inner, "init", "-q", "-b", "main")
	runIn(inner, "config", "user.name", "Test")
	runIn(inner, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(inner, "f.txt"), []byte("v1\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	runIn(inner, "add", ".")
	runIn(inner, "commit", "-q", "-m", "feat: v1")

	sha1 := runIn(inner, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(inner, "f.txt"), []byte("v2\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	runIn(inner, "commit", "-q", "-am", "fix: v2")
	sha2 := runIn(inner, "rev-parse", "HEAD")

	// Outer repo with a hand-written .gitmodules (the file-transport
	// restriction blocks git submodule add with local paths).
	runIn(dir, "init", "-q", "-b", "main")
	runIn(dir, "config", "user.name", "Test")
	runIn(dir, "config", "user.email", "test@example.com")

	gitmodules := "[submodule \"sub\"]\n\tpath = sub\n\turl = https://example.com/org/sub.git\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitmodules"), []byte(gitmodules), 0600); err != nil {
		t.Fatalf("write .gitmodules: %v", err)
	}

	runIn(dir, "update-index", "--add", "--cacheinfo", "160000,"+sha1+",sub")
	runIn(dir, "commit", "-q", "-m", "chore: add submodule sub")
	runIn(dir, "update-index", "--cacheinfo", "160000,"+sha2+",sub")
	gitlinkDiff := runIn(dir, "diff", "--cached")

	t.Chdir(dir)

	app := &AppContext{}

	got := buildSubmoduleContext(context.Background(), app, gitlinkDiff, historyPresent)
	for _, want := range []string{"Submodule (gitlink) changes:", "sub", "https://example.com/org/sub.git", "fix: v2"} {
		if !strings.Contains(got, want) {
			t.Errorf("buildSubmoduleContext() missing %q in:\n%s", want, got)
		}
	}

	// A plain file diff must produce no submodule fragment.
	plainDiff := "diff --git a/f.txt b/f.txt\n" +
		"index 1111111..2222222 100644\n--- a/f.txt\n+++ b/f.txt\n" +
		"@@ -1 +1 @@\n-x\n+y\n"
	if got := buildSubmoduleContext(context.Background(), app, plainDiff, historyPresent); got != "" {
		t.Errorf("buildSubmoduleContext() should be empty for non-gitlink diffs, got: %q", got)
	}
}

// TestBuildSubmoduleContext_RepositoryState pins FR-008 in the unborn
// state (D8): the fragment still names the sub-project, path, URL, and
// the sha1..sha2 pointer range, but carries no commit subjects; in the
// present state the subject line is included.
func TestBuildSubmoduleContext_RepositoryState(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	runIn := func(workDir string, args ...string) string {
		t.Helper()
		//nolint:gosec // test-only git invocation with fixed repo-local args
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = workDir

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}

		return strings.TrimSpace(string(out))
	}

	// Inner repo: the submodule, with two commits so the update range
	// sha1..sha2 resolves to exactly one subject.
	inner := filepath.Join(dir, "sub")
	if err := os.MkdirAll(inner, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	runIn(inner, "init", "-q", "-b", "main")
	runIn(inner, "config", "user.name", "Test")
	runIn(inner, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(inner, "f.txt"), []byte("v1\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	runIn(inner, "add", ".")
	runIn(inner, "commit", "-q", "-m", "feat: v1")

	sha1 := runIn(inner, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(inner, "f.txt"), []byte("v2\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	runIn(inner, "commit", "-q", "-am", "fix: v2")
	sha2 := runIn(inner, "rev-parse", "HEAD")

	// Outer repo: unborn, with a hand-written .gitmodules. The gitlink
	// update diff is synthesized rather than taken from git: staging the
	// pointer in this repo and then updating it would read back as an
	// *added* gitlink (the parent has no commit to anchor an update diff
	// against), while the child range sha1..sha2 needs exactly the update
	// shape for the subject suppression to be meaningful. The entry shape
	// matches the real update-index diffs used elsewhere in this file.
	runIn(dir, "init", "-q", "-b", "main")
	runIn(dir, "config", "user.name", "Test")
	runIn(dir, "config", "user.email", "test@example.com")

	gitmodules := "[submodule \"sub\"]\n\tpath = sub\n\turl = https://example.com/org/sub.git\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitmodules"), []byte(gitmodules), 0600); err != nil {
		t.Fatalf("write .gitmodules: %v", err)
	}

	gitlinkDiff := "diff --git a/sub b/sub\n" +
		"index " + sha1[:7] + ".." + sha2[:7] + " 160000\n" +
		"--- a/sub\n+++ b/sub\n" +
		"@@ -1 +1 @@\n" +
		"-Subproject commit " + sha1 + "\n" +
		"+Subproject commit " + sha2 + "\n"

	t.Chdir(dir)

	app := &AppContext{}
	ctx := context.Background()

	absent := buildSubmoduleContext(ctx, app, gitlinkDiff, historyAbsent)
	for _, want := range []string{"sub", "https://example.com/org/sub.git", sha1[:7] + ".." + sha2[:7]} {
		if !strings.Contains(absent, want) {
			t.Errorf("historyAbsent fragment missing %q in:\n%s", want, absent)
		}
	}

	if strings.Contains(absent, "> fix: v2") {
		t.Errorf("historyAbsent fragment must not carry commit subjects, got:\n%s", absent)
	}

	present := buildSubmoduleContext(ctx, app, gitlinkDiff, historyPresent)
	if !strings.Contains(present, "fix: v2") {
		t.Errorf("historyPresent fragment should carry the commit subject, got:\n%s", present)
	}
}
