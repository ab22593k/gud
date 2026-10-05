package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testCommitA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testCommitB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func testSource() SourceRef {
	return SourceRef{
		Host:      "github.com",
		Owner:     "OCA",
		Repo:      "server-tools",
		Ref:       "19.0",
		Subpath:   "auto_backup",
		SourceURL: "https://github.com/OCA/server-tools/tree/19.0/auto_backup",
	}
}

func successFetcher() *fakeFetcher {
	return &fakeFetcher{
		commits: map[string]string{"19.0": testCommitA},
		files: map[string]map[string]string{
			testCommitA: {"a.txt": "alpha", "sub/b.txt": "beta"},
		},
	}
}

func TestFetchSuccess(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "auto_backup")

	summary, err := Fetch(context.Background(), FetchOptions{Fetcher: successFetcher()}, testSource(), target)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !strings.Contains(summary, "Fetched github.com/OCA/server-tools@19.0:auto_backup") {
		t.Fatalf("summary missing display: %q", summary)
	}

	for _, name := range []string{"a.txt", "sub/b.txt"} {
		data, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}

		if len(data) == 0 {
			t.Fatalf("empty file %s", name)
		}
	}

	rec, err := LoadRecord(target)
	if err != nil {
		t.Fatalf("LoadRecord: %v", err)
	}

	if rec.ResolvedCommit != testCommitA {
		t.Fatalf("commit = %q", rec.ResolvedCommit)
	}

	live, err := HashDir(target)
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	if rec.ExportHash != live {
		t.Fatal("record hash does not match checkout")
	}
}

func TestFetchUnknownRef(t *testing.T) {
	t.Parallel()

	src := testSource()
	src.Ref = "no-such-branch"

	_, err := Fetch(context.Background(), FetchOptions{Fetcher: successFetcher()}, src, filepath.Join(t.TempDir(), "x"))
	if !errors.Is(err, ErrUnknownRef) {
		t.Fatalf("err = %v, want ErrUnknownRef", err)
	}
}

func TestFetchMissingUpstreamPath(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	f.files = map[string]map[string]string{}
	target := filepath.Join(t.TempDir(), "x")

	_, err := Fetch(context.Background(), FetchOptions{Fetcher: f}, testSource(), target)
	if !errors.Is(err, ErrMissingPath) {
		t.Fatalf("err = %v, want ErrMissingPath", err)
	}

	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatal("failed fetch left a target behind")
	}
}

func TestFetchNonEmptyTarget(t *testing.T) {
	t.Parallel()

	target := t.TempDir()
	writeFile(t, target, "mine.txt", "keep me")

	_, err := Fetch(context.Background(), FetchOptions{Fetcher: successFetcher()}, testSource(), target)
	if !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("err = %v, want ErrTargetNotEmpty", err)
	}

	if _, statErr := os.Stat(filepath.Join(target, "mine.txt")); statErr != nil {
		t.Fatal("refusal removed local file")
	}

	forced, err := Fetch(context.Background(), FetchOptions{Fetcher: successFetcher(), Force: true}, testSource(), target)
	if err != nil {
		t.Fatalf("forced Fetch: %v", err)
	}

	if forced == "" {
		t.Fatal("empty summary")
	}

	if _, statErr := os.Stat(filepath.Join(target, "mine.txt")); !os.IsNotExist(statErr) {
		t.Fatal("force did not replace target contents")
	}
}

func TestFetchGuardsTargetBeforeNetwork(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	f.resolveErr = context.DeadlineExceeded

	target := t.TempDir()
	writeFile(t, target, "mine.txt", "keep me")

	_, err := Fetch(context.Background(), FetchOptions{Fetcher: f}, testSource(), target)
	if !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("err = %v, want local guard before network", err)
	}
}
