package core

import (
	"context"
	"os/exec"
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
		{"bare flag means HEAD", []string{"--amend"}, nil, "HEAD", true},
		{"space form", []string{"--amend"}, []string{"HEAD~2"}, "HEAD~2", true},
		{"equals form", []string{"--amend=HEAD~2"}, nil, "HEAD~2", true},
		{"explicit value wins over positional", []string{"--amend=HEAD~1"}, []string{"HEAD~2"}, "HEAD~1", true},
		{"sha rev", []string{"--amend=abc1234"}, nil, "abc1234", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := &cobra.Command{Use: "message"}
			cmd.Flags().String("amend", "", "Amend a previous commit")
			cmd.Flags().Lookup("amend").NoOptDefVal = "HEAD"
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
		cmd := exec.Command("git", args...)
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
