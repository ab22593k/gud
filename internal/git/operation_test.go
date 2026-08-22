package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLastDoneCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		done string
		want string
	}{
		{
			name: "single pick command",
			done: "pick 1a2b3c4d # feat: change\n",
			want: "pick",
		},
		{
			name: "last of several commands wins",
			done: "pick 1a2b3c4d # feat: one\nsquash 5e6f7a8b # feat: two\n",
			want: "squash",
		},
		{
			name: "fixup with -c keeps fixup word",
			done: "pick 1a2b3c4d # feat: target\nfixup -c 5e6f7a8b # fixup! feat: target\n",
			want: "fixup",
		},
		{
			name: "comment lines are skipped",
			done: "# rebase in progress\n# comment\npick 1a2b3c4d # feat: change\n",
			want: "pick",
		},
		{
			name: "blank and comment only returns empty",
			done: "\n# comment\n",
			want: "",
		},
		{
			name: "empty file returns empty",
			done: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tt.done != "" {
				if err := os.WriteFile(filepath.Join(dir, "done"), []byte(tt.done), 0600); err != nil {
					t.Fatalf("write done: %v", err)
				}
			}

			if got := lastDoneCommand(dir); got != tt.want {
				t.Errorf("lastDoneCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCurrentRebaseCommand_TodoFallback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git-rebase-todo"), []byte("# comment\nreword 1a2b3c4d # feat: change\n"), 0600); err != nil {
		t.Fatalf("write todo: %v", err)
	}

	if got := currentRebaseCommand(dir); got != "reword" {
		t.Errorf("currentRebaseCommand() = %q, want %q", got, "reword")
	}
}

func TestCurrentRebaseCommand_NoFiles(t *testing.T) {
	t.Parallel()

	if got := currentRebaseCommand(t.TempDir()); got != "" {
		t.Errorf("currentRebaseCommand() = %q, want empty", got)
	}
}

func TestStripCommentLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "merge message with conflict comments",
			in:   "Merge branch 'side'\n\n# Conflicts:\n#\tside.txt\n",
			want: "Merge branch 'side'",
		},
		{
			name: "squash combination template",
			in:   "# This is a combination of 2 commits.\n# This is the 1st commit message:\n\nfeat: first\n\n# This is the commit message #2:\n\nfeat: second\n",
			want: "feat: first\n\n\nfeat: second",
		},
		{
			name: "plain message unchanged",
			in:   "feat: change\n\nBody.\n",
			want: "feat: change\n\nBody.",
		},
		{
			name: "comment-only becomes empty",
			in:   "# comment\n# comment\n",
			want: "",
		},
		{
			name: "empty input stays empty",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := stripCommentLines(tt.in); got != tt.want {
				t.Errorf("stripCommentLines() = %q, want %q", got, tt.want)
			}
		})
	}
}

// newTestRepo initialises a bare git repository in a temp dir, chdirs into it,
// and returns a cleanup that restores the original working directory.
func newTestRepo(t *testing.T) {
	t.Helper()
	dir := t.TempDir()

	cmd := exec.CommandContext(context.Background(), "git", "init", "-q")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Skipf("git init unavailable: %v", err)
	}

	t.Chdir(dir)
}

// writeRepoFile writes a file inside the repository (used for state files).
func writeRepoFile(t *testing.T, rel, content string) {
	t.Helper()
	if err := os.WriteFile(rel, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestDetectOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	newTestRepo(t)

	// No state files: ordinary commit.
	if got := DetectOperation(ctx); got != OperationNone {
		t.Errorf("DetectOperation() on clean repo = %q, want %q", got, OperationNone)
	}

	t.Run("merge", func(t *testing.T) {
		writeRepoFile(t, ".git/MERGE_HEAD", "deadbeef\n")
		if got := DetectOperation(ctx); got != OperationMerge {
			t.Errorf("DetectOperation() = %q, want %q", got, OperationMerge)
		}
		_ = os.Remove(".git/MERGE_HEAD")
	})

	t.Run("cherry-pick", func(t *testing.T) {
		writeRepoFile(t, ".git/CHERRY_PICK_HEAD", "deadbeef\n")
		if got := DetectOperation(ctx); got != OperationCherryPick {
			t.Errorf("DetectOperation() = %q, want %q", got, OperationCherryPick)
		}
		_ = os.Remove(".git/CHERRY_PICK_HEAD")
	})

	t.Run("revert", func(t *testing.T) {
		writeRepoFile(t, ".git/REVERT_HEAD", "deadbeef\n")
		if got := DetectOperation(ctx); got != OperationRevert {
			t.Errorf("DetectOperation() = %q, want %q", got, OperationRevert)
		}
		_ = os.Remove(".git/REVERT_HEAD")
	})

	t.Run("merge wins over rebase", func(t *testing.T) {
		if err := os.MkdirAll(".git/rebase-merge", 0750); err != nil {
			t.Fatalf("mkdir rebase-merge: %v", err)
		}
		writeRepoFile(t, ".git/MERGE_HEAD", "deadbeef\n")
		if got := DetectOperation(ctx); got != OperationMerge {
			t.Errorf("DetectOperation() = %q, want %q (merge takes precedence)", got, OperationMerge)
		}
		_ = os.Remove(".git/MERGE_HEAD")
		_ = os.RemoveAll(".git/rebase-merge")
	})

	t.Run("rebase pick stop", func(t *testing.T) {
		if err := os.MkdirAll(".git/rebase-merge", 0750); err != nil {
			t.Fatalf("mkdir rebase-merge: %v", err)
		}
		writeRepoFile(t, ".git/rebase-merge/done", "pick deadbeef # feat: change\n")
		if got := DetectOperation(ctx); got != OperationRebase {
			t.Errorf("DetectOperation() = %q, want %q", got, OperationRebase)
		}
		_ = os.RemoveAll(".git/rebase-merge")
	})

	t.Run("rebase squash stop", func(t *testing.T) {
		if err := os.MkdirAll(".git/rebase-merge", 0750); err != nil {
			t.Fatalf("mkdir rebase-merge: %v", err)
		}
		writeRepoFile(t, ".git/rebase-merge/done", "pick deadbeef # feat: one\nsquash beefdead # feat: two\n")
		if got := DetectOperation(ctx); got != OperationSquash {
			t.Errorf("DetectOperation() = %q, want %q", got, OperationSquash)
		}
		_ = os.RemoveAll(".git/rebase-merge")
	})

	t.Run("squash detected via SQUASH_MSG when done is empty", func(t *testing.T) {
		if err := os.MkdirAll(".git/rebase-merge", 0750); err != nil {
			t.Fatalf("mkdir rebase-merge: %v", err)
		}
		writeRepoFile(t, ".git/SQUASH_MSG", "# This is a combination of 2 commits.\n")
		if got := DetectOperation(ctx); got != OperationSquash {
			t.Errorf("DetectOperation() = %q, want %q", got, OperationSquash)
		}
		_ = os.Remove(".git/SQUASH_MSG")
		_ = os.RemoveAll(".git/rebase-merge")
	})

	t.Run("rebase fixup stop", func(t *testing.T) {
		if err := os.MkdirAll(".git/rebase-merge", 0750); err != nil {
			t.Fatalf("mkdir rebase-merge: %v", err)
		}
		writeRepoFile(t, ".git/rebase-merge/done", "pick deadbeef # feat: target\nfixup beefdead # fixup! feat: target\n")
		if got := DetectOperation(ctx); got != OperationFixup {
			t.Errorf("DetectOperation() = %q, want %q", got, OperationFixup)
		}
		_ = os.RemoveAll(".git/rebase-merge")
	})

	t.Run("rebase-apply am backend", func(t *testing.T) {
		if err := os.MkdirAll(".git/rebase-apply", 0750); err != nil {
			t.Fatalf("mkdir rebase-apply: %v", err)
		}
		if got := DetectOperation(ctx); got != OperationRebase {
			t.Errorf("DetectOperation() = %q, want %q", got, OperationRebase)
		}
		_ = os.RemoveAll(".git/rebase-apply")
	})
}

// TestPreparedMessage exercises message reconstruction with real commits so
// the ref lookups (git log / git rev-parse) resolve.
func TestPreparedMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	newTestRepo(t)

	commit := func(msg string) string {
		t.Helper()
		if err := os.WriteFile("file.txt", []byte("content\n"), 0600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		run := func(args ...string) {
			cmd := exec.CommandContext(ctx, "git", args...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v failed: %v\n%s", args, err, out)
			}
		}
		run("add", "file.txt")
		run("-c", "user.name=T", "-c", "user.email=t@t", "commit", "-q", "-m", msg)

		return strings.TrimSpace(runGitOutput(ctx, "rev-parse", "HEAD"))
	}

	sha := commit("feat: initial commit\n\nAdd file.txt.")

	t.Run("merge reads MERGE_MSG stripped of comments", func(t *testing.T) {
		writeRepoFile(t, ".git/MERGE_MSG", "Merge branch 'feature'\n\n# Conflicts:\n#\tfile.txt\n")
		if got := PreparedMessage(ctx, OperationMerge); got != "Merge branch 'feature'" {
			t.Errorf("PreparedMessage(merge) = %q, want %q", got, "Merge branch 'feature'")
		}
		_ = os.Remove(".git/MERGE_MSG")
	})

	t.Run("cherry-pick preserves original message", func(t *testing.T) {
		writeRepoFile(t, ".git/CHERRY_PICK_HEAD", sha+"\n")
		want := "feat: initial commit\n\nAdd file.txt."
		if got := PreparedMessage(ctx, OperationCherryPick); got != want {
			t.Errorf("PreparedMessage(cherry-pick) = %q, want %q", got, want)
		}
		_ = os.Remove(".git/CHERRY_PICK_HEAD")
	})

	t.Run("revert builds git's revert message", func(t *testing.T) {
		writeRepoFile(t, ".git/REVERT_HEAD", sha+"\n")
		want := "Revert \"feat: initial commit\"\n\nThis reverts commit " + sha + "."
		if got := PreparedMessage(ctx, OperationRevert); got != want {
			t.Errorf("PreparedMessage(revert) = %q, want %q", got, want)
		}
		_ = os.Remove(".git/REVERT_HEAD")
	})

	t.Run("squash strips the combination template", func(t *testing.T) {
		writeRepoFile(t, ".git/SQUASH_MSG",
			"# This is a combination of 2 commits.\n# This is the 1st commit message:\n\nfeat: first\n\n# This is the commit message #2:\n\nfeat: second\n")
		want := "feat: first\n\n\nfeat: second"
		if got := PreparedMessage(ctx, OperationSquash); got != want {
			t.Errorf("PreparedMessage(squash) = %q, want %q", got, want)
		}
		_ = os.Remove(".git/SQUASH_MSG")
	})

	t.Run("rebase reads REBASE_HEAD message", func(t *testing.T) {
		writeRepoFile(t, ".git/REBASE_HEAD", sha+"\n")
		want := "feat: initial commit\n\nAdd file.txt."
		if got := PreparedMessage(ctx, OperationRebase); got != want {
			t.Errorf("PreparedMessage(rebase) = %q, want %q", got, want)
		}
		_ = os.Remove(".git/REBASE_HEAD")
	})

	t.Run("fixup reads REBASE_HEAD message", func(t *testing.T) {
		writeRepoFile(t, ".git/REBASE_HEAD", sha+"\n")
		if got := PreparedMessage(ctx, OperationFixup); got == "" {
			t.Error("PreparedMessage(fixup) = empty, want the fixed-up commit message")
		}
		_ = os.Remove(".git/REBASE_HEAD")
	})

	t.Run("unknown operation returns empty", func(t *testing.T) {
		if got := PreparedMessage(ctx, OperationNone); got != "" {
			t.Errorf("PreparedMessage(none) = %q, want empty", got)
		}
	})
}

// TestDetectOperation_OutsideRepo guards the graceful degradation: detection
// must never error or panic outside a git repository. It chdirs, so it stays
// sequential like the other integration tests.
func TestDetectOperation_OutsideRepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if got := DetectOperation(context.Background()); got != OperationNone {
		t.Errorf("DetectOperation() outside a repo = %q, want %q", got, OperationNone)
	}
	if got := PreparedMessage(context.Background(), OperationMerge); got != "" {
		t.Errorf("PreparedMessage() outside a repo = %q, want empty", got)
	}
}
