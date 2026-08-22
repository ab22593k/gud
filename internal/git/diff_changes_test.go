package git

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// newGitRunner returns a function that runs git commands in the current
// directory (the caller must have chdir'd into a repo via newTestRepo).
func runGit(t *testing.T, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// newConfiguredRepo creates a temp repo with committer identity configured
// and chdirs into it.
func newConfiguredRepo(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	newTestRepo(t)
	runGit(t, "config", "user.email", "test@example.com")
	runGit(t, "config", "user.name", "Test User")
}

func TestGetAuthor(t *testing.T) {
	newConfiguredRepo(t)

	if got, want := GetAuthor(context.Background()), "Test User <test@example.com>"; got != want {
		t.Errorf("GetAuthor() = %q, want %q", got, want)
	}
}

func TestGetStagedChanges_NothingStaged(t *testing.T) {
	newConfiguredRepo(t)

	sc, err := GetStagedChanges(context.Background())
	if err != nil {
		t.Fatalf("GetStagedChanges() error = %v", err)
	}

	if sc == nil {
		t.Fatal("GetStagedChanges() = nil, want non-nil")
	}

	if sc.Diff != "" {
		t.Errorf("Diff = %q, want empty", sc.Diff)
	}

	if len(sc.Deleted) != 0 {
		t.Errorf("Deleted = %v, want empty", sc.Deleted)
	}
}

func TestGetStagedChanges_StagedModificationAndDeletion(t *testing.T) {
	newConfiguredRepo(t)

	ctx := context.Background()

	base := "package main\n\n" +
		"// a reasonably sized original file so git does not\n" +
		"// mistake its removal for a rename of some other new file.\n"
	if err := os.WriteFile("base.go", []byte(base), 0600); err != nil {
		t.Fatalf("write base.go: %v", err)
	}

	runGit(t, "add", ".")

	if _, err := Commit(ctx, "init"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	// Stage an unrelated addition and a staged deletion in one pass. The
	// added file shares no similarity with base.go, so git reports a real
	// deletion rather than a rename.
	notes := "# Notes\n\nCompletely unrelated documentation content.\n"
	if err := os.WriteFile("notes.md", []byte(notes), 0600); err != nil {
		t.Fatalf("write notes.md: %v", err)
	}

	runGit(t, "add", "notes.md")

	if err := os.Remove("base.go"); err != nil {
		t.Fatalf("remove base.go: %v", err)
	}

	runGit(t, "add", "-A")

	sc, err := GetStagedChanges(ctx)
	if err != nil {
		t.Fatalf("GetStagedChanges() error = %v", err)
	}

	if !strings.Contains(sc.Diff, "notes.md") {
		t.Errorf("Diff missing staged addition notes.md:\n%s", sc.Diff)
	}

	if len(sc.Deleted) != 1 || sc.Deleted[0] != "base.go" {
		t.Errorf("Deleted = %v, want [base.go]", sc.Deleted)
	}
}

func TestExtractDeletedFiles(t *testing.T) {
	tests := []struct {
		name string
		diff string
		want []string
	}{
		{name: "empty diff", diff: "", want: nil},
		{
			name: "modified file is not deleted",
			diff: "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-a\n+b\n",
			want: nil,
		},
		{
			name: "single deleted file",
			diff: "--- a/old.txt\n+++ /dev/null\n-deleted content\n",
			want: []string{"old.txt"},
		},
		{
			name: "deletion after modification",
			diff: "--- a/keep.go\n+++ b/keep.go\n+x\n--- a/gone.go\n+++ /dev/null\n-y\n",
			want: []string{"gone.go"},
		},
		{
			name: "multiple deletions in order",
			diff: "--- a/one.txt\n+++ /dev/null\n--- a/two.txt\n+++ /dev/null\n",
			want: []string{"one.txt", "two.txt"},
		},
		{
			name: "dev/null without preceding minus line is ignored",
			diff: "index abc..def\n+++ /dev/null\n",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := extractDeletedFiles(tt.diff)
			if len(got) != len(tt.want) {
				t.Fatalf("extractDeletedFiles() = %v, want %v", got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractDeletedFiles()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
