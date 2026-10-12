package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractSubmoduleChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		diff string
		want []SubmoduleChange
	}{
		{
			name: "update gitlink",
			diff: "diff --git a/sub b/sub\n" +
				"index 63058a7..84a348a 160000\n" +
				"--- a/sub\n+++ b/sub\n" +
				"@@ -1 +1 @@\n" +
				"-Subproject commit 63058a7ee2dbeaa7d173fc0ce7d16696d2ca9a3d\n" +
				"+Subproject commit 84a348a87712d8fbddb88172c5dd34174d04f312\n",
			want: []SubmoduleChange{{
				Path:      "sub",
				OldCommit: "63058a7ee2dbeaa7d173fc0ce7d16696d2ca9a3d",
				NewCommit: "84a348a87712d8fbddb88172c5dd34174d04f312",
			}},
		},
		{
			name: "add gitlink",
			diff: "diff --git a/sub2 b/sub2\n" +
				"new file mode 160000\n" +
				"index 0000000..5ef786e\n" +
				"--- /dev/null\n+++ b/sub2\n",
			want: []SubmoduleChange{{Path: "sub2", OldCommit: "0000000", NewCommit: "5ef786e"}},
		},
		{
			name: "remove gitlink",
			diff: "diff --git a/sub2 b/sub2\n" +
				"deleted file mode 160000\n" +
				"index 5ef786e..0000000\n" +
				"--- a/sub2\n+++ /dev/null\n",
			want: []SubmoduleChange{{Path: "sub2", OldCommit: "5ef786e", NewCommit: "0000000"}},
		},
		{
			name: "regular file change is ignored",
			diff: "diff --git a/main.go b/main.go\n" +
				"index 1111111..2222222 100644\n" +
				"--- a/main.go\n+++ b/main.go\n" +
				"@@ -1 +1 @@\n-func a()\n+func b()\n",
			want: nil,
		},
		{
			name: "mixed regular and gitlink changes",
			diff: "diff --git a/main.go b/main.go\n" +
				"index 1111111..2222222 100644\n--- a/main.go\n+++ b/main.go\n" +
				"@@ -1 +1 @@\n-func a()\n+func b()\n" +
				"diff --git a/sub b/sub\n" +
				"index 63058a7..84a348a 160000\n--- a/sub\n+++ b/sub\n" +
				"@@ -1 +1 @@\n" +
				"-Subproject commit 63058a7ee2dbeaa7d173fc0ce7d16696d2ca9a3d\n" +
				"+Subproject commit 84a348a87712d8fbddb88172c5dd34174d04f312\n",
			want: []SubmoduleChange{{
				Path:      "sub",
				OldCommit: "63058a7ee2dbeaa7d173fc0ce7d16696d2ca9a3d",
				NewCommit: "84a348a87712d8fbddb88172c5dd34174d04f312",
			}},
		},
		{
			name: "empty diff",
			diff: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ExtractSubmoduleChanges(tt.diff)
			if len(got) != len(tt.want) {
				t.Fatalf("ExtractSubmoduleChanges() = %v, want %v", got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ExtractSubmoduleChanges()[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestSubmoduleContext_PathEscapeSkipsSubjects is the regression lock for a
// crafted diff path: ch.Path derives from +++ b/<path>, which a malicious
// filename can shape with .. segments. git -C must never operate outside the
// repo root, so escaping paths keep the SHA-only summary with no subjects
// from outside the repository.
func TestSubmoduleContext_PathEscapeSkipsSubjects(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	tmp := t.TempDir()
	root := filepath.Join(tmp, "root")
	sib := filepath.Join(tmp, "sibling")

	runIn := func(workDir string, args ...string) string {
		t.Helper()

		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = workDir

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}

		return strings.TrimSpace(string(out))
	}

	for _, d := range []string{root, sib} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		runIn(d, "init", "-q", "-b", "main")
		runIn(d, "config", "user.name", "Test")
		runIn(d, "config", "user.email", "test@example.com")
	}

	// Sibling repo holds a commit reachable only outside root.
	if err := os.WriteFile(filepath.Join(sib, "secret.txt"), []byte("s3cret\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	runIn(sib, "add", ".")
	runIn(sib, "commit", "-q", "-m", "SECRET-OUTSIDE-ROOT")
	sha := runIn(sib, "rev-parse", "HEAD")

	const zero = "0000000000000000000000000000000000000000"

	for _, path := range []string{"../sibling", "sub/../../sibling", ".."} {
		ch := SubmoduleChange{Path: path, OldCommit: zero, NewCommit: sha}

		got := SubmoduleContext(context.Background(), root, []SubmoduleChange{ch})
		if strings.Contains(got, "SECRET-OUTSIDE-ROOT") {
			t.Errorf("path %q: subjects leaked from outside repo root:\n%s", path, got)
		}

		if !strings.Contains(got, shortSHA(sha)) {
			t.Errorf("path %q: SHA-only summary lost, got:\n%s", path, got)
		}
	}
}

func TestIsWithinDir(t *testing.T) {
	t.Parallel()

	root := filepath.FromSlash("/repo")

	cases := []struct {
		name string
		dir  string
		want bool
	}{
		{name: "root itself", dir: filepath.FromSlash("/repo"), want: true},
		{name: "child", dir: filepath.FromSlash("/repo/sub"), want: true},
		{name: "nested child", dir: filepath.FromSlash("/repo/a/b"), want: true},
		{name: "parent", dir: filepath.FromSlash("/"), want: false},
		{name: "dotdot", dir: filepath.FromSlash("/repo/.."), want: false},
		{name: "sibling via dotdot", dir: filepath.FromSlash("/repo/../sibling"), want: false},
		{name: "nested escape", dir: filepath.FromSlash("/repo/sub/../../sibling"), want: false},
		{name: "sibling", dir: filepath.FromSlash("/sibling"), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isWithinDir(root, tc.dir); got != tc.want {
				t.Errorf("isWithinDir(%q, %q) = %v, want %v", root, tc.dir, got, tc.want)
			}
		})
	}
}

// TestSubmoduleContext verifies end-to-end enrichment: a staged gitlink update
// yields the submodule name, URL, range, and locally available commit subjects;
// an uninitialized submodule degrades to a SHA-only summary.
func TestSubmoduleContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	runIn := func(workDir string, args ...string) string {
		t.Helper()

		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = workDir

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}

		return strings.TrimSpace(string(out))
	}

	// Inner repo: the submodule, with two commits to range over.
	inner := filepath.Join(dir, "sub")
	if err := os.MkdirAll(inner, 0755); err != nil {
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

	// Outer repo: register the submodule via a hand-written .gitmodules (the
	// file-transport restriction blocks git submodule add with local paths).
	runIn(dir, "init", "-q", "-b", "main")
	runIn(dir, "config", "user.name", "Test")
	runIn(dir, "config", "user.email", "test@example.com")

	gitmodules := "[submodule \"sub\"]\n\tpath = sub\n\turl = https://example.com/org/sub.git\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitmodules"), []byte(gitmodules), 0600); err != nil {
		t.Fatalf("write .gitmodules: %v", err)
	}

	runIn(dir, "update-index", "--add", "--cacheinfo", "160000,"+sha1+",sub")
	runIn(dir, "commit", "-q", "-m", "chore: add submodule sub")

	// Stage an update of the gitlink to sha2.
	runIn(dir, "update-index", "--cacheinfo", "160000,"+sha2+",sub")
	diff := runIn(dir, "diff", "--cached")

	changes := ExtractSubmoduleChanges(diff)
	if len(changes) != 1 {
		t.Fatalf("expected 1 gitlink change, got %v", changes)
	}

	got := SubmoduleContext(context.Background(), dir, changes)
	// The range old..new contains only the new commit's subject.
	for _, want := range []string{"sub", "https://example.com/org/sub.git", "fix: v2"} {
		if !strings.Contains(got, want) {
			t.Errorf("SubmoduleContext() missing %q in:\n%s", want, got)
		}
	}

	// Removing the checkout simulates an uninitialized submodule: subjects must
	// disappear but the SHA-only range must remain.
	if err := os.RemoveAll(inner); err != nil {
		t.Fatalf("remove checkout: %v", err)
	}

	got = SubmoduleContext(context.Background(), dir, changes)
	if !strings.Contains(got, sha1[:7]) {
		t.Errorf("SubmoduleContext() should keep the range summary, got:\n%s", got)
	}

	if strings.Contains(got, "fix: v2") {
		t.Errorf("SubmoduleContext() should drop subjects without local history, got:\n%s", got)
	}
}
