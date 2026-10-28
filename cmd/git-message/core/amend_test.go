package core

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"gud/internal/git"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

func TestAmendTarget(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		flagArgs   []string
		positional []string
		wantRev    string
		wantAmend  bool
	}{
		{"unset", []string{}, nil, "", false},
		{"bare flag means HEAD~1", []string{amendFlag}, nil, "HEAD~1", true},
		{"space form", []string{amendFlag}, []string{"HEAD~2"}, "HEAD~2", true},
		{"equals form", []string{amendFlag + "=HEAD~2"}, nil, "HEAD~2", true},
		{"explicit value wins over positional", []string{amendFlag + "=HEAD"}, []string{"HEAD~2"}, "HEAD", true},
		{"sha rev", []string{amendFlag + "=abc1234"}, nil, "abc1234", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cobra.Command{Use: "message"}
			cmd.Flags().String("amend", "", "Amend a previous commit")
			cmd.Flags().Lookup("amend").NoOptDefVal = amendHeadRev

			if err := cmd.ParseFlags(tc.flagArgs); err != nil {
				t.Fatalf("ParseFlags(%v)=%v", tc.flagArgs, err)
			}

			gotRev, gotAmend := amendTarget(cmd, tc.positional)

			if gotRev != tc.wantRev || gotAmend != tc.wantAmend {
				t.Errorf("amendTarget(%v,%v)=(%q,%v), want (%q,%v)",
					tc.flagArgs, tc.positional, gotRev, gotAmend, tc.wantRev, tc.wantAmend)
			}
		})
	}
}

func TestIsHeadCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	dir := t.TempDir()

	setup := func(args ...string) {
		t.Helper()

		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	setup("init", "-q")

	setup("commit", "-q", "--allow-empty", "-m", "only")

	t.Chdir(dir)

	ctx := context.Background()

	sha, err := git.ResolveRevision(ctx, "HEAD")
	if err != nil {
		t.Fatalf("ResolveRevision(HEAD)=%v", err)
	}

	if !isHeadCommit(ctx, sha) {
		t.Errorf("isHeadCommit(HEAD sha)=false, want true")
	}

	if isHeadCommit(ctx, "0000000000000000000000000000000000000000") {
		t.Error("isHeadCommit(zero sha)=true, want false")
	}
}

func TestNormalizeAmendArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"no amend", []string{"--profile", "x"}, []string{"--profile", "x"}},
		{"bare last", []string{amendFlag}, []string{amendFlag}},
		{"space form joined", []string{amendFlag, "HEAD~2"}, []string{amendFlag + "=HEAD~2"}},
		{"equals untouched", []string{amendFlag + "=HEAD~2"}, []string{amendFlag + "=HEAD~2"}},
		{"flag next stays bare", []string{amendFlag, "--profile", "x"}, []string{amendFlag, "--profile", "x"}},
		{"empty", nil, []string{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeAmendArgs(tc.in); !slices.Equal(got, tc.want) {
				t.Errorf("normalizeAmendArgs(%v)=%v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// amendRefusalTestRepo builds a repo with identity configured and chdirs
// into it, returning git invocation scoped to it.
func amendRefusalTestRepo(t *testing.T) (dir string, run func(args ...string)) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	dir = t.TempDir()

	run = func(args ...string) {
		t.Helper()

		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")

	t.Chdir(dir)

	return dir, run
}

// TestResolveAmendCommitRefusals pins FR-029: the root commit is refused for
// HEAD as well as any revision, naming the reason — and the pre-existing
// refusals (merge commit, dirty tree for an earlier commit, in-progress
// operation) behave as before. Refusal happens before any diff or message
// work, so nothing is rewritten.
func TestResolveAmendCommitRefusals(t *testing.T) {
	t.Run("root HEAD refused", func(t *testing.T) {
		_, run := amendRefusalTestRepo(t)
		run("commit", "-q", "--allow-empty", "-m", "only")

		_, err := resolveAmendCommit(context.Background(), &AppContext{}, "HEAD")
		if err == nil {
			t.Fatal("resolveAmendCommit(HEAD on root): expected error, got nil")
		}

		if !strings.Contains(err.Error(), "root commit") {
			t.Errorf("error = %v, want the root-commit reason named", err)
		}
	})

	t.Run("root by revision refused", func(t *testing.T) {
		_, run := amendRefusalTestRepo(t)
		run("commit", "-q", "--allow-empty", "-m", "first")
		run("commit", "-q", "--allow-empty", "-m", "second")

		_, err := resolveAmendCommit(context.Background(), &AppContext{}, "HEAD~1")
		if err == nil {
			t.Fatal("resolveAmendCommit(HEAD~1 root): expected error, got nil")
		}

		if !strings.Contains(err.Error(), "root commit") {
			t.Errorf("error = %v, want the root-commit reason named", err)
		}
	})

	t.Run("merge commit refused", func(t *testing.T) {
		_, run := amendRefusalTestRepo(t)
		run("commit", "-q", "--allow-empty", "-m", "base")
		run("checkout", "-q", "-b", "side")
		run("commit", "-q", "--allow-empty", "-m", "side work")
		run("checkout", "-q", "main")
		run("merge", "--no-ff", "-m", "merge side", "side")

		_, err := resolveAmendCommit(context.Background(), &AppContext{}, "HEAD")
		if err == nil {
			t.Fatal("resolveAmendCommit(merge HEAD): expected error, got nil")
		}

		if !strings.Contains(err.Error(), "merge") {
			t.Errorf("error = %v, want the merge-commit reason named", err)
		}
	})

	t.Run("dirty tree for earlier commit refused", func(t *testing.T) {
		dir, run := amendRefusalTestRepo(t)
		run("commit", "-q", "--allow-empty", "-m", "first")

		if err := os.WriteFile(dir+"/tracked.txt", []byte("v1\n"), 0600); err != nil {
			t.Fatalf("write tracked.txt: %v", err)
		}

		run("add", "tracked.txt")
		run("commit", "-q", "-m", "second")
		run("commit", "-q", "--allow-empty", "-m", "third")

		// A tracked modification (unstaged) dirties the tree; untracked
		// scratch files are ignored by design, so touch a tracked file.
		if err := os.WriteFile(dir+"/tracked.txt", []byte("v1 dirty\n"), 0600); err != nil {
			t.Fatalf("dirty tracked.txt: %v", err)
		}

		_, err := resolveAmendCommit(context.Background(), &AppContext{}, "HEAD~1")
		if err == nil {
			t.Fatal("resolveAmendCommit(HEAD~1, dirty tree): expected error, got nil")
		}

		if !strings.Contains(err.Error(), "working tree") {
			t.Errorf("error = %v, want the dirty-tree reason named", err)
		}
	})

	t.Run("in-progress operation refused", func(t *testing.T) {
		dir, run := amendRefusalTestRepo(t)
		run("commit", "-q", "--allow-empty", "-m", "first")
		run("commit", "-q", "--allow-empty", "-m", "second")

		mergeHead := []byte("0000000000000000000000000000000000000000\n")
		if err := os.WriteFile(dir+"/.git/MERGE_HEAD", mergeHead, 0600); err != nil {
			t.Fatalf("write MERGE_HEAD: %v", err)
		}

		_, err := resolveAmendCommit(context.Background(), &AppContext{}, "HEAD")
		if err == nil {
			t.Fatal("resolveAmendCommit(HEAD, merge in progress): expected error, got nil")
		}

		if !strings.Contains(err.Error(), "merge") {
			t.Errorf("error = %v, want the in-progress reason named", err)
		}
	})

	t.Run("no commits refused before resolution", func(t *testing.T) {
		amendRefusalTestRepo(t)

		_, err := resolveAmendCommit(context.Background(), &AppContext{}, "HEAD")
		if err == nil {
			t.Fatal("resolveAmendCommit(HEAD, unborn repo): expected error, got nil")
		}

		if !strings.Contains(err.Error(), "no commits") {
			t.Errorf("error = %v, want the no-commits reason named", err)
		}

		if strings.Contains(err.Error(), "Needed a single revision") {
			t.Errorf("error = %v, want the refusal before revision resolution, not git's fatal", err)
		}
	})
}

// TestAmendChangePaths pins the amend wire-up: the target commit's own
// patch supplies the paths — never its parent's — the probe runs exactly
// once, and a description naming the pre-image is grounded even when the
// job diff is cut before the rename record.
func TestAmendChangePaths(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	_, run := amendRefusalTestRepo(t)
	ctx := context.Background()

	run("commit", "-q", "--allow-empty", "-m", "base")

	if err := os.WriteFile("old_name.go", []byte("package old\n"), 0600); err != nil {
		t.Fatalf("write old_name.go: %v", err)
	}

	run("add", ".")
	run("commit", "-q", "-m", "add old_name.go")
	run("mv", "old_name.go", "new_name.go")
	run("add", "-A")
	run("commit", "-q", "-m", "rename to new")
	run("commit", "-q", "--allow-empty", "-m", "tip")

	t.Run("HEAD reword grounds pre-image", func(t *testing.T) {
		var calls atomic.Int32

		app := &AppContext{commitPathsFn: func(_ context.Context, sha string) ([]string, error) {
			calls.Add(1)

			return git.CommitChangePaths(ctx, sha)
		}}

		job, err := resolveAmendCommit(ctx, app, "HEAD~1")
		if err != nil {
			t.Fatalf("resolveAmendCommit(HEAD~1) = %v", err)
		}

		if calls.Load() != 1 {
			t.Fatalf("commit probe calls = %d, want 1", calls.Load())
		}

		for _, want := range []string{"old_name.go", "new_name.go"} {
			if !slices.Contains(job.paths, want) {
				t.Errorf("job.paths = %v, want %q", job.paths, want)
			}
		}

		msg := "Rename old file\n\nRename old_name.go to new_name.go."
		if got := request.VerifyMessage(msg, request.Change{Diff: job.diff, Paths: job.paths}, 72); len(got) != 0 {
			t.Errorf("VerifyMessage(pre-image) = %v, want clean", got)
		}

		// Even with the job diff cut before the rename record, the probe
		// paths still ground the pre-image.
		cut := cutBeforeRename(t, job.diff)
		if got := request.VerifyMessage(msg, request.Change{Diff: cut, Paths: job.paths}, 72); len(got) != 0 {
			t.Errorf("VerifyMessage(cut diff) = %v, want clean (probe supplies pre-image)", got)
		}
	})

	t.Run("paths come from the target, never its parent", func(t *testing.T) {
		var calls atomic.Int32

		app := &AppContext{commitPathsFn: func(_ context.Context, sha string) ([]string, error) {
			calls.Add(1)

			return git.CommitChangePaths(ctx, sha)
		}}

		job, err := resolveAmendCommit(ctx, app, "HEAD~1")
		if err != nil {
			t.Fatalf("resolveAmendCommit(HEAD~1) = %v", err)
		}

		if calls.Load() != 1 {
			t.Fatalf("commit probe calls = %d, want 1", calls.Load())
		}

		tipPaths, err := git.CommitChangePaths(ctx, "HEAD")
		if err != nil {
			t.Fatalf("CommitChangePaths(HEAD) = %v", err)
		}

		if slices.Contains(tipPaths, "old_name.go") || slices.Contains(tipPaths, "new_name.go") {
			t.Errorf("tip commit claims rename paths: tip=%v", tipPaths)
		}

		if !slices.Contains(job.paths, "old_name.go") {
			t.Errorf("job.paths = %v, want the target's own pre-image", job.paths)
		}
	})
}

// cutBeforeRename truncates diff just before the rename-record line, or
// skips the caller when the diff has no rename record to cut.
func cutBeforeRename(t *testing.T, diff string) string {
	t.Helper()

	for line := range strings.Lines(diff) {
		if strings.HasPrefix(line, "rename from") {
			if before, _, ok := strings.Cut(diff, line); ok {
				return before
			}
		}
	}

	t.Skip("no rename record to cut; skipping cut-diff case")

	return ""
}
