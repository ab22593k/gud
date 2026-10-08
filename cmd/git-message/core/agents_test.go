package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gud/internal/request"
)

func TestReadAgentsFile(t *testing.T) {
	t.Parallel()

	const content = "Use conventional commits.\nName the ticket in the subject."

	t.Run("returns content", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, agentsFileName), content)

		got, ok := readAgentsFile(dir)
		if !ok {
			t.Fatal("readAgentsFile() ok = false, want true")
		}

		if got != content {
			t.Errorf("readAgentsFile() = %q, want %q", got, content)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		if _, ok := readAgentsFile(t.TempDir()); ok {
			t.Error("readAgentsFile() ok = true, want false for a missing file")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, agentsFileName), "")

		if _, ok := readAgentsFile(dir); ok {
			t.Error("readAgentsFile() ok = true, want false for an empty file")
		}
	})

	t.Run("whitespace-only file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, agentsFileName), "\n\t  \n")

		if _, ok := readAgentsFile(dir); ok {
			t.Error("readAgentsFile() ok = true, want false for a whitespace-only file")
		}
	})

	t.Run("surrounding whitespace is trimmed", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, agentsFileName), "\n\n"+content+"\n\n")

		got, ok := readAgentsFile(dir)
		if !ok {
			t.Fatal("readAgentsFile() ok = false, want true")
		}

		if got != content {
			t.Errorf("readAgentsFile() = %q, want %q", got, content)
		}
	})

	t.Run("UTF-8 BOM is stripped", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, agentsFileName), "\ufeff"+content)

		got, ok := readAgentsFile(dir)
		if !ok {
			t.Fatal("readAgentsFile() ok = false, want true")
		}

		if got != content {
			t.Errorf("readAgentsFile() = %q, want %q", got, content)
		}
	})

	t.Run("directory named AGENTS.md", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, agentsFileName), 0750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if _, ok := readAgentsFile(dir); ok {
			t.Error("readAgentsFile() ok = true, want false for a directory")
		}
	})

	// A symlink is followed, so an AGENTS.md that lives elsewhere and is linked
	// in still works — but a broken one is treated as absent, not as a failure.
	t.Run("symlink to a file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "real.md"), content)

		if err := os.Symlink(filepath.Join(dir, "real.md"), filepath.Join(dir, agentsFileName)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}

		got, ok := readAgentsFile(dir)
		if !ok || got != content {
			t.Errorf("readAgentsFile() = %q, %v; want %q, true", got, ok, content)
		}
	})

	t.Run("broken symlink", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		if err := os.Symlink(filepath.Join(dir, "gone.md"), filepath.Join(dir, agentsFileName)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}

		if _, ok := readAgentsFile(dir); ok {
			t.Error("readAgentsFile() ok = true, want false for a broken symlink")
		}
	})

	t.Run("invalid UTF-8", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// The request body is JSON, so this must be rejected before it reaches
		// the encoder rather than producing a marshal error.
		writeFile(t, filepath.Join(dir, agentsFileName), "valid \xff\xfe invalid")

		if _, ok := readAgentsFile(dir); ok {
			t.Error("readAgentsFile() ok = true, want false for invalid UTF-8")
		}
	})

	t.Run("file at the size limit", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, agentsFileName), strings.Repeat("a", maxAgentsFileBytes))

		got, ok := readAgentsFile(dir)
		if !ok {
			t.Fatal("readAgentsFile() ok = false, want true at exactly the limit")
		}

		if len(got) != maxAgentsFileBytes {
			t.Errorf("len = %d, want %d", len(got), maxAgentsFileBytes)
		}
	})

	// Skipped, not truncated: silently halving instructions would drop exactly
	// the rules a large file is most likely to hold.
	t.Run("file over the size limit", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, agentsFileName), strings.Repeat("a", maxAgentsFileBytes+1))

		if _, ok := readAgentsFile(dir); ok {
			t.Error("readAgentsFile() ok = true, want false above the limit")
		}
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// agentsTree builds root/... with an AGENTS.md in each of dirs, returning root.
func agentsTree(t *testing.T, dirs map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for dir, content := range dirs {
		full := filepath.Join(root, filepath.FromSlash(dir))
		if err := os.MkdirAll(full, 0750); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}

		writeFile(t, filepath.Join(full, agentsFileName), content)
	}

	return root
}

// filesAt maps the resolved mount list to path -> content for order-insensitive
// assertions, and also reports the order.
func filesAt(files []request.AgentFile) (map[string]string, []string) {
	byPath := make(map[string]string, len(files))
	order := make([]string, 0, len(files))

	for _, f := range files {
		byPath[f.Path] = f.Content
		order = append(order, f.Path)
	}

	return byPath, order
}

// The nearest file occupies the slot the Antigravity runtime loads as system
// instructions, which is what makes the closest file take precedence.
func TestResolveAgentFiles_NearestTakesRootSlot(t *testing.T) {
	t.Parallel()

	root := agentsTree(t, map[string]string{
		"":           "root instructions",
		"sub":        "sub instructions",
		"sub/nested": "nested instructions",
	})

	got := agentFilesFrom(t.Context(), filepath.Join(root, "sub", "nested"), root)

	if len(got) == 0 {
		t.Fatal("resolveAgentFiles() returned no files")
	}

	if got[0].Path != request.RootMountPath {
		t.Errorf("first mount path = %q, want %q", got[0].Path, request.RootMountPath)
	}

	if got[0].Content != "nested instructions" {
		t.Errorf("first mount content = %q, want the nearest file", got[0].Content)
	}
}

// Ancestors are the precedence chain, ordered nearest first.
func TestResolveAgentFiles_AncestorsOrderedNearestFirst(t *testing.T) {
	t.Parallel()

	root := agentsTree(t, map[string]string{
		"":      "root instructions",
		"sub":   "sub instructions",
		"a":     "a instructions",
		"a/b":   "a/b instructions",
		"a/b/c": "deep instructions",
	})

	got := agentFilesFrom(t.Context(), filepath.Join(root, "a", "b", "c"), root)

	byPath, order := filesAt(got)

	if byPath[request.RootMountPath] != "deep instructions" {
		t.Errorf("root slot = %q, want the deepest file", byPath[request.RootMountPath])
	}

	for path, want := range map[string]string{
		"a/b/" + agentsFileName: "a/b instructions",
		"a/" + agentsFileName:   "a instructions",
	} {
		if byPath[path] != want {
			t.Errorf("mount %s = %q, want %q", path, byPath[path], want)
		}
	}

	// The repository-root file's natural path is the slot the nearest file
	// claimed, so it is dropped rather than mounted twice.
	for path, content := range byPath {
		if content == "root instructions" {
			t.Errorf("superseded root file still mounted at %s", path)
		}
	}

	if len(order) != 3 {
		t.Errorf("order = %v, want exactly the three files in the chain", order)
	}
}

// A subproject below the invocation directory ships its own file, and the
// repository root is reachable from anywhere.
func TestResolveAgentFiles_NestedSubprojectsAreMounted(t *testing.T) {
	t.Parallel()

	root := agentsTree(t, map[string]string{
		"":             "root instructions",
		"packages/ui":  "ui instructions",
		"packages/api": "api instructions",
	})

	got := agentFilesFrom(t.Context(), root, root)

	byPath, order := filesAt(got)

	for _, want := range []string{
		request.RootMountPath,
		"packages/ui/" + agentsFileName,
		"packages/api/" + agentsFileName,
	} {
		if _, ok := byPath[want]; !ok {
			t.Errorf("missing mount %q, got %v", want, order)
		}
	}
}

// Invoking from a subdirectory must not lose the files below it.
func TestResolveAgentFiles_FromSubdirectory(t *testing.T) {
	t.Parallel()

	root := agentsTree(t, map[string]string{
		"":            "root instructions",
		"packages":    "packages instructions",
		"packages/ui": "ui instructions",
	})

	got := agentFilesFrom(t.Context(), filepath.Join(root, "packages"), root)

	byPath, _ := filesAt(got)

	if byPath[request.RootMountPath] != "packages instructions" {
		t.Errorf("root slot = %q, want the invocation directory's file", byPath[request.RootMountPath])
	}

	if byPath["packages/ui/"+agentsFileName] != "ui instructions" {
		t.Errorf("missing the file below the invocation directory: %v", byPath)
	}
}

// The repository-root file would collide with the nearest file's slot, and the
// closer file supersedes it, so it is dropped rather than duplicated.
func TestResolveAgentFiles_RootFileSupersededByCloser(t *testing.T) {
	t.Parallel()

	root := agentsTree(t, map[string]string{
		"":    "root instructions",
		"sub": "sub instructions",
	})

	got := agentFilesFrom(t.Context(), filepath.Join(root, "sub"), root)

	byPath, _ := filesAt(got)

	if byPath[request.RootMountPath] != "sub instructions" {
		t.Errorf("root slot = %q, want the closer file", byPath[request.RootMountPath])
	}

	if byPath[agentsFileName] == "root instructions" && byPath[request.RootMountPath] != "root instructions" {
		t.Error("root file mounted twice under two paths")
	}

	if len(got) != 1 {
		t.Errorf("files = %d, want 1 (nothing below sub)", len(got))
	}
}

func TestDiscoveryStart(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sub := filepath.Join(root, "sub")

	cases := []struct {
		name string
		cwd  string
		root string
		want string
	}{
		{"cwd inside root is kept", sub, root, sub},
		{"cwd equal to root is kept", root, root, root},
		{"cwd outside root falls back to root", t.TempDir(), root, root},
		{"no root leaves cwd alone", sub, "", sub},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := discoveryStart(tc.cwd, tc.root); got != tc.want {
				t.Errorf("discoveryStart(%q, %q) = %q, want %q", tc.cwd, tc.root, got, tc.want)
			}
		})
	}
}

func TestResolveAgentFiles_NoFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if got := agentFilesFrom(t.Context(), filepath.Join(root, "sub"), root); len(got) != 0 {
		t.Errorf("resolveAgentFiles() = %v, want none", got)
	}
}

// A repository root that is not an ancestor of the invocation directory must not
// send the walk to the filesystem root; discovery falls back to the root.
func TestResolveAgentFiles_InvocationOutsideRoot(t *testing.T) {
	t.Parallel()

	root := agentsTree(t, map[string]string{
		"":    "root instructions",
		"sub": "sub instructions",
	})
	elsewhere := t.TempDir()

	if got := discoveryStart(elsewhere, root); got != root {
		t.Fatalf("discoveryStart(outside cwd) = %q, want the repository root", got)
	}

	got := agentFilesFrom(t.Context(), discoveryStart(elsewhere, root), root)

	byPath, _ := filesAt(got)
	if byPath[request.RootMountPath] != "root instructions" {
		t.Errorf("root slot = %q, want the repository root's file", byPath[request.RootMountPath])
	}

	if byPath["sub/"+agentsFileName] != "sub instructions" {
		t.Errorf("nested file missing: %v", byPath)
	}
}

func TestResolveAgentFiles_BudgetCapsFileCount(t *testing.T) {
	t.Parallel()

	dirs := map[string]string{"": "root instructions"}
	for i := range maxAgentsFiles + 5 {
		dirs[filepath.Join("p", string(rune('a'+i%26))+string(rune('a'+i/26)))] = "x"
	}

	root := agentsTree(t, dirs)

	if got := agentFilesFrom(t.Context(), root, root); len(got) > maxAgentsFiles {
		t.Errorf("files = %d, want at most %d", len(got), maxAgentsFiles)
	}
}

func TestResolveAgentFiles_BudgetCapsTotalBytes(t *testing.T) {
	t.Parallel()

	big := strings.Repeat("a", 40<<10)

	dirs := map[string]string{"": big}
	for i := range 8 {
		dirs[filepath.Join("p", string(rune('a'+i)))] = big
	}

	root := agentsTree(t, dirs)

	got := agentFilesFrom(t.Context(), root, root)

	var total int
	for _, f := range got {
		total += len(f.Content)
	}

	if total > maxAgentsTotalBytes {
		t.Errorf("total = %d, want at most %d", total, maxAgentsTotalBytes)
	}

	if len(got) == 0 {
		t.Error("no files mounted at all; the first file must always fit")
	}
}

// The scan skips ignored and vendored trees, so a large monorepo stays cheap.
func TestResolveAgentFiles_SkipsIgnoredAndVendorTrees(t *testing.T) {
	t.Parallel()

	root := agentsTree(t, map[string]string{
		"":             "root instructions",
		"node_modules": "should not be mounted",
		"vendor/dep":   "should not be mounted",
		"real":         "real instructions",
	})

	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")

	if err := os.MkdirAll(filepath.Join(root, "ignored"), 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeFile(t, filepath.Join(root, "ignored", agentsFileName), "should not be mounted")

	byPath, order := filesAt(agentFilesFrom(t.Context(), root, root))

	for bad := range map[string]string{
		"node_modules/" + agentsFileName: "",
		"vendor/dep/" + agentsFileName:   "",
		"ignored/" + agentsFileName:      "",
	} {
		if _, ok := byPath[bad]; ok {
			t.Errorf("mounted a skipped tree: %q among %v", bad, order)
		}
	}

	if byPath["real/"+agentsFileName] != "real instructions" {
		t.Errorf("real file missing, got %v", order)
	}
}

// TestResolveAgentFiles_NoFile exercises the real process-cwd lookup with
// nothing to find. It is a top-level test rather than a subtest because
// t.Chdir cannot run inside a parallel parent, and the working directory must
// move away from a repository that has its own AGENTS.md.
func TestResolveAgentFiles_NoFile(t *testing.T) {
	t.Chdir(t.TempDir())

	// An empty AppContext has no memoised repo root, so this also covers the
	// "outside a repository" path: discovery yields nothing either way.
	if got := resolveAgentFiles(t.Context(), &AppContext{}); len(got) != 0 {
		t.Errorf("resolveAgentFiles() = %v, want none", got)
	}
}

// A realistic monorepo, invoked from a nested package. This is the regression
// lock for the case that motivated nested support: the invocation directory's
// file wins the root slot, its ancestors follow, and trees that are ignored or
// vendored are never scanned.
func TestResolveAgentFiles_MonorepoEndToEnd(t *testing.T) {
	root := agentsTree(t, map[string]string{
		"":                          "monorepo root: conventional commits",
		"services":                  "services layer",
		"services/payments":         "payments: name the ticket",
		"services/payments/gateway": "gateway: never log PANs",
		"node_modules/dep":          "IGNORED",
	})

	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")
	writeFile(t, filepath.Join(root, ".git"), "gitdir: /elsewhere")

	if err := os.MkdirAll(filepath.Join(root, "ignored"), 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeFile(t, filepath.Join(root, "ignored", agentsFileName), "IGNORED")

	t.Chdir(filepath.Join(root, "services", "payments", "gateway"))

	got := resolveAgentFiles(t.Context(), &AppContext{})

	byPath, _ := filesAt(got)

	want := map[string]string{
		request.RootMountPath:                 "gateway: never log PANs",
		"services/payments/" + agentsFileName: "payments: name the ticket",
		"services/" + agentsFileName:          "services layer",
	}

	if len(got) != len(want) {
		t.Errorf("mounted %d files (%v), want %d", len(got), byPath, len(want))
	}

	for path, content := range want {
		if byPath[path] != content {
			t.Errorf("mount %s = %q, want %q", path, byPath[path], content)
		}
	}

	for path, content := range byPath {
		if content == "IGNORED" {
			t.Errorf("mounted a skipped tree at %s", path)
		}

		// The repository-root file lost the root slot to a closer file and is
		// therefore not mounted at all.
		if content == "monorepo root: conventional commits" {
			t.Errorf("superseded root file still mounted at %s", path)
		}
	}
}

// upRoot falls back to the nearest .git ancestor when git cannot resolve the
// repository, so ancestor files survive a failed repo-root lookup.
func TestUpRoot(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()

	nested := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(nested, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeFile(t, filepath.Join(repo, ".git"), "gitdir: /elsewhere")

	if got := upRoot(nested, ""); got != repo {
		t.Errorf("upRoot(no git root) = %q, want the .git ancestor %q", got, repo)
	}

	// An authoritative root that contains start beats .git detection, even
	// when .git sits deeper.
	outer := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outer, filepath.FromSlash("a/b")), 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	deep := filepath.Join(outer, "a", "b")
	if got := upRoot(deep, outer); got != outer {
		t.Errorf("upRoot(authoritative root) = %q, want %q", got, outer)
	}

	// A root that does not contain start is ignored in favour of .git.
	if got := upRoot(nested, t.TempDir()); got != repo {
		t.Errorf("upRoot(unrelated root) = %q, want the .git ancestor %q", got, repo)
	}
}
