package detect

import (
	"os"
	"path/filepath"
	"testing"
)

// benchWalk generates a synthetic repo tree with n files spread over dirs
// and returns the root, mirroring a real project (node_modules-style bloat
// included) to expose the cost of ComputeStats' full-tree walk.
func benchWalk(b *testing.B, files, dirs int) string {
	b.Helper()

	root := b.TempDir()
	for i := range dirs {
		dir := filepath.Join(root, "pkg", "d"+string(rune('a'+i%26))+string(rune('0'+i/26)))
		if err := os.MkdirAll(dir, 0750); err != nil {
			b.Fatal(err)
		}

		for j := range files / dirs {
			if err := os.WriteFile(
				filepath.Join(dir, "f"+string(rune('a'+j%26))+".go"),
				[]byte("package x\n"), 0600,
			); err != nil {
				b.Fatal(err)
			}
		}
	}
	// node_modules-style bloat: one dir with many small files, ignored via the
	// repo's own .gitignore exactly as a real project would declare it.
	_ = os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules/\n"), 0600)
	bloat := filepath.Join(root, "node_modules", "pkg")

	_ = os.MkdirAll(bloat, 0750)
	for j := range files {
		_ = os.WriteFile(
			filepath.Join(bloat, "m"+string(rune('a'+j%26))+".js"),
			[]byte("x"), 0600)
	}

	return root
}

func BenchmarkComputeStats_500Files(b *testing.B) {
	root := benchWalk(b, 250, 10)
	b.ResetTimer()

	for b.Loop() {
		if _, err := ComputeStats(root); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkComputeStats_10kFiles(b *testing.B) {
	root := benchWalk(b, 5000, 25)
	b.ResetTimer()

	for b.Loop() {
		if _, err := ComputeStats(root); err != nil {
			b.Fatal(err)
		}
	}
}
