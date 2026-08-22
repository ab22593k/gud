package git

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestExtractChangedPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		diff string
		want []string
	}{
		{
			name: "modified file",
			diff: "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n",
			want: []string{"file.txt"},
		},
		{
			name: "multiple files in order",
			diff: "diff --git a/a.go b/a.go\n+++ b/a.go\n" +
				"diff --git a/b.go b/b.go\n+++ b/b.go\n",
			want: []string{"a.go", "b.go"},
		},
		{
			name: "deleted files are skipped",
			diff: "diff --git a/gone.go b/gone.go\n--- a/gone.go\n+++ /dev/null\n",
			want: nil,
		},
		{
			name: "rename resolves to the new path",
			diff: "diff --git a/old.go b/new.go\n--- a/old.go\n+++ b/new.go\n",
			want: []string{"new.go"},
		},
		{
			name: "quoted path is unquoted",
			diff: "diff --git \"a/my file.txt\" \"b/my file.txt\"\n--- \"a/my file.txt\"\n+++ \"b/my file.txt\"\n",
			want: []string{"my file.txt"},
		},
		{
			name: "nested paths are preserved",
			diff: "diff --git a/cmd/app/main.go b/cmd/app/main.go\n+++ b/cmd/app/main.go\n",
			want: []string{"cmd/app/main.go"},
		},
		{
			name: "duplicates are deduplicated",
			diff: "diff --git a/x.go b/x.go\n+++ b/x.go\ndiff --git a/x.go b/x.go\n+++ b/x.go\n",
			want: []string{"x.go"},
		},
		{
			name: "empty diff",
			diff: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ExtractChangedPaths(tt.diff)
			if len(got) != len(tt.want) {
				t.Fatalf("ExtractChangedPaths() = %v, want %v", got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ExtractChangedPaths()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// newHistoryTestRepo builds a repository with a diverged topic branch and
// chdirs into it:
//
//	main:      "feat: base" → "feat: main work"
//	origin/main → "feat: base" (topic upstream)
//	topic:     "feat: base" → "feat: change file1" → "feat: change file2"
func newHistoryTestRepo(t *testing.T) {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()

		cmd := exec.CommandContext(context.Background(), "git", args...)

		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()

		if err := os.WriteFile(dir+"/"+name, []byte(content), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")
	write("file1.txt", "one\n")
	write("file2.txt", "two\n")
	run("add", ".")
	run("commit", "-q", "-m", "feat: base")

	// Publish an upstream ref and configure the topic to track it. The remote
	// URL is a throwaway path: upstream resolution only needs the remote to be
	// registered, not reachable.
	run("remote", "add", "origin", t.TempDir())
	run("update-ref", "refs/remotes/origin/main", "HEAD")
	run("config", "branch.topic.remote", "origin")
	run("config", "branch.topic.merge", "refs/heads/main")

	// Diverge: topic commits then main advances.
	run("checkout", "-q", "-b", "topic")
	write("file1.txt", "one changed\n")
	run("add", ".")
	run("commit", "-q", "-m", "feat: change file1")
	write("file2.txt", "two changed\n")
	run("add", ".")
	run("commit", "-q", "-m", "feat: change file2")
	run("checkout", "-q", "main")
	write("file3.txt", "three\n")
	run("add", ".")
	run("commit", "-q", "-m", "feat: main work")
	run("checkout", "-q", "topic")

	t.Chdir(dir)
}

func TestGetUpstreamBranch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	newHistoryTestRepo(t)

	if got := GetUpstreamBranch(ctx); got != "origin/main" {
		t.Errorf("GetUpstreamBranch() = %q, want %q", got, "origin/main")
	}
}

func TestGetUpstreamBranch_None(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()

		cmd := exec.CommandContext(context.Background(), "git", args...)

		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")

	if err := os.WriteFile(dir+"/f.txt", []byte("x\n"), 0600); err != nil {
		t.Fatal(err)
	}

	run("add", ".")
	run("commit", "-q", "-m", "init")

	t.Chdir(dir)

	if got := GetUpstreamBranch(context.Background()); got != "" {
		t.Errorf("GetUpstreamBranch() without upstream = %q, want empty", got)
	}
}

func TestGetTopicHistory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	newHistoryTestRepo(t)

	t.Run("commits since divergence", func(t *testing.T) {
		got, err := GetTopicHistory(ctx, "origin/main", 10, nil)
		if err != nil {
			t.Fatalf("GetTopicHistory() error = %v", err)
		}

		if !strings.Contains(got, "feat: change file2") || !strings.Contains(got, "feat: change file1") {
			t.Errorf("GetTopicHistory() should contain both topic commits, got:\n%s", got)
		}
		// Newest commit first, and the merge-base/main commit is excluded.
		lines := strings.Split(strings.TrimSpace(got), "\n")
		if len(lines) != 2 {
			t.Errorf("GetTopicHistory() = %d commits, want 2 (since divergence), got:\n%s", len(lines), got)
		}

		if !strings.HasSuffix(lines[0], "feat: change file2") {
			t.Errorf("newest commit should come first, got: %q", lines[0])
		}
	})

	t.Run("cap limits the result", func(t *testing.T) {
		got, err := GetTopicHistory(ctx, "origin/main", 1, nil)
		if err != nil {
			t.Fatalf("GetTopicHistory() error = %v", err)
		}

		lines := strings.Split(strings.TrimSpace(got), "\n")
		if len(lines) != 1 || !strings.Contains(lines[0], "feat: change file2") {
			t.Errorf("GetTopicHistory(1) = %q, want only the newest topic commit", got)
		}
	})

	t.Run("path limiting", func(t *testing.T) {
		got, err := GetTopicHistory(ctx, "origin/main", 10, []string{"file1.txt"})
		if err != nil {
			t.Fatalf("GetTopicHistory() error = %v", err)
		}

		if strings.Contains(got, "feat: change file2") {
			t.Errorf("path-limited history should exclude commits not touching file1.txt, got:\n%s", got)
		}

		if !strings.Contains(got, "feat: change file1") {
			t.Errorf("path-limited history should include commits touching file1.txt, got:\n%s", got)
		}
	})

	t.Run("empty upstream is a no-op", func(t *testing.T) {
		got, err := GetTopicHistory(ctx, "", 10, nil)
		if err != nil || got != "" {
			t.Errorf("GetTopicHistory() with empty upstream = %q, err %v; want empty, nil", got, err)
		}
	})

	t.Run("unresolvable upstream errors", func(t *testing.T) {
		if _, err := GetTopicHistory(ctx, "no-such-branch", 10, nil); err == nil {
			t.Error("GetTopicHistory() with unresolvable upstream should error")
		}
	})
}
