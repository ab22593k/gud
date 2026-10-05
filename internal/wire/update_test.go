package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fetchFixture fetches the fake's initial commit into a fresh target.
func fetchFixture(t *testing.T, f *fakeFetcher) string {
	t.Helper()

	target := filepath.Join(t.TempDir(), "checkout")

	if _, err := Fetch(context.Background(), FetchOptions{Fetcher: f}, testSource(), target); err != nil {
		t.Fatalf("fixture fetch: %v", err)
	}

	return target
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
	target := fetchFixture(t, f)

	beforeRec, err := os.ReadFile(RecordPath(target))
	if err != nil {
		t.Fatalf("read record: %v", err)
	}

	summary, err := Update(context.Background(), UpdateOptions{Fetcher: f}, target)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if !strings.Contains(summary, "Already up to date") {
		t.Fatalf("summary = %q", summary)
	}

	afterRec, err := os.ReadFile(RecordPath(target))
	if err != nil {
		t.Fatalf("read record: %v", err)
	}

	if string(beforeRec) != string(afterRec) {
		t.Fatal("no-op update rewrote the tracking record")
	}

	if readTarget(t, target, "a.txt") != "alpha" {
		t.Fatal("no-op update changed files")
	}
}

func TestUpdateBehind(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	target := fetchFixture(t, f)

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{"a.txt": "alpha2", "c.txt": "gamma"}

	summary, err := Update(context.Background(), UpdateOptions{Fetcher: f}, target)
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

	rec, err := LoadRecord(target)
	if err != nil {
		t.Fatalf("LoadRecord: %v", err)
	}

	if rec.ResolvedCommit != testCommitB {
		t.Fatalf("commit = %q", rec.ResolvedCommit)
	}
}

func TestUpdateDiverged(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	target := fetchFixture(t, f)
	writeFile(t, target, "a.txt", "local edits")

	_, err := Update(context.Background(), UpdateOptions{Fetcher: f}, target)
	if !errors.Is(err, ErrDiverged) {
		t.Fatalf("err = %v, want ErrDiverged", err)
	}

	if readTarget(t, target, "a.txt") != "local edits" {
		t.Fatal("refusal modified local files")
	}

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{"a.txt": "alpha2"}

	forced, err := Update(context.Background(), UpdateOptions{Fetcher: f, Force: true}, target)
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

	_, err := Update(context.Background(), UpdateOptions{Fetcher: successFetcher()}, t.TempDir())
	if !errors.Is(err, ErrNotACheckout) {
		t.Fatalf("err = %v, want ErrNotACheckout", err)
	}
}

func TestUpdateMissingUpstreamPath(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	target := fetchFixture(t, f)

	f.files = map[string]map[string]string{}
	f.commits["19.0"] = testCommitB

	_, err := Update(context.Background(), UpdateOptions{Fetcher: f}, target)
	if !errors.Is(err, ErrMissingPath) {
		t.Fatalf("err = %v, want ErrMissingPath", err)
	}

	if readTarget(t, target, "a.txt") != "alpha" {
		t.Fatal("failed update touched local files")
	}

	rec, err := LoadRecord(target)
	if err != nil {
		t.Fatalf("LoadRecord: %v", err)
	}

	if rec.ResolvedCommit != testCommitA {
		t.Fatal("failed update rewrote the tracking record")
	}
}

func TestUpdateUnreachable(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	target := fetchFixture(t, f)
	f.resolveErr = context.DeadlineExceeded

	_, err := Update(context.Background(), UpdateOptions{Fetcher: f}, target)
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
	target := fetchFixture(t, f)

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{
		"a.txt":     "alpha2",
		"sub/b.txt": "beta",
		"c.txt":     "gamma",
	}

	writeFile(t, target, "sub/b.txt", "local edits")

	summary, err := Update(context.Background(), UpdateOptions{Fetcher: f}, target)
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

	rec, err := LoadRecord(target)
	if err != nil {
		t.Fatalf("LoadRecord: %v", err)
	}

	if rec.ResolvedCommit != testCommitB {
		t.Fatalf("commit = %q", rec.ResolvedCommit)
	}

	live, err := HashDir(target)
	if err != nil {
		t.Fatalf("HashDir: %v", err)
	}

	if rec.ExportHash != live {
		t.Fatal("record hash does not match merged checkout")
	}
}

func TestUpdateConflict(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	target := fetchFixture(t, f)

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{"a.txt": "alpha2", "sub/b.txt": "beta"}

	writeFile(t, target, "a.txt", "local edits")

	_, err := Update(context.Background(), UpdateOptions{Fetcher: f}, target)
	if !errors.Is(err, ErrDiverged) {
		t.Fatalf("err = %v, want ErrDiverged", err)
	}

	if !strings.Contains(err.Error(), "a.txt") {
		t.Fatalf("err names no paths: %v", err)
	}

	if readTarget(t, target, "a.txt") != "local edits" {
		t.Fatal("conflict modified local files")
	}

	rec, err := LoadRecord(target)
	if err != nil {
		t.Fatalf("LoadRecord: %v", err)
	}

	if rec.ResolvedCommit != testCommitA {
		t.Fatal("conflict rewrote the tracking record")
	}
}

func TestUpdateDeleteModifyConflict(t *testing.T) {
	t.Parallel()

	f := successFetcher()
	target := fetchFixture(t, f)

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{"a.txt": "alpha"}

	writeFile(t, target, "sub/b.txt", "local edits")

	_, err := Update(context.Background(), UpdateOptions{Fetcher: f}, target)
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
	target := fetchFixture(t, f)

	f.commits["19.0"] = testCommitB
	f.files[testCommitB] = map[string]string{"a.txt": "alpha2"}

	summary, err := Update(context.Background(), UpdateOptions{Fetcher: f}, target)
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
