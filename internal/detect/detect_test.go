package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeStats_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	stats, err := ComputeStats(dir)
	if err != nil {
		t.Fatalf("ComputeStats(empty) = _, %v; want nil error", err)
	}
	if stats.TotalFiles != 0 {
		t.Errorf("TotalFiles = %d, want 0", stats.TotalFiles)
	}
	if len(stats.FilesByExtension) != 0 {
		t.Errorf("FilesByExtension = %v, want empty", stats.FilesByExtension)
	}
}

func TestComputeStats_SingleExtension(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	files := []string{"main.go", "handler.go", "utils.go"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("package main"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := ComputeStats(dir)
	if err != nil {
		t.Fatalf("ComputeStats = _, %v; want nil error", err)
	}
	if stats.TotalFiles != 3 {
		t.Errorf("TotalFiles = %d, want 3", stats.TotalFiles)
	}
	if stats.FilesByExtension[".go"] != 3 {
		t.Errorf("FilesByExtension['.go'] = %d, want 3", stats.FilesByExtension[".go"])
	}
}

func TestComputeStats_MultipleExtensions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "app.ts"), []byte("const x = 1;"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "style.css"), []byte("body {}"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Project"), 0600)

	stats, err := ComputeStats(dir)
	if err != nil {
		t.Fatalf("ComputeStats = _, %v; want nil error", err)
	}
	if stats.TotalFiles != 4 {
		t.Errorf("TotalFiles = %d, want 4", stats.TotalFiles)
	}
	for _, ext := range []string{".go", ".ts", ".css", ".md"} {
		if stats.FilesByExtension[ext] != 1 {
			t.Errorf("FilesByExtension[%q] = %d, want 1", ext, stats.FilesByExtension[ext])
		}
	}
}

func TestComputeStats_SkipsGitDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_ = os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0750)
	_ = os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0600)
	_ = os.WriteFile(filepath.Join(dir, ".git", "objects", "pack"), []byte("data"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0600)

	stats, err := ComputeStats(dir)
	if err != nil {
		t.Fatalf("ComputeStats = _, %v; want nil error", err)
	}
	if stats.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1 (should skip .git)", stats.TotalFiles)
	}
	if stats.FilesByExtension[".go"] != 1 {
		t.Errorf("FilesByExtension['.go'] = %d, want 1", stats.FilesByExtension[".go"])
	}
}

func TestComputeStats_SkipsGitignoredDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Vendored/generated trees listed in the repo's own .gitignore must not
	// dominate the walk or the prompt stats.
	gitignore := []byte("node_modules/\nvendor/\nbuild/\nvenv/\n")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), gitignore, 0600); err != nil {
		t.Fatal(err)
	}

	_ = os.MkdirAll(filepath.Join(dir, "node_modules", "pkg", "src"), 0750)
	_ = os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "src", "index.js"), []byte("x"), 0600)
	_ = os.MkdirAll(filepath.Join(dir, "vendor", "github.com", "x"), 0750)
	_ = os.WriteFile(filepath.Join(dir, "vendor", "github.com", "x", "lib.go"), []byte("package x"), 0600)
	_ = os.MkdirAll(filepath.Join(dir, "build", "out"), 0750)
	_ = os.WriteFile(filepath.Join(dir, "build", "out", "app"), []byte("binary"), 0600)
	_ = os.MkdirAll(filepath.Join(dir, "venv", "lib"), 0750)
	_ = os.WriteFile(filepath.Join(dir, "venv", "lib", "site.py"), []byte("import sys"), 0600)

	// Real source alongside the vendored trees: main.go plus .gitignore.
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0600)

	stats, err := ComputeStats(dir)
	if err != nil {
		t.Fatalf("ComputeStats = _, %v; want nil error", err)
	}
	if stats.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2 (main.go + .gitignore; vendored dirs skipped)", stats.TotalFiles)
	}
	if stats.FilesByExtension[".go"] != 1 {
		t.Errorf("FilesByExtension['.go'] = %d, want 1", stats.FilesByExtension[".go"])
	}
}

func TestComputeStats_GitignorePatternSemantics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		gitignore string
		// build creates the tree; each entry is a slash-relative path.
		files     map[string]string
		wantFiles int
	}{
		{
			name:      "unignored dirs are walked",
			gitignore: "node_modules/\n",
			files: map[string]string{
				"main.go":     "package main",
				"app/util.go": "package util",
			},
			wantFiles: 3, // main.go, app/util.go, .gitignore
		},
		{
			name:      "basename match applies at any depth",
			gitignore: "node_modules/\n",
			files: map[string]string{
				"main.go":                         "package main",
				"a/b/c/node_modules/pkg/index.js": "x",
			},
			wantFiles: 2, // main.go, .gitignore
		},
		{
			name:      "root-anchored pattern only matches at root",
			gitignore: "/dist/\n",
			files: map[string]string{
				"main.go":            "package main",
				"dist/bundle.js":     "x",
				"src/dist/bundle.js": "x",
			},
			wantFiles: 3, // main.go, src/dist/bundle.js, .gitignore
		},
		{
			name:      "negation re-includes an excluded directory",
			gitignore: "vendor/\n!vendor/\n",
			files: map[string]string{
				"main.go":          "package main",
				"vendor/drop/a.go": "package a",
				"vendor/keep/b.go": "package b",
			},
			wantFiles: 4, // main.go, vendor/drop/a.go, vendor/keep/b.go, .gitignore
		},
		{
			name:      "negation cannot re-include under an excluded parent",
			gitignore: "vendor/\n!vendor/keep/\n",
			files: map[string]string{
				"main.go":          "package main",
				"vendor/drop/a.go": "package a",
				"vendor/keep/b.go": "package b",
			},
			wantFiles: 2, // main.go, .gitignore (vendor/ pruned at the parent, as git does)
		},
		{
			name:      "glob patterns match",
			gitignore: "*.cache/\n",
			files: map[string]string{
				"main.go":         "package main",
				"x.cache/data.js": "x",
				"keep.go":         "package keep",
			},
			wantFiles: 3, // main.go, keep.go, .gitignore
		},
		{
			name:      "comments and blank lines are ignored",
			gitignore: "# vendored\n\nnode_modules/\n",
			files: map[string]string{
				"main.go":                   "package main",
				"node_modules/pkg/index.js": "x",
			},
			wantFiles: 2, // main.go, .gitignore
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()

			if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(tt.gitignore), 0600); err != nil {
				t.Fatal(err)
			}
			for rel, content := range tt.files {
				p := filepath.Join(dir, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}

			stats, err := ComputeStats(dir)
			if err != nil {
				t.Fatalf("ComputeStats = _, %v; want nil error", err)
			}
			if stats.TotalFiles != tt.wantFiles {
				t.Errorf("TotalFiles = %d, want %d", stats.TotalFiles, tt.wantFiles)
			}
		})
	}
}

func TestComputeStats_NoExtensionFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_ = os.WriteFile(filepath.Join(dir, "Makefile"), []byte("all:\n"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM ubuntu"), 0600)

	stats, err := ComputeStats(dir)
	if err != nil {
		t.Fatalf("ComputeStats = _, %v; want nil error", err)
	}
	if stats.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", stats.TotalFiles)
	}
	if stats.FilesByExtension["(no extension)"] != 2 {
		t.Errorf("FilesByExtension['(no extension)'] = %d, want 2", stats.FilesByExtension["(no extension)"])
	}
}

func TestFormatRepoContext_Empty(t *testing.T) {
	t.Parallel()

	if got := FormatRepoContext(nil); got != "" {
		t.Errorf("FormatRepoContext(nil) = %q, want ''", got)
	}

	empty := &RepoStats{FilesByExtension: make(map[string]int)}
	if got := FormatRepoContext(empty); got != "" {
		t.Errorf("FormatRepoContext(empty) = %q, want ''", got)
	}
}

func TestFormatRepoContext_SingleExt(t *testing.T) {
	t.Parallel()

	stats := &RepoStats{
		FilesByExtension: map[string]int{".go": 5},
		TotalFiles:       5,
	}

	got := FormatRepoContext(stats)

	if !strings.Contains(got, "Repository: 5 files across 1 extensions") {
		t.Errorf("FormatRepoContext missing summary, got:\n%s", got)
	}
	if !strings.Contains(got, ".go") || !strings.Contains(got, "5") || !strings.Contains(got, "100%") {
		t.Errorf("FormatRepoContext missing .go/5/100%%, got:\n%s", got)
	}
}

func TestFormatRepoContext_MultipleExts(t *testing.T) {
	t.Parallel()

	stats := &RepoStats{
		FilesByExtension: map[string]int{".go": 12, ".ts": 7, ".css": 3, ".md": 2},
		TotalFiles:       24,
	}

	got := FormatRepoContext(stats)

	// Should be sorted by count desc, then extension alpha
	if !strings.Contains(got, "Repository: 24 files across 4 extensions") {
		t.Errorf("FormatRepoContext missing summary, got:\n%s", got)
	}
	if !strings.Contains(got, ".go") || !strings.Contains(got, "50%") {
		t.Errorf("FormatRepoContext missing .go/50%%, got:\n%s", got)
	}
	if !strings.Contains(got, ".ts") || !strings.Contains(got, "29%") {
		t.Errorf("FormatRepoContext missing .ts/29%%, got:\n%s", got)
	}
	if !strings.Contains(got, ".css") || !strings.Contains(got, "12%") {
		t.Errorf("FormatRepoContext missing .css/12%%, got:\n%s", got)
	}
	if !strings.Contains(got, ".md") || !strings.Contains(got, "8%") {
		t.Errorf("FormatRepoContext missing .md/8%%, got:\n%s", got)
	}

	// .go should appear before .ts (sorted by count desc)
	if strings.Index(got, ".go") > strings.Index(got, ".ts") {
		t.Errorf("FormatRepoContext: .go should appear before .ts")
	}
}
