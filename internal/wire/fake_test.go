package wire

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFetcher scripts Fetcher behavior for orchestration tests. Files maps a
// commit SHA to relative-path → content pairs; commits maps ref → SHA.
type fakeFetcher struct {
	commits map[string]string
	files   map[string]map[string]string

	resolveErr     error
	materializeErr error

	resolves     int
	materializes int
}

func (f *fakeFetcher) Resolve(_ context.Context, source SourceRef) (Resolution, error) {
	f.resolves++

	if f.resolveErr != nil {
		return Resolution{}, f.resolveErr
	}

	sha, ok := f.commits[source.Ref]
	if !ok {
		return Resolution{}, fmt.Errorf("ref %q: %w", source.Ref, ErrUnknownRef)
	}

	return Resolution{Commit: sha, Ref: source.Ref, Subpath: source.Subpath}, nil
}

func (f *fakeFetcher) Materialize(_ context.Context, _ SourceRef, res Resolution, dir string) (int, error) {
	f.materializes++

	if f.materializeErr != nil {
		return 0, f.materializeErr
	}

	files, ok := f.files[res.Commit]
	if !ok {
		return 0, fmt.Errorf("commit %q: %w", ShortSHA(res.Commit), ErrMissingPath)
	}

	for name, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))

		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return 0, err
		}

		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			return 0, err
		}
	}

	return len(files), nil
}

func TestFakeFetcherResolvesAndMaterializes(t *testing.T) {
	t.Parallel()

	f := &fakeFetcher{
		commits: map[string]string{"19.0": strings.Repeat("a", 40)},
		files:   map[string]map[string]string{strings.Repeat("a", 40): {"a.txt": "hi"}},
	}

	src := SourceRef{Ref: "19.0", Subpath: "auto_backup"}

	res, err := f.Resolve(context.Background(), src)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	n, err := f.Materialize(context.Background(), src, res, t.TempDir())
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}

	if n != 1 {
		t.Fatalf("files = %d, want 1", n)
	}
}
