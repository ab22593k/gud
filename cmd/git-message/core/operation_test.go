package core

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"gud/internal/config"
	"gud/internal/git"

	"github.com/spf13/cobra"
)

// TestAppContextOperationMemoises guards the operation subprocess dedup: the
// state-file probe must run at most once per invocation, like Branch.
func TestAppContextOperationMemoises(t *testing.T) {
	app := &AppContext{}
	calls := 0
	app.operationFn = func(context.Context) git.Operation {
		calls++

		return git.OperationMerge
	}

	if got := app.Operation(context.Background()); got != git.OperationMerge {
		t.Fatalf("Operation() = %q, want merge", got)
	}
	if got := app.Operation(context.Background()); got != git.OperationMerge {
		t.Fatalf("second Operation() = %q, want merge", got)
	}
	if calls != 1 {
		t.Errorf("operation probe ran %d times, want 1 (memoised)", calls)
	}
}

func TestBuildOperationContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		op   git.Operation
		want string
	}{
		{op: git.OperationNone, want: ""},
		{op: git.OperationMerge, want: "merge"},
		{op: git.OperationCherryPick, want: "cherry-pick"},
		{op: git.OperationRevert, want: "revert"},
		{op: git.OperationRebase, want: "rebase"},
		{op: git.OperationSquash, want: "squash"},
		{op: git.OperationFixup, want: "fixup"},
	}

	for _, tt := range tests {
		t.Run(string(tt.op), func(t *testing.T) {
			t.Parallel()
			got := buildOperationContext(tt.op)
			if tt.op == git.OperationNone {
				if got != "" {
					t.Errorf("buildOperationContext(none) = %q, want empty", got)
				}
			} else if !strings.Contains(got, tt.want) {
				t.Errorf("buildOperationContext(%q) should mention %q, got: %q", tt.op, tt.want, got)
			}
		})
	}
}

// newOperationTestRepo initialises a git repository with diverged main/side
// branches and chdirs into it, leaving the repo with a non-fast-forward merge
// in progress (git merge --no-commit) so a completing commit is a real merge
// commit with two parents.
func newOperationTestRepo(t *testing.T) {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		//nolint:gosec // test-only git invocation with fixed repo-local args
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")
	if err := os.WriteFile(dir+"/file.txt", []byte("line1\n"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "feat: base")

	run("checkout", "-q", "-b", "side")
	if err := os.WriteFile(dir+"/side.txt", []byte("side\n"), 0600); err != nil {
		t.Fatalf("write side file: %v", err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "feat: side work")

	// Diverge main so the merge is not a fast-forward.
	run("checkout", "-q", "main")
	if err := os.WriteFile(dir+"/main.txt", []byte("main\n"), 0600); err != nil {
		t.Fatalf("write main file: %v", err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "feat: main work")

	// Leave a real (clean) merge stop: merged tree staged, MERGE_HEAD set.
	run("merge", "--no-commit", "side")

	t.Chdir(dir)
}

func mustOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	//nolint:gosec // test-only git invocation with fixed repo-local args
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}

	return string(out)
}

// TestInteractiveCommit_UsesPreparedMessage verifies the operation-aware flow:
// when git is mid-operation and has prepared a message, interactiveCommit
// presents that message and commits it without calling the model at all —
// no standalone message is generated over git's prior intent.
func TestInteractiveCommit_UsesPreparedMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	newOperationTestRepo(t)

	app := &AppContext{cfg: config.Config{WrapLine: 72}}
	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetOut(&outBuf)

	err := interactiveCommit(context.Background(), cmd, app,
		"diff --git a/file.txt b/file.txt", "repo context", nil,
		git.OperationMerge, "Merge branch 'side'")
	if err != nil {
		t.Fatalf("interactiveCommit() error = %v", err)
	}

	// The notice explains why no message was generated.
	if !strings.Contains(outBuf.String(), "Git merge in progress") {
		t.Errorf("output should announce the merge, got: %q", outBuf.String())
	}

	// The commit used git's prepared message unchanged.
	msg := strings.TrimSpace(mustOutput(t, ".", "log", "-1", "--format=%B"))
	if msg != "Merge branch 'side'" {
		t.Errorf("committed message = %q, want %q", msg, "Merge branch 'side'")
	}
	// The commit is a merge commit (two parents), matching the in-progress merge.
	parents := strings.Fields(strings.TrimSpace(mustOutput(t, ".", "log", "-1", "--format=%P")))
	if len(parents) != 2 {
		t.Errorf("commit has %d parents, want 2 (merge commit)", len(parents))
	}
}

// TestInteractiveCommit_PreparedMessageWithIssues verifies explicit --issue
// trailers are still applied to a preserved git message.
func TestInteractiveCommit_PreparedMessageWithIssues(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	newOperationTestRepo(t)

	app := &AppContext{cfg: config.Config{WrapLine: 72, Issues: []int{42}}}
	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetOut(&outBuf)

	err := interactiveCommit(context.Background(), cmd, app,
		"", "", nil, git.OperationMerge, "Merge branch 'side'")
	if err != nil {
		t.Fatalf("interactiveCommit() error = %v", err)
	}

	msg := strings.TrimSpace(mustOutput(t, ".", "log", "-1", "--format=%B"))
	if !strings.Contains(msg, "Fixes: #42") {
		t.Errorf("committed message missing Fixes: #42, got: %q", msg)
	}
	// Preserved messages are git's own: no Assisted-by trailer.
	if strings.Contains(msg, "Assisted-by:") {
		t.Errorf("preserved message should not carry an Assisted-by trailer, got: %q", msg)
	}
}
