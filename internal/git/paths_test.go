package git

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// changePathsRepo builds a temp repo with identity configured and chdirs
// into it, returning a git runner scoped to it.
func changePathsRepo(t *testing.T) (run func(args ...string) string) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping temp-repo test in short mode")
	}

	dir := t.TempDir()

	run = func(args ...string) string {
		t.Helper()

		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}

		return strings.TrimSpace(string(out))
	}

	run("init", "-q", "-b", "main")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")

	t.Chdir(dir)

	return run
}

// TestParseChangePaths pins the D9 NUL-record parser without a repository:
// one path per M/D/A record, both paths for R/C records (with or without a
// score suffix), byte-exact paths, and a wrapped shape-naming error — never
// path content — for malformed input. Empty input yields a non-nil empty
// slice so nil stays reserved for "probe unavailable".
func TestParseChangePaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    []string
		wantErr string
	}{
		{name: "modified", in: "M\x00keep.go\x00", want: []string{"keep.go"}},
		{name: "rename with score", in: "R100\x00old.go\x00new.go\x00", want: []string{"old.go", "new.go"}},
		{name: "rename without score", in: "R\x00old.go\x00new.go\x00", want: []string{"old.go", "new.go"}},
		{name: "copy with score", in: "C100\x00src.go\x00dst.go\x00", want: []string{"src.go", "dst.go"}},
		{name: "delete and add", in: "D\x00gone.go\x00A\x00added.go\x00", want: []string{"gone.go", "added.go"}},
		{
			name: "spaces survive byte-exactly",
			in:   "R100\x00a b.txt\x00c d.txt\x00",
			want: []string{"a b.txt", "c d.txt"},
		},
		{
			name: "newline survives byte-exactly",
			in:   "M\x00a\nb.txt\x00",
			want: []string{"a\nb.txt"},
		},
		{
			name: "non-ascii survives byte-exactly",
			in:   "R100\x00ümlaut.txt\x00ö.txt\x00",
			want: []string{"ümlaut.txt", "ö.txt"},
		},
		{name: "empty input", in: "", want: []string{}},
		{name: "short record", in: "R100\x00only.go\x00", wantErr: "short"},
		{name: "unknown status", in: "?\x00weird.go\x00", wantErr: "unknown status"},
		{name: "trailing non-empty field", in: "M\x00keep.go\x00junk", wantErr: "trailing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseChangePaths([]byte(tt.in))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parseChangePaths(%q) = %v, want error containing %q", tt.in, got, tt.wantErr)
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %v, want shape %q named", err, tt.wantErr)
				}

				if strings.Contains(err.Error(), tt.in) && tt.in != "" {
					t.Errorf("error = %v, must not echo path content", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseChangePaths(%q) = %v", tt.in, err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseChangePaths(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

// TestStagedChangePaths pins the staged-index probe: git's own rename
// classification (via explicit -M/-C) supplies both names even when the
// rendered diff would show a delete+add, repository config cannot suppress
// it, and a clean index yields an empty slice with no error.
func TestStagedChangePaths(t *testing.T) {
	run := changePathsRepo(t)
	ctx := context.Background()

	write := func(name, content string) {
		t.Helper()

		if err := os.WriteFile(name, []byte(content), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	write("old_name.go", "package old\n")
	write("keep.go", "package keep\n")
	run("add", ".")
	run("commit", "-q", "-m", "init")
	// A clean index reports no paths and no error.
	if got, err := StagedChangePaths(ctx); err != nil || len(got) != 0 {
		t.Fatalf("StagedChangePaths(clean) = (%v, %v), want (empty, nil)", got, err)
	}

	// A true rename reports both names.
	run("mv", "old_name.go", "new_name.go")
	write("keep.go", "package keep\n// edit\n")
	run("add", "-A")

	got, err := StagedChangePaths(ctx)
	if err != nil {
		t.Fatalf("StagedChangePaths() = %v", err)
	}

	for _, want := range []string{"old_name.go", "new_name.go", "keep.go"} {
		if !containsPath(got, want) {
			t.Errorf("StagedChangePaths() = %v, want %q", got, want)
		}
	}

	// Below-threshold similarity: the same change reads as delete+add, and
	// both names are still returned because git reports D and A records.
	write("new_name.go", "package totally_unrelated_content_zero_overlap_here\n")
	run("add", "-A")

	got, err = StagedChangePaths(ctx)
	if err != nil {
		t.Fatalf("StagedChangePaths() = %v", err)
	}

	for _, want := range []string{"old_name.go", "new_name.go"} {
		if !containsPath(got, want) {
			t.Errorf("StagedChangePaths(delete+add) = %v, want %q", got, want)
		}
	}

	// Repository config cannot suppress the rename: -M is explicit.
	run("config", "diff.renames", "false")
	run("mv", "keep.go", "kept.go")
	run("add", "-A")

	got, err = StagedChangePaths(ctx)
	if err != nil {
		t.Fatalf("StagedChangePaths() = %v", err)
	}

	for _, want := range []string{"keep.go", "kept.go"} {
		if !containsPath(got, want) {
			t.Errorf("StagedChangePaths(diff.renames=false) = %v, want %q", got, want)
		}
	}

	// A copy from a modified source reports both sides of the C record.
	src := strings.Repeat("line\n", 20)
	write("src.go", src)
	run("add", ".")
	run("commit", "-q", "-m", "add src")
	write("src.go", src+"extra\n")
	write("dst.go", src+"extra\n")
	run("add", "-A")

	got, err = StagedChangePaths(ctx)
	if err != nil {
		t.Fatalf("StagedChangePaths() = %v", err)
	}

	for _, want := range []string{"src.go", "dst.go"} {
		if !containsPath(got, want) {
			t.Errorf("StagedChangePaths(copy) = %v, want %q", got, want)
		}
	}
}

// TestCommitChangePaths pins the commit probe: the target commit's own patch
// supplies the same records for HEAD and for an older commit — never the
// parent's records.
func TestCommitChangePaths(t *testing.T) {
	run := changePathsRepo(t)
	ctx := context.Background()

	if err := os.WriteFile("a.go", []byte("package a\n"), 0600); err != nil {
		t.Fatalf("write a.go: %v", err)
	}

	run("add", ".")
	run("commit", "-q", "-m", "add a")
	run("mv", "a.go", "b.go")
	run("add", "-A")
	run("commit", "-q", "-m", "rename a to b")

	head, err := CommitChangePaths(ctx, "HEAD")
	if err != nil {
		t.Fatalf("CommitChangePaths(HEAD) = %v", err)
	}

	for _, want := range []string{"a.go", "b.go"} {
		if !containsPath(head, want) {
			t.Errorf("CommitChangePaths(HEAD) = %v, want %q", head, want)
		}
	}

	older, err := CommitChangePaths(ctx, "HEAD~1")
	if err != nil {
		t.Fatalf("CommitChangePaths(HEAD~1) = %v", err)
	}

	if !reflect.DeepEqual(older, []string{"a.go"}) {
		t.Errorf("CommitChangePaths(HEAD~1) = %v, want [a.go] (own patch only)", older)
	}
}

func containsPath(paths []string, want string) bool {
	return slices.Contains(paths, want)
}
