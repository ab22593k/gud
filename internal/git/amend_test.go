package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitAmend(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func initAmendRepo(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}
	dir := t.TempDir()
	gitAmend(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("first-content\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitAmend(t, dir, "add", "b.txt")
	gitAmend(t, dir, "commit", "-q", "-m", "first")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitAmend(t, dir, "add", "a.txt")
	gitAmend(t, dir, "commit", "-q", "-m", "second")
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("third-content\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitAmend(t, dir, "add", "c.txt")
	gitAmend(t, dir, "commit", "-q", "-m", "third")
	return dir
}

func chdirAmend(t *testing.T, dir string) {
	t.Helper()
	t.Chdir(dir)
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")
}

func TestResolveRevision(t *testing.T) {
	dir := initAmendRepo(t)
	chdirAmend(t, dir)
	ctx := context.Background()
	sha, err := ResolveRevision(ctx, "HEAD~1")
	if err != nil {
		t.Fatalf("ResolveRevision(HEAD~1)=%v", err)
	}
	if len(sha) != 40 {
		t.Errorf("ResolveRevision len=%d, want 40 (%q)", len(sha), sha)
	}
	if _, err := ResolveRevision(ctx, "no-such-rev"); err == nil {
		t.Error("ResolveRevision(bad rev)=nil, want error")
	}
	if _, err := ResolveRevision(ctx, ""); err == nil {
		t.Error("ResolveRevision(empty)=nil, want error")
	}
}

func TestGetCommitDiff_OnlyTargetCommit(t *testing.T) {
	dir := initAmendRepo(t)
	chdirAmend(t, dir)
	ctx := context.Background()
	first, err := ResolveRevision(ctx, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	head, err := ResolveRevision(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	firstDiff, err := GetCommitDiff(ctx, first)
	if err != nil {
		t.Fatalf("GetCommitDiff(HEAD~1)=%v", err)
	}
	if !strings.Contains(firstDiff, "a.txt") || !strings.Contains(firstDiff, "hello") {
		t.Errorf("HEAD~1 diff missing own patch:\n%s", firstDiff)
	}
	if strings.Contains(firstDiff, "c.txt") || strings.Contains(firstDiff, "third-content") {
		t.Errorf("HEAD~1 diff leaks HEAD patch:\n%s", firstDiff)
	}
	if strings.Contains(firstDiff, "first-content") {
		t.Errorf("HEAD~1 diff leaks HEAD~2 patch:\n%s", firstDiff)
	}
	headDiff, err := GetCommitDiff(ctx, head)
	if err != nil {
		t.Fatalf("GetCommitDiff(HEAD)=%v", err)
	}
	if !strings.Contains(headDiff, "c.txt") || !strings.Contains(headDiff, "third-content") {
		t.Errorf("HEAD diff missing own patch:\n%s", headDiff)
	}
	if strings.Contains(headDiff, "hello") {
		t.Errorf("HEAD diff leaks HEAD~1 patch:\n%s", headDiff)
	}
	if msg, err := GetCommitMessage(ctx, head); err != nil || msg != "third" {
		t.Errorf("GetCommitMessage(HEAD)=(%q,%v), want (third,nil)", msg, err)
	}
}

func TestAmendHead(t *testing.T) {
	dir := initAmendRepo(t)
	chdirAmend(t, dir)
	ctx := context.Background()
	if _, err := AmendHead(ctx, "third rewritten"); err != nil {
		t.Fatalf("AmendHead=%v", err)
	}
	msg, err := GetCommitMessage(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if msg != "third rewritten" {
		t.Errorf("HEAD message=%q, want %q", msg, "third rewritten")
	}
}

func TestRewordCommit(t *testing.T) {
	dir := initAmendRepo(t)
	chdirAmend(t, dir)
	ctx := context.Background()
	first, err := ResolveRevision(ctx, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	if err := RewordCommit(ctx, first, "second rewritten"); err != nil {
		t.Fatalf("RewordCommit=%v", err)
	}
	msg, err := GetCommitMessage(ctx, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	if msg != "second rewritten" {
		t.Errorf("HEAD~1 message=%q, want %q", msg, "second rewritten")
	}
	headMsg, err := GetCommitMessage(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if headMsg != "third" {
		t.Errorf("HEAD message=%q, want preserved %q", headMsg, "third")
	}
}
