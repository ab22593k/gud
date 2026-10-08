package wire

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Local-cost benchmarks for git-wire. Everything here runs offline against
// fake fetchers and temp dirs — no network, no credentials — so they measure
// exactly the work gud controls. The model/API round trip is out of scope;
// the point is keeping local hashing, copying, and scanning from creeping.
//
// Timings on a loaded workstation vary several-fold; allocated bytes and
// allocation counts are deterministic — treat those as the regression signal.

// benchTree builds a tree of 400 4 KiB files spread across subdirectories,
// a shape that exercises walk, stat, read, and hash paths at a realistic
// checkout size.
func benchTree(b *testing.B) string {
	b.Helper()

	dir := b.TempDir()
	content := strings.Repeat("x", 4096)

	for i := range 400 {
		p := filepath.Join(dir, fmt.Sprintf("dir%02d", i%20), fmt.Sprintf("file%04d.txt", i))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			b.Fatal(err)
		}

		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			b.Fatal(err)
		}
	}

	return dir
}

func BenchmarkHashDir(b *testing.B) {
	dir := benchTree(b)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		if _, err := HashDir(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSnapshotDir(b *testing.B) {
	dir := benchTree(b)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		if _, err := snapshotDir(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCopyTree(b *testing.B) {
	src := benchTree(b)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		if _, err := CopyTree(src, filepath.Join(b.TempDir(), "out"), 1<<30); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCloneCheckout(b *testing.B) {
	src := benchTree(b)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		if err := cloneCheckout(src, filepath.Join(b.TempDir(), "out")); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCheckTargetForce covers the --force path on a wide directory: the
// guard must answer "non-empty" without reading and sorting every entry.
func BenchmarkCheckTargetForce(b *testing.B) {
	dir := b.TempDir()
	for i := range 5000 {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%05d", i)), []byte("x"), 0o600); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		if err := checkTarget(dir, true); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkList exercises the multi-checkout status path: per-entry hashing
// (parallel) plus memoized resolution (sequential).
func BenchmarkList(b *testing.B) {
	commit := strings.Repeat("a", 40)
	files := make(map[string]string, 100)

	for i := range 100 {
		files[fmt.Sprintf("dir%02d/file%04d.txt", i%10, i)] = strings.Repeat("x", 4096)
	}

	f := &fakeFetcher{
		commits: map[string]string{"19.0": commit},
		files:   map[string]map[string]string{commit: files},
	}

	root := b.TempDir()
	registry := RegistryPath(root)

	for i := range 8 {
		src := SourceRef{Host: "h", Owner: "o", Repo: "r", Ref: "19.0", Subpath: "s", SourceURL: "https://h/o/r/tree/19.0/s"}
		if _, err := Fetch(context.Background(), FetchOptions{Fetcher: f, RegistryPath: registry},
			src, filepath.Join(root, fmt.Sprintf("co%02d", i))); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		entries, err := List(context.Background(), f, root)
		if err != nil {
			b.Fatal(err)
		}

		if len(entries) != 8 {
			b.Fatalf("entries = %d, want 8", len(entries))
		}
	}
}

// BenchmarkMergeSnaps exercises the three-snapshot staging at the heart of a
// conflict-free update (base + local + new, hashed concurrently).
func BenchmarkMergeSnaps(b *testing.B) {
	base := strings.Repeat("a", 40)
	commit := strings.Repeat("b", 40)

	baseFiles := make(map[string]string, 100)
	for i := range 100 {
		baseFiles[fmt.Sprintf("f%03d.txt", i)] = strings.Repeat("y", 4096)
	}

	newFiles := make(map[string]string, len(baseFiles))
	maps.Copy(newFiles, baseFiles)

	newFiles["f000.txt"] = "changed"

	f := &fakeFetcher{
		commits: map[string]string{"19.0": commit},
		files:   map[string]map[string]string{base: baseFiles, commit: newFiles},
	}

	dir := b.TempDir()
	for name, content := range baseFiles {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(content), 0o600); err != nil {
			b.Fatal(err)
		}
	}

	src := SourceRef{Host: "h", Owner: "o", Repo: "r", Ref: "19.0", Subpath: "s"}
	baseRes := Resolution{Commit: base, Ref: "19.0", Subpath: "s"}
	newRes := Resolution{Commit: commit, Ref: "19.0", Subpath: "s"}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		snaps, cleanup, err := stageMergeSnaps(context.Background(), f, src, baseRes, newRes, dir)
		if err != nil {
			b.Fatalf("stage: %v", err)
		}

		cleanup()

		if len(snaps.base) != 100 || len(snaps.local) != 100 || len(snaps.new) != 100 {
			b.Fatalf("snapshots hold %d/%d/%d entries, want 100 each",
				len(snaps.base), len(snaps.local), len(snaps.new))
		}
	}
}
