package core

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"gud/internal/git"

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
		{"bare flag means HEAD", []string{amendFlag}, nil, "HEAD", true},
		{"space form", []string{amendFlag}, []string{"HEAD~2"}, "HEAD~2", true},
		{"equals form", []string{amendFlag + "=HEAD~2"}, nil, "HEAD~2", true},
		{"explicit value wins over positional", []string{amendFlag + "=HEAD~1"}, []string{"HEAD~2"}, "HEAD~1", true},
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
}
