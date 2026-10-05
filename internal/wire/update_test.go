package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fetchFixture fetches the fake's initial commit into a fresh run dir,
// returning the run-level registry path and the checkout target.
func fetchFixture(t *testing.T, f *fakeFetcher) (registry, target string) {
	t.Helper()

	run := t.TempDir()
	registry = RegistryPath(run)
	target = filepath.Join(run, "checkout")

	opts := FetchOptions{Fetcher: f, RegistryPath: registry}

	if _, err := Fetch(context.Background(), opts, testSource(), target); err != nil {
		t.Fatalf("fixture fetch: %v", err)
	}

	return registry, target
}

// updateFixture runs Update against the fixture registry.
func updateFixture(ctx context.Context, t *testing.T, f *fakeFetcher, registry, target string, force bool) string {
	t.Helper()

	summary, err := Update(ctx, UpdateOptions{Fetcher: f, Force: force, RegistryPath: registry}, target)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	return summary
}

// lookupFixture reads the fixture entry for assertions.
func lookupFixture(t *testing.T, registry string) RegistryEntry {
	t.Helper()

	reg, err := LoadRegistry(registry)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	entry, err := reg.Lookup("./checkout")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	return entry
}

func readTarget(t *testing.T, target, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	return string(data)
}

func TestUpdateNoOp(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	registry, target := fetchFixture(t, f)

	beforeReg, err := os.ReadFile(registry)
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}

	summary := updateFixture(context.Background(), t, f, registry, target, false)

	if !strings.Contains(summary, "Already up to date") {
		t.Fatalf("summary = %q", summary)
	}

	afterReg, err := os.ReadFile(registry)
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}

	if string(beforeReg) != string(afterReg) {
		t.Fatal("no-op update rewrote the registry")
	}

	if readTarget(t, target, "a.txt") != "alpha" {
		t.Fatal("no-op update changed files")
	}
}

func TestUpdateBehind(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	registry, target := fetchFixture(t, f)

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{"a.txt": "alpha2", "c.txt": "gamma"}

	summary, err := Update(context.Background(), UpdateOptions{Fetcher: f, RegistryPath: registry}, target)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if !strings.Contains(summary, "Updated") || !strings.Contains(summary, ShortSHA(testCommitB)) {
		t.Fatalf("summary = %q", summary)
	}

	if readTarget(t, target, "a.txt") != "alpha2" || readTarget(t, target, "c.txt") != "gamma" {
		t.Fatal("target does not reflect new upstream state")
	}

	if _, err := os.Stat(filepath.Join(target, "sub", "b.txt")); !os.IsNotExist(err) {
		t.Fatal("removed upstream file still present")
	}

	if got := lookupFixture(t, registry).ResolvedCommit; got != testCommitB {
		t.Fatalf("commit = %q", got)
	}
}

func TestUpdateDiverged(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	registry, target := fetchFixture(t, f)
	writeFile(t, target, "a.txt", "local edits")

	_, err := Update(context.Background(), UpdateOptions{Fetcher: f, RegistryPath: registry}, target)
	if !errors.Is(err, ErrDiverged) {
		t.Fatalf("err = %v, want ErrDiverged", err)
	}

	if readTarget(t, target, "a.txt") != "local edits" {
		t.Fatal("refusal modified local files")
	}

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{"a.txt": "alpha2"}

	forced, err := Update(context.Background(), UpdateOptions{Fetcher: f, Force: true, RegistryPath: registry}, target)
	if err != nil {
		t.Fatalf("forced Update: %v", err)
	}

	if !strings.Contains(forced, "Updated") {
		t.Fatalf("summary = %q", forced)
	}

	if readTarget(t, target, "a.txt") != "alpha2" {
		t.Fatal("force did not discard local edits")
	}
}

func TestUpdateNotACheckout(t *testing.T) {
	t.Parallel()

	run := t.TempDir()
	registry := RegistryPath(run)

	// Unknown key under a valid tree: no entry was ever fetched here.
	_, err := Update(context.Background(),
		UpdateOptions{Fetcher: successFetcher(), RegistryPath: registry}, filepath.Join(run, "ghost"))
	if !errors.Is(err, ErrNotACheckout) {
		t.Fatalf("err = %v, want ErrNotACheckout", err)
	}

	// Target outside the registry tree is not tracked here either.
	_, err = Update(context.Background(),
		UpdateOptions{Fetcher: successFetcher(), RegistryPath: registry}, filepath.Join(t.TempDir(), "far"))
	if !errors.Is(err, ErrNotACheckout) {
		t.Fatalf("err = %v, want ErrNotACheckout", err)
	}
}

func TestUpdateMissingTargetDir(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	registry, target := fetchFixture(t, f)

	if err := os.RemoveAll(target); err != nil {
		t.Fatalf("remove target: %v", err)
	}

	_, err := Update(context.Background(), UpdateOptions{Fetcher: f, RegistryPath: registry}, target)
	if !errors.Is(err, ErrDiverged) {
		t.Fatalf("err = %v, want ErrDiverged", err)
	}

	if got := lookupFixture(t, registry).ResolvedCommit; got != testCommitA {
		t.Fatal("missing-target update rewrote the registry entry")
	}
}

func TestUpdateMissingUpstreamPath(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	registry, target := fetchFixture(t, f)

	f.files = map[string]map[string]string{}
	f.commits["19.0"] = testCommitB

	_, err := Update(context.Background(), UpdateOptions{Fetcher: f, RegistryPath: registry}, target)
	if !errors.Is(err, ErrMissingPath) {
		t.Fatalf("err = %v, want ErrMissingPath", err)
	}

	if readTarget(t, target, "a.txt") != "alpha" {
		t.Fatal("failed update touched local files")
	}

	if got := lookupFixture(t, registry).ResolvedCommit; got != testCommitA {
		t.Fatal("failed update rewrote the registry entry")
	}
}

func TestUpdateUnreachable(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	registry, target := fetchFixture(t, f)
	f.resolveErr = context.DeadlineExceeded

	_, err := Update(context.Background(), UpdateOptions{Fetcher: f, RegistryPath: registry}, target)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the transport cause", err)
	}

	if readTarget(t, target, "a.txt") != "alpha" {
		t.Fatal("failed update touched local files")
	}
}

func TestUpdateMergeClean(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	registry, target := fetchFixture(t, f)

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{
		"a.txt":     "alpha2",
		"sub/b.txt": "beta",
		"c.txt":     "gamma",
	}

	writeFile(t, target, "sub/b.txt", "local edits")

	summary, err := Update(context.Background(), UpdateOptions{Fetcher: f, RegistryPath: registry}, target)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if !strings.Contains(summary, "Merged") || !strings.Contains(summary, "local files kept") {
		t.Fatalf("summary = %q", summary)
	}

	if readTarget(t, target, "a.txt") != "alpha2" {
		t.Fatal("upstream change missing")
	}

	if readTarget(t, target, "sub/b.txt") != "local edits" {
		t.Fatal("local edit lost")
	}

	if readTarget(t, target, "c.txt") != "gamma" {
		t.Fatal("upstream add missing")
	}

	if got := lookupFixture(t, registry).ResolvedCommit; got != testCommitB {
		t.Fatalf("commit = %q", got)
	}

	live, err := HashDir(target)
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	if entry := lookupFixture(t, registry); entry.ExportHash != live {
		t.Fatal("entry hash does not match merged checkout")
	}
}

func TestUpdateConflict(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	registry, target := fetchFixture(t, f)

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{"a.txt": "alpha2", "sub/b.txt": "beta"}

	writeFile(t, target, "a.txt", "local edits")

	_, err := Update(context.Background(), UpdateOptions{Fetcher: f, RegistryPath: registry}, target)
	if !errors.Is(err, ErrDiverged) {
		t.Fatalf("err = %v, want ErrDiverged", err)
	}

	if !strings.Contains(err.Error(), "a.txt") {
		t.Fatalf("err names no paths: %v", err)
	}

	if readTarget(t, target, "a.txt") != "local edits" {
		t.Fatal("conflict modified local files")
	}

	if got := lookupFixture(t, registry).ResolvedCommit; got != testCommitA {
		t.Fatal("conflict rewrote the registry entry")
	}
}

func TestUpdateDeleteModifyConflict(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	registry, target := fetchFixture(t, f)

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{"a.txt": "alpha"}

	writeFile(t, target, "sub/b.txt", "local edits")

	_, err := Update(context.Background(), UpdateOptions{Fetcher: f, RegistryPath: registry}, target)
	if !errors.Is(err, ErrDiverged) {
		t.Fatalf("err = %v, want ErrDiverged", err)
	}

	if readTarget(t, target, "sub/b.txt") != "local edits" {
		t.Fatal("conflict modified local files")
	}
}

func TestUpdateCleanDelete(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	registry, target := fetchFixture(t, f)

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{"a.txt": "alpha2"}

	summary, err := Update(context.Background(), UpdateOptions{Fetcher: f, RegistryPath: registry}, target)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if !strings.Contains(summary, "Updated") {
		t.Fatalf("summary = %q (want clean Updated, no local edits involved)", summary)
	}

	if _, err := os.Stat(filepath.Join(target, "sub", "b.txt")); !os.IsNotExist(err) {
		t.Fatal("clean upstream delete not applied")
	}
}
