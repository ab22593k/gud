package wire

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(name))

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestHashDirDeterministic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "b.txt", "bee")
	writeFile(t, dir, "a/c.txt", "see")

	first, err := HashDir(dir)
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	second, err := HashDir(dir)
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	if first != second || len(first) != 64 {
		t.Fatalf("unstable hash: %q vs %q", first, second)
	}
}

func TestHashDirDetectsChange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "one")

	before, err := HashDir(dir)
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	writeFile(t, dir, "f.txt", "two")

	after, err := HashDir(dir)
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	if before == after {
		t.Fatal("hash did not change with content")
	}
}

func TestHashDirIgnoresRegistry(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "data")

	before, err := HashDir(dir)
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	writeFile(t, dir, registryFileName, `{"version":1}`)

	after, err := HashDir(dir)
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	if before != after {
		t.Fatal("registry file changed the content hash")
	}
}

func TestHashDirEmpty(t *testing.T) {
	t.Parallel()

	got, err := HashDir(t.TempDir())
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	if len(got) != 64 || strings.Trim(got, "0123456789abcdef") != "" {
		t.Fatalf("bad empty hash %q", got)
	}
}

// TestHashDirFollowsSymlinkToRegular guards the listFiles fast path: WalkDir
// reports a symlink's own type, so a naive type check would skip links. A
// link to a regular file must still count — removing it changes the hash.
func TestHashDirFollowsSymlinkToRegular(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "real.txt", "content")

	if err := os.Symlink("real.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	before, err := HashDir(dir)
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	if err := os.Remove(filepath.Join(dir, "link.txt")); err != nil {
		t.Fatalf("remove link: %v", err)
	}

	after, err := HashDir(dir)
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	if before == after {
		t.Fatal("symlinked file not hashed: removing the link left the hash unchanged")
	}
}
