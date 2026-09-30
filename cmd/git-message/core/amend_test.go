package core

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"gud/internal/config"
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
		{"no amend", []string{"--persona", "x"}, []string{"--persona", "x"}},
		{"bare last", []string{amendFlag}, []string{amendFlag}},
		{"space form joined", []string{amendFlag, "HEAD~2"}, []string{amendFlag + "=HEAD~2"}},
		{"equals untouched", []string{amendFlag + "=HEAD~2"}, []string{amendFlag + "=HEAD~2"}},
		{"flag next stays bare", []string{amendFlag, "--persona", "x"}, []string{amendFlag, "--persona", "x"}},
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

// TestResolveAmendCommit_FiltersRemovedContent commits a deletion as HEAD and
// verifies the amend prompt diff excludes removed lines while naming the
// file; opt-in restores full content.
func TestResolveAmendCommit_FiltersRemovedContent(t *testing.T) {
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

	setup("init", "-q", "-b", "main")

	if err := os.WriteFile(dir+"/doomed.go", []byte("package doomed\n// removed line\n"), 0600); err != nil {
		t.Fatalf("write doomed.go: %v", err)
	}

	if err := os.WriteFile(dir+"/keep.go", []byte("package main\n"), 0600); err != nil {
		t.Fatalf("write keep.go: %v", err)
	}

	setup("add", ".")
	setup("commit", "-q", "-m", "init")
	setup("rm", "-q", "doomed.go")
	setup("commit", "-q", "-m", "delete doomed")

	t.Chdir(dir)

	ctx := context.Background()

	job, err := resolveAmendCommit(ctx, &AppContext{}, "HEAD")
	if err != nil {
		t.Fatalf("resolveAmendCommit(HEAD) error = %v", err)
	}

	if strings.Contains(job.diff, "removed line") {
		t.Errorf("amend diff leaks deleted content:\n%s", job.diff)
	}

	if !strings.Contains(job.diff, "doomed.go") {
		t.Errorf("amend diff missing deleted name:\n%s", job.diff)
	}

	optIn := &AppContext{cfg: config.Config{IncludeRemovedContent: config.Ptr(true)}}

	full, err := resolveAmendCommit(ctx, optIn, "HEAD")
	if err != nil {
		t.Fatalf("resolveAmendCommit(HEAD, opt-in) error = %v", err)
	}

	if !strings.Contains(full.diff, "removed line") {
		t.Errorf("opt-in amend diff missing deleted content:\n%s", full.diff)
	}
}
