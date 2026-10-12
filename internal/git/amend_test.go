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

	cmd := exec.CommandContext(context.Background(), "git", args...)
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

func TestRewordRefusesMerge(t *testing.T) {
	dir := initAmendRepo(t)

	chdirAmend(t, dir)

	ctx := context.Background()

	branch := strings.TrimSpace(gitAmend(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))

	gitAmend(t, dir, "checkout", "-qb", "side")

	if err := os.WriteFile(filepath.Join(dir, "b2.txt"), []byte("b2\n"), 0600); err != nil {
		t.Fatal(err)
	}

	gitAmend(t, dir, "add", "b2.txt")

	gitAmend(t, dir, "commit", "-qm", "side work")

	gitAmend(t, dir, "checkout", "-q", branch)

	gitAmend(t, dir, "merge", "--no-ff", "-m", "merge side", "side")

	mergeSHA, err := ResolveRevision(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	if ok, err := IsMergeCommit(ctx, mergeSHA); err != nil || !ok {
		t.Fatalf("IsMergeCommit(merge)=(%v,%v), want (true,nil)", ok, err)
	}

	if err := RewordCommit(ctx, mergeSHA, "new"); err == nil || !strings.Contains(err.Error(), "merge") {
		t.Errorf("RewordCommit(merge)=%v, want merge error", err)
	}
}

func TestRewordRefusesRoot(t *testing.T) {
	dir := initAmendRepo(t)

	chdirAmend(t, dir)

	ctx := context.Background()

	root, err := ResolveRevision(ctx, "HEAD~2")
	if err != nil {
		t.Fatal(err)
	}

	if err := RewordCommit(ctx, root, "new"); err == nil || !strings.Contains(err.Error(), "root") {
		t.Errorf("RewordCommit(root)=%v, want root error", err)
	}
}

func TestIsCleanTree(t *testing.T) {
	dir := initAmendRepo(t)

	chdirAmend(t, dir)

	ctx := context.Background()

	if ok, err := IsCleanTree(ctx); err != nil || !ok {
		t.Fatalf("IsCleanTree(fresh)=(%v,%v), want (true,nil)", ok, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("dirty\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if ok, err := IsCleanTree(ctx); err != nil || ok {
		t.Errorf("IsCleanTree(dirty tracked)=(%v,%v), want (false,nil)", ok, err)
	}

	gitAmend(t, dir, "checkout", "--", "a.txt")

	if err := os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("x\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if ok, err := IsCleanTree(ctx); err != nil || !ok {
		t.Errorf("IsCleanTree(untracked only)=(%v,%v), want (true,nil)", ok, err)
	}
}

func TestOverrideEnvReplacesExistingKeys(t *testing.T) {
	t.Parallel()

	base := []string{
		"PATH=/bin",
		"GIT_SEQUENCE_EDITOR=false",
		"GIT_EDITOR=false",
		"OTHER=1",
		"NOEQUALS",
	}

	got := overrideEnv(base,
		"GIT_SEQUENCE_EDITOR=sh /x",
		"GIT_EDITOR=sh /y",
		"GUD_NEWMSG=/z")

	count := func(key string) int {
		n := 0

		for _, e := range got {
			if k, _, ok := strings.Cut(e, "="); ok && k == key {
				n++
			}
		}

		return n
	}

	for _, key := range []string{"GIT_SEQUENCE_EDITOR", "GIT_EDITOR", "GUD_NEWMSG"} {
		if n := count(key); n != 1 {
			t.Errorf("%s entries = %d, want exactly 1", key, n)
		}
	}

	if got[len(got)-3] != "GIT_SEQUENCE_EDITOR=sh /x" ||
		got[len(got)-2] != "GIT_EDITOR=sh /y" ||
		got[len(got)-1] != "GUD_NEWMSG=/z" {
		t.Errorf("overrides not appended last: %q", got)
	}

	// Untouched entries (including keyless ones) survive in order.
	joined := strings.Join(got, "\n")
	for _, want := range []string{"PATH=/bin", "OTHER=1", "NOEQUALS"} {
		if !strings.Contains(joined, want) {
			t.Errorf("overrideEnv dropped %q: %q", want, got)
		}
	}
}

// TestRewordCommitIgnoresUserEditors is the end-to-end lock for duplicate env
// keys: a user with GIT_SEQUENCE_EDITOR/GIT_EDITOR set must still get gud's
// scripted rebase. Before filtering, libc getenv in the git grandchild
// returned the FIRST match (the user's "false"), failing the rebase.
func TestRewordCommitIgnoresUserEditors(t *testing.T) {
	dir := initAmendRepo(t)

	chdirAmend(t, dir)

	t.Setenv("GIT_SEQUENCE_EDITOR", "false")
	t.Setenv("GIT_EDITOR", "false")

	ctx := context.Background()

	first, err := ResolveRevision(ctx, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}

	if err := RewordCommit(ctx, first, "second rewritten hostile-env"); err != nil {
		t.Fatalf("RewordCommit with hostile editors: %v", err)
	}

	msg, err := GetCommitMessage(ctx, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}

	if msg != "second rewritten hostile-env" {
		t.Errorf("HEAD~1 message=%q, want rewritten message", msg)
	}
}
