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

// fetchRun mints a run directory: registry at its root, targets beneath it.
func fetchRun(t *testing.T, targetName string) (registry, target string) {
	t.Helper()

	run := t.TempDir()

	return RegistryPath(run), filepath.Join(run, targetName)
}

func fetchWithRegistry(t *testing.T, f *fakeFetcher, registry, target string) string {
	t.Helper()

	summary, err := Fetch(context.Background(), FetchOptions{Fetcher: f, RegistryPath: registry}, testSource(), target)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	return summary
}

func TestFetchSuccess(t *testing.T) {
	t.Parallel()

	registry, target := fetchRun(t, "auto_backup")

	summary := fetchWithRegistry(t, successFetcher(), registry, target)

	if !strings.Contains(summary, "Fetched github.com/OCA/server-tools@19.0:auto_backup") {
		t.Fatalf("summary missing display: %q", summary)
	}

	if !strings.Contains(summary, "Tracked for future updates ("+registry) {
		t.Fatalf("summary missing registry path: %q", summary)
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

	if _, err := os.Stat(filepath.Join(target, registryFileName)); !os.IsNotExist(err) {
		t.Fatal("target holds bookkeeping files; fetched folders stay pristine")
	}

	reg, err := LoadRegistry(registry)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	entry, err := reg.Lookup("./auto_backup")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	if entry.ResolvedCommit != testCommitA {
		t.Fatalf("commit = %q", entry.ResolvedCommit)
	}

	live, err := HashDir(target)
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	if entry.ExportHash != live {
		t.Fatal("entry hash does not match checkout")
	}
}

func TestFetchUnknownRef(t *testing.T) {
	t.Parallel()

	src := testSource()
	src.Ref = "no-such-branch"

	registry, target := fetchRun(t, "x")

	_, err := Fetch(context.Background(), FetchOptions{Fetcher: successFetcher(), RegistryPath: registry}, src, target)
	if !errors.Is(err, ErrUnknownRef) {
		t.Fatalf("err = %v, want ErrUnknownRef", err)
	}
}

func TestFetchMissingUpstreamPath(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	f.files = map[string]map[string]string{}

	registry, target := fetchRun(t, "x")

	_, err := Fetch(context.Background(), FetchOptions{Fetcher: f, RegistryPath: registry}, testSource(), target)
	if !errors.Is(err, ErrMissingPath) {
		t.Fatalf("err = %v, want ErrMissingPath", err)
	}

	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatal("failed fetch left a target behind")
	}
}

func TestFetchOutsideRegistryTree(t *testing.T) {
	t.Parallel()

	run := t.TempDir()
	registry := RegistryPath(run)
	outside := filepath.Join(t.TempDir(), "elsewhere")

	_, err := Fetch(context.Background(), FetchOptions{Fetcher: successFetcher(), RegistryPath: registry},
		testSource(), outside)
	if !errors.Is(err, ErrNotACheckout) {
		t.Fatalf("err = %v, want ErrNotACheckout", err)
	}

	if _, statErr := os.Stat(outside); !os.IsNotExist(statErr) {
		t.Fatal("rejected fetch left a target behind")
	}
}

func TestFetchNonEmptyTarget(t *testing.T) {
	t.Parallel()

	registry, target := fetchRun(t, "co")
	writeFile(t, target, "mine.txt", "keep me")

	_, err := Fetch(context.Background(), FetchOptions{Fetcher: successFetcher(), RegistryPath: registry},
		testSource(), target)
	if !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("err = %v, want ErrTargetNotEmpty", err)
	}

	if _, statErr := os.Stat(filepath.Join(target, "mine.txt")); statErr != nil {
		t.Fatal("refusal removed local file")
	}

	forced, err := Fetch(context.Background(),
		FetchOptions{Fetcher: successFetcher(), Force: true, RegistryPath: registry}, testSource(), target)
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

	registry, target := fetchRun(t, "co")
	writeFile(t, target, "mine.txt", "keep me")

	_, err := Fetch(context.Background(), FetchOptions{Fetcher: f, RegistryPath: registry}, testSource(), target)
	if !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("err = %v, want local guard before network", err)
	}
}
