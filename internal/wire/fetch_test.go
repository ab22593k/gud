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

func TestFetchNoOp(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	registry, target := fetchRun(t, "auto_backup")
	fetchWithRegistry(t, f, registry, target)

	beforeReg, err := os.ReadFile(registry)
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}

	mat, res := f.materializes, f.resolves

	summary, err := Fetch(context.Background(),
		FetchOptions{Fetcher: f, RegistryPath: registry}, testSource(), target)
	if err != nil {
		t.Fatalf("re-fetch: %v", err)
	}

	if !strings.Contains(summary, "Already up to date") || !strings.Contains(summary, ShortSHA(testCommitA)) {
		t.Fatalf("summary = %q, want no-op status", summary)
	}

	if f.materializes != mat {
		t.Fatal("no-op re-fetch re-downloaded contents")
	}

	if f.resolves != res+1 {
		t.Fatal("no-op re-fetch skipped the freshness check")
	}

	afterReg, err := os.ReadFile(registry)
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}

	if string(beforeReg) != string(afterReg) {
		t.Fatal("no-op re-fetch rewrote the registry")
	}

	data, err := os.ReadFile(filepath.Join(target, "a.txt"))
	if err != nil || string(data) != "alpha" {
		t.Fatalf("content = %q, err = %v", data, err)
	}
}

func TestFetchMovedUpstreamRefetchesWithForce(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	registry, target := fetchRun(t, "auto_backup")
	fetchWithRegistry(t, f, registry, target)

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{"a.txt": "alpha2"}

	summary, err := Fetch(context.Background(),
		FetchOptions{Fetcher: f, Force: true, RegistryPath: registry}, testSource(), target)
	if err != nil {
		t.Fatalf("re-fetch: %v", err)
	}

	if !strings.Contains(summary, "Fetched") || !strings.Contains(summary, ShortSHA(testCommitB)) {
		t.Fatalf("summary = %q, want fresh fetch", summary)
	}

	data, err := os.ReadFile(filepath.Join(target, "a.txt"))
	if err != nil || string(data) != "alpha2" {
		t.Fatalf("content = %q, err = %v", data, err)
	}

	reg, err := LoadRegistry(registry)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	entry, err := reg.Lookup("./auto_backup")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	if entry.ResolvedCommit != testCommitB {
		t.Fatalf("commit = %q, want %q", entry.ResolvedCommit, testCommitB)
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

// TestCheckTargetEdges locks the guard contract the fast path must preserve:
// missing and empty targets pass, a regular file fails without the
// not-empty sentinel, a populated directory reports its count, and force
// bypasses the population check.
func TestCheckTargetEdges(t *testing.T) {
	t.Parallel()

	if err := checkTarget(filepath.Join(t.TempDir(), "nope"), false); err != nil {
		t.Fatalf("missing target: %v", err)
	}

	if err := checkTarget(t.TempDir(), false); err != nil {
		t.Fatalf("empty target: %v", err)
	}

	regular := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(regular, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := checkTarget(regular, false); err == nil {
		t.Fatal("regular file target passed the guard")
	} else if errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("regular file reported as non-empty directory: %v", err)
	}

	full := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		writeFile(t, full, name, "data")
	}

	if err := checkTarget(full, false); !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("err = %v, want ErrTargetNotEmpty", err)
	} else if !strings.Contains(err.Error(), "3 entries") {
		t.Fatalf("err = %v, want the entry count", err)
	}

	if err := checkTarget(full, true); err != nil {
		t.Fatalf("force on populated target: %v", err)
	}
}
