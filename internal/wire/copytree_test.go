package wire

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyTree(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	writeFile(t, src, "a.txt", "alpha")
	writeFile(t, src, "sub/b.txt", "beta")

	target := filepath.Join(t.TempDir(), "out")

	n, err := CopyTree(src, target, 1<<20)
	if err != nil {
		t.Fatalf("CopyTree: %v", err)
	}

	if n != 2 {
		t.Fatalf("files = %d, want 2", n)
	}

	data, err := os.ReadFile(filepath.Join(target, "sub", "b.txt"))
	if err != nil || string(data) != "beta" {
		t.Fatalf("nested file = %q, err = %v", data, err)
	}
}

func TestCopyTreeSkipsLinks(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	writeFile(t, src, "a.txt", "alpha")

	if err := os.Symlink("a.txt", filepath.Join(src, "link.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	target := filepath.Join(t.TempDir(), "out")

	n, err := CopyTree(src, target, 1<<20)
	if err != nil {
		t.Fatalf("CopyTree: %v", err)
	}

	if n != 1 {
		t.Fatalf("files = %d, want 1 (link skipped)", n)
	}

	if _, err := os.Lstat(filepath.Join(target, "link.txt")); !os.IsNotExist(err) {
		t.Fatal("symlink materialized in target")
	}
}

func TestCopyTreeBudget(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	writeFile(t, src, "big.txt", "1234567890")

	if _, err := CopyTree(src, filepath.Join(t.TempDir(), "out"), 4); err == nil {
		t.Fatal("expected budget error")
	}
}

func TestCopyTreeMissingSource(t *testing.T) {
	t.Parallel()

	if _, err := CopyTree(filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "out"), 1<<20); err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestCopyTreeEmpty(t *testing.T) {
	t.Parallel()

	n, err := CopyTree(t.TempDir(), filepath.Join(t.TempDir(), "out"), 1<<20)
	if err != nil {
		t.Fatalf("CopyTree: %v", err)
	}

	if n != 0 {
		t.Fatalf("files = %d, want 0", n)
	}
}
