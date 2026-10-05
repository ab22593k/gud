package wire

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeSegment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		in    string
		want  string
		dirty bool
	}{
		{name: "host", in: "github.com", want: "github.com"},
		{name: "owner", in: "OCA", want: "OCA"},
		{name: "repo dash", in: "server-tools", want: "server-tools"},
		{name: "slash becomes underscore", in: "a/b", want: "a_b", dirty: true},
		{name: "dotdot collapses", in: "..", want: "_", dirty: true},
		{name: "empty collapses", in: "", want: "_", dirty: true},
		{name: "leading dash kept", in: "-x", want: "-x"},
		{name: "control chars scrubbed", in: "a\x00b", want: "a_b", dirty: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := safeSegment(tc.in); got != tc.want {
				t.Fatalf("safeSegment(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMirrorDirStaysUnderRoot(t *testing.T) {
	t.Parallel()

	s := NewStoreWithDir(t.TempDir())

	dir := s.MirrorDir("github.com", "../../evil", "x")

	rel, err := filepath.Rel(s.Root(), dir)
	if err != nil {
		t.Fatalf("rel: %v", err)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("mirror dir escapes root: %q", dir)
	}
}

func TestRecordPath(t *testing.T) {
	t.Parallel()

	if got, want := RecordPath("/tmp/auto_backup"), filepath.Join("/tmp/auto_backup", ".git-wire.json"); got != want {
		t.Fatalf("RecordPath = %q, want %q", got, want)
	}
}

func TestWorktreeDirUnique(t *testing.T) {
	t.Parallel()

	s := NewStoreWithDir(t.TempDir())

	first, err := s.WorktreeDir()
	if err != nil {
		t.Fatalf("WorktreeDir: %v", err)
	}

	second, err := s.WorktreeDir()
	if err != nil {
		t.Fatalf("WorktreeDir: %v", err)
	}

	if first == second {
		t.Fatal("worktree dirs must be unique")
	}

	for _, dir := range []string{first, second} {
		rel, err := filepath.Rel(s.Root(), dir)
		if err != nil {
			t.Fatalf("rel: %v", err)
		}

		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("worktree dir escapes root: %q", dir)
		}

		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("worktree dir missing: %q", dir)
		}
	}
}

func TestNewStoreDefaultRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	want := filepath.Join(home, ".config", "gud", "wire")
	if s.Root() != want {
		t.Fatalf("Root = %q, want %q", s.Root(), want)
	}
}
