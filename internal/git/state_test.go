package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestHasCommits pins the D8 state probe: a repo with one commit reports
// presence, a fresh git init with no commit reports unborn (no error), and a
// non-repository reports a probe error rather than being mistaken for an
// unborn repository. The exit code — not the stderr text — is asserted, so
// the test is locale-independent.
func TestHasCommits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	ctx := context.Background()

	t.Run("repo with one commit", func(t *testing.T) {
		dir := t.TempDir()
		gitAmend(t, dir, "init", "-q", "-b", "main")
		gitAmend(t, dir, "config", "user.name", "Test")
		gitAmend(t, dir, "config", "user.email", "test@example.com")

		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0600); err != nil {
			t.Fatalf("write a.txt: %v", err)
		}

		gitAmend(t, dir, "add", "a.txt")
		gitAmend(t, dir, "commit", "-q", "-m", "feat: first")

		t.Chdir(dir)

		has, err := HasCommits(ctx)
		if err != nil {
			t.Fatalf("HasCommits() = %v, want nil", err)
		}

		if !has {
			t.Error("HasCommits() = false, want true for a repo with one commit")
		}
	})

	t.Run("fresh repo with no commits", func(t *testing.T) {
		dir := t.TempDir()
		gitAmend(t, dir, "init", "-q", "-b", "main")
		gitAmend(t, dir, "config", "user.name", "Test")
		gitAmend(t, dir, "config", "user.email", "test@example.com")

		t.Chdir(dir)

		has, err := HasCommits(ctx)
		if err != nil {
			t.Fatalf("HasCommits() = %v, want nil for an unborn repository", err)
		}

		if has {
			t.Error("HasCommits() = true, want false for an unborn repository")
		}
	})

	t.Run("non-repository is an error not an absence", func(t *testing.T) {
		dir := t.TempDir()

		t.Chdir(dir)

		has, err := HasCommits(ctx)
		if err == nil {
			t.Fatalf("HasCommits() in %s = (%v, nil), want an error", dir, has)
		}

		if has {
			t.Errorf("HasCommits() = true with error, want false with error")
		}
	})
}
