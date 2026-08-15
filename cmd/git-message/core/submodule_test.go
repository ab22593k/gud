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
		cmd := exec.Command("git", args...)
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

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	app := &AppContext{}
	got := buildSubmoduleContext(context.Background(), app, gitlinkDiff)
	for _, want := range []string{"Submodule (gitlink) changes:", "sub", "https://example.com/org/sub.git", "fix: v2"} {
		if !strings.Contains(got, want) {
			t.Errorf("buildSubmoduleContext() missing %q in:\n%s", want, got)
		}
	}

	// A plain file diff must produce no submodule fragment.
	plainDiff := "diff --git a/f.txt b/f.txt\n" +
		"index 1111111..2222222 100644\n--- a/f.txt\n+++ b/f.txt\n" +
		"@@ -1 +1 @@\n-x\n+y\n"
	if got := buildSubmoduleContext(context.Background(), app, plainDiff); got != "" {
		t.Errorf("buildSubmoduleContext() should be empty for non-gitlink diffs, got: %q", got)
	}
}
