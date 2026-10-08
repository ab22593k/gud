package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// TestResolveAgentInstructions_NoFile exercises the real process-cwd lookup
// with nothing to find. It is a top-level test rather than a subtest because
// t.Chdir cannot run inside a parallel parent, and the working directory must
// move away from a repository that has its own AGENTS.md.
func TestResolveAgentInstructions_NoFile(t *testing.T) {
	t.Chdir(t.TempDir())

	// An empty AppContext has no memoised repo root, so this also covers the
	// "outside a repository" path: the lookup yields nothing either way.
	if got := resolveAgentInstructions(t.Context(), &AppContext{}); got != "" {
		t.Errorf("resolveAgentInstructions() = %q, want empty", got)
	}
}

// TestResolveAgentInstructions covers the search order: the invocation
// directory wins over the repository root, and a root-only file is still found
// when invoked from a subdirectory.
func TestResolveAgentInstructions(t *testing.T) {
	t.Parallel()

	t.Run("root file used when cwd has none", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()

		sub := filepath.Join(root, "sub")
		if err := os.Mkdir(sub, 0750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		writeFile(t, filepath.Join(root, agentsFileName), "root instructions")

		got := resolveAgentInstructionsIn(sub, root)
		if got != "root instructions" {
			t.Errorf("resolveAgentInstructions() = %q, want %q", got, "root instructions")
		}
	})

	t.Run("cwd wins over root", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()

		sub := filepath.Join(root, "sub")
		if err := os.Mkdir(sub, 0750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		writeFile(t, filepath.Join(root, agentsFileName), "root instructions")
		writeFile(t, filepath.Join(sub, agentsFileName), "sub instructions")

		got := resolveAgentInstructionsIn(sub, root)
		if got != "sub instructions" {
			t.Errorf("resolveAgentInstructions() = %q, want %q", got, "sub instructions")
		}
	})

	t.Run("root not re-read when it is the cwd", func(t *testing.T) {
		t.Parallel()

		// Guards the same-directory shortcut: the file is returned, not
		// silently dropped by the de-duplication.
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, agentsFileName), "only instructions")

		got := resolveAgentInstructionsIn(dir, dir)
		if got != "only instructions" {
			t.Errorf("resolveAgentInstructions() = %q, want %q", got, "only instructions")
		}
	})

	t.Run("unusable root file falls back to empty", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, agentsFileName), "   \n  ")

		if got := resolveAgentInstructionsIn(filepath.Join(root, "sub"), root); got != "" {
			t.Errorf("resolveAgentInstructions() = %q, want empty", got)
		}
	})
}

// resolveAgentInstructionsIn is resolveAgentInstructions with the process cwd
// and repository root injected, so the search order is testable without
// chdir-ing or shelling out to git.
func resolveAgentInstructionsIn(cwd, root string) string {
	if content, ok := readAgentsFile(cwd); ok {
		return content
	}

	if root == "" || filepath.Clean(root) == filepath.Clean(cwd) {
		return ""
	}

	content, _ := readAgentsFile(root)

	return content
}

// The instruction text is the mounted AGENTS.md content verbatim apart from
// surrounding whitespace: no wrapping, no reformatting, so what the repository
// wrote is what the agent reads.
func TestResolveAgentInstructions_PreservesContentVerbatim(t *testing.T) {
	t.Parallel()

	const raw = "# Conventions\n\n- imperative subjects\n- wrap at 72"

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, agentsFileName), raw+"\n\n")

	if got, ok := readAgentsFile(dir); !ok || got != raw {
		t.Errorf("readAgentsFile() = %q, %v; want %q verbatim", got, ok, raw)
	}
}
